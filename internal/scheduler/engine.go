package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"ecs-cdt-panel/internal/aliyun"
	"ecs-cdt-panel/internal/crypto"
	"ecs-cdt-panel/internal/db"
	"ecs-cdt-panel/internal/notify"
)

// RingLog represents a lightweight in-memory check trace item shown in instance details.
type RingLog struct {
	Seq              int64  `json:"seq"`
	InstanceIDRef    int64  `json:"instance_id_ref"`
	AliyunInstanceID string `json:"aliyun_instance_id"`
	RegionID         string `json:"region_id"`
	Level            string `json:"level"`
	Event            string `json:"event"`
	Message          string `json:"message"`
	CreatedAt        string `json:"created_at"`
}

// RuntimeStatus summarizes live scheduler state for the dashboard header.
type RuntimeStatus struct {
	Enabled         bool   `json:"enabled"`
	IntervalSeconds int    `json:"interval_seconds"`
	IsRunning       bool   `json:"is_running"`
	LastRunAt       string `json:"last_run_at"`
	NextRunAt       string `json:"next_run_at"`
	Timezone        string `json:"timezone"`
	ServerTime      string `json:"server_time"`
}

// Engine executes periodic minute-aligned CDT traffic and ECS policy checks.
type Engine struct {
	store       *db.Store
	ali         *aliyun.Client
	notifier    *notify.Sender
	defaultIntv int

	wakeCh chan struct{}

	runSem    chan struct{}
	isRunning bool
	lastRunAt time.Time
	nextRunAt time.Time

	ringMu   sync.RWMutex
	ringLogs []RingLog
	ringCap  int
	ringSeq  int64

	errMu     sync.Mutex
	lastErrBy map[string]string
	lastPrune time.Time
}

// NewEngine creates a scheduler engine.
func NewEngine(store *db.Store, ali *aliyun.Client, notifier *notify.Sender, defaultInterval int) *Engine {
	return &Engine{
		store:       store,
		ali:         ali,
		notifier:    notifier,
		defaultIntv: defaultInterval,
		wakeCh:      make(chan struct{}, 1),
		runSem:      make(chan struct{}, 1),
		ringCap:     400,
		ringLogs:    make([]RingLog, 0, 128),
		lastErrBy:   make(map[string]string),
	}
}

// Start runs the minute-aligned background polling loop until ctx is canceled.
func (e *Engine) Start(ctx context.Context) {
	// Run an initial check quickly on startup if configured instances/accounts exist.
	go e.runOnce(ctx)

	for {
		cfg, err := e.store.GetSchedulerSettings(e.defaultIntv)
		if err != nil {
			slog.Error("load scheduler settings", "err", err)
			cfg = db.SchedulerSettings{IntervalSeconds: e.defaultIntv, Enabled: true}
		}

		next := nextAlignedTick(time.Now(), cfg.IntervalSeconds)
		e.setNextRun(next)
		wait := time.Until(next)
		if wait < time.Second {
			wait = time.Second
		}

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-e.wakeCh:
			timer.Stop()
			// Re-evaluate loop (either manual trigger or settings updated).
		case <-timer.C:
			if cfg.Enabled {
				e.runOnce(ctx)
			}
		}
	}
}

// TriggerNow runs a check cycle asynchronously without waiting for the next minute tick.
func (e *Engine) TriggerNow(ctx context.Context) bool {
	if ctx.Err() != nil {
		return false
	}
	select {
	case e.runSem <- struct{}{}:
	default:
		return false
	}
	go func() {
		defer func() { <-e.runSem }()
		e.executeCycle(ctx)
	}()
	return true
}

// RunNowSync runs a check cycle synchronously until completion or ctx timeout.
func (e *Engine) RunNowSync(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	select {
	case e.runSem <- struct{}{}:
	case <-ctx.Done():
		return
	}
	defer func() { <-e.runSem }()
	e.executeCycle(ctx)
}

// Wait blocks until any in-flight check cycle finishes.
func (e *Engine) Wait() {
	e.runSem <- struct{}{}
	<-e.runSem
}

// NotifyConfigChanged wakes the scheduler loop to pick up a new interval immediately.
func (e *Engine) NotifyConfigChanged() {
	select {
	case e.wakeCh <- struct{}{}:
	default:
	}
}

// Status returns live scheduler timing and timezone info.
func (e *Engine) Status() RuntimeStatus {
	cfg, err := e.store.GetSchedulerSettings(e.defaultIntv)
	if err != nil {
		cfg = db.SchedulerSettings{IntervalSeconds: e.defaultIntv, Enabled: true}
	}
	e.ringMu.RLock()
	running := e.isRunning
	last := e.lastRunAt
	next := e.nextRunAt
	e.ringMu.RUnlock()

	now := time.Now()
	tzName, offsetSec := now.Zone()
	tzDisplay := fmt.Sprintf("%s (UTC%+d)", tzName, offsetSec/3600)
	if loc := now.Location().String(); loc != "" && loc != "Local" {
		tzDisplay = fmt.Sprintf("%s (%s)", loc, tzName)
	}

	lastStr := ""
	if !last.IsZero() {
		lastStr = last.Format(time.RFC3339)
	}
	nextStr := ""
	if !next.IsZero() && cfg.Enabled {
		nextStr = next.Format(time.RFC3339)
	}

	return RuntimeStatus{
		Enabled:         cfg.Enabled,
		IntervalSeconds: cfg.IntervalSeconds,
		IsRunning:       running,
		LastRunAt:       lastStr,
		NextRunAt:       nextStr,
		Timezone:        tzDisplay,
		ServerTime:      now.Format(time.RFC3339),
	}
}

// GetRecentLogs returns in-memory check trace logs, optionally filtered by instanceIDRef.
func (e *Engine) GetRecentLogs(instanceIDRef int64, limit int) []RingLog {
	if limit <= 0 || limit > e.ringCap {
		limit = 100
	}
	e.ringMu.RLock()
	defer e.ringMu.RUnlock()

	out := make([]RingLog, 0, limit)
	for i := len(e.ringLogs) - 1; i >= 0 && len(out) < limit; i-- {
		item := e.ringLogs[i]
		if instanceIDRef > 0 && item.InstanceIDRef != instanceIDRef {
			continue
		}
		out = append(out, item)
	}
	return out
}

func (e *Engine) setNextRun(t time.Time) {
	e.ringMu.Lock()
	e.nextRunAt = t
	e.ringMu.Unlock()
}

func (e *Engine) pushRingLog(instanceIDRef int64, aliyunID, regionID, level, event, msg string) {
	e.ringMu.Lock()
	defer e.ringMu.Unlock()
	e.ringSeq++
	entry := RingLog{
		Seq:              e.ringSeq,
		InstanceIDRef:    instanceIDRef,
		AliyunInstanceID: aliyunID,
		RegionID:         regionID,
		Level:            level,
		Event:            event,
		Message:          msg,
		CreatedAt:        time.Now().Format(time.RFC3339),
	}
	if len(e.ringLogs) >= e.ringCap {
		copy(e.ringLogs, e.ringLogs[1:])
		e.ringLogs[len(e.ringLogs)-1] = entry
	} else {
		e.ringLogs = append(e.ringLogs, entry)
	}
}

func (e *Engine) recordAudit(ctx context.Context, inst *db.Instance, level, event, msg string, sendErrNotify bool) {
	var refID int64
	var aliyunID, regionID string
	if inst != nil {
		refID = inst.ID
		aliyunID = inst.InstanceID
		regionID = inst.RegionID
	}
	e.pushRingLog(refID, aliyunID, regionID, level, event, msg)
	_ = e.store.AddAuditLog(db.AuditLog{
		InstanceIDRef:    refID,
		AliyunInstanceID: aliyunID,
		RegionID:         regionID,
		Level:            level,
		Event:            event,
		Message:          msg,
	})
	if sendErrNotify && level == "error" {
		_ = e.notifier.Dispatch(ctx, notify.Event{
			Type:       "error",
			Title:      fmt.Sprintf("CDT面板告警 · %s", regionID),
			Body:       msg,
			InstanceID: aliyunID,
			RegionID:   regionID,
		})
	}
}

type credTarget struct {
	Fingerprint     string
	Creds           aliyun.Credentials
	Label           string
	DomesticQuotaGB float64
	OverseasQuotaGB float64
}

func (e *Engine) runOnce(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}
	select {
	case e.runSem <- struct{}{}:
	default:
		return
	}
	defer func() { <-e.runSem }()
	e.executeCycle(ctx)
}

func (e *Engine) executeCycle(ctx context.Context) {
	if ctx.Err() != nil {
		return
	}

	e.ringMu.Lock()
	e.isRunning = true
	e.ringMu.Unlock()

	defer func() {
		e.ringMu.Lock()
		e.isRunning = false
		e.lastRunAt = time.Now()
		e.ringMu.Unlock()
	}()

	accounts, err := e.store.ListAccounts()
	if err != nil {
		slog.Error("list accounts failed", "err", err)
		return
	}
	instances, err := e.store.ListInstances()
	if err != nil {
		slog.Error("list instances failed", "err", err)
		return
	}

	// Build deduplicated credential map across enabled accounts and enabled instances.
	credMap := make(map[string]*credTarget)
	for _, acc := range accounts {
		if !acc.Enabled {
			continue
		}
		creds, _, err := e.store.GetAccountCredentials(acc.ID)
		if err != nil {
			continue
		}
		credMap[acc.KeyFingerprint] = &credTarget{
			Fingerprint:     acc.KeyFingerprint,
			Creds:           creds,
			Label:           acc.Name,
			DomesticQuotaGB: acc.DomesticQuotaGB,
			OverseasQuotaGB: acc.OverseasQuotaGB,
		}
	}

	instCreds := make(map[int64]aliyun.Credentials)
	for _, inst := range instances {
		if !inst.Enabled {
			continue
		}
		creds, err := e.store.ResolveInstanceCredentials(&inst)
		if err != nil {
			e.recordErrorOnce(ctx, &inst, fmt.Sprintf("cred:%d", inst.ID), "解析凭证失败: "+err.Error())
			continue
		}
		instCreds[inst.ID] = creds
		fp := inst.KeyFingerprint
		if _, exists := credMap[fp]; !exists {
			label := inst.Name
			if label == "" {
				label = inst.InstanceID
			}
			credMap[fp] = &credTarget{
				Fingerprint:     fp,
				Creds:           creds,
				Label:           "独立凭证 · " + label,
				DomesticQuotaGB: 20,
				OverseasQuotaGB: 180,
			}
		}
	}

	// 1. Fetch CDT traffic once per unique AccessKey.
	cdtResults := make(map[string]*aliyun.CDTSummary, len(credMap))
	cdtErrors := make(map[string]error, len(credMap))
	existingLatest, _ := e.store.ListCDTLatest()
	prevLatestMap := make(map[string]db.CDTLatest, len(existingLatest))
	for _, item := range existingLatest {
		prevLatestMap[item.KeyFingerprint] = item
	}

	now := time.Now()
	for fp, target := range credMap {
		summary, err := e.ali.ListCdtInternetTraffic(ctx, target.Creds)
		if err != nil {
			cdtErrors[fp] = err
			prev := prevLatestMap[fp]
			prev.KeyFingerprint = fp
			prev.AccessKeyMasked = crypto.MaskSecret(target.Creds.AccessKeyID)
			prev.Label = target.Label
			prev.DomesticQuotaGB = target.DomesticQuotaGB
			prev.OverseasQuotaGB = target.OverseasQuotaGB
			prev.LastError = err.Error()
			prev.UpdatedAt = now.Format(time.RFC3339)
			_ = e.store.UpsertCDTLatest(prev)
			e.recordErrorOnce(ctx, nil, "cdt:"+fp, fmt.Sprintf("[%s] 查询 CDT 流量失败: %v", target.Label, err))
			continue
		}

		e.clearError("cdt:" + fp)
		cdtResults[fp] = summary
		_ = e.store.UpsertCDTLatest(db.CDTLatest{
			KeyFingerprint:  fp,
			AccessKeyMasked: crypto.MaskSecret(target.Creds.AccessKeyID),
			Label:           target.Label,
			DomesticBytes:   summary.DomesticBytes,
			OverseasBytes:   summary.OverseasBytes,
			DomesticGB:      summary.DomesticGB,
			OverseasGB:      summary.OverseasGB,
			DomesticQuotaGB: target.DomesticQuotaGB,
			OverseasQuotaGB: target.OverseasQuotaGB,
			Details:         summary.Details,
			LastError:       "",
			UpdatedAt:       now.Format(time.RFC3339),
		})
		_ = e.store.InsertTrafficSnapshotIfNeeded(fp, summary.DomesticGB, summary.OverseasGB, now, 5*time.Minute)
	}

	// 2. Group enabled instances by (key_fingerprint, region_id) for batched DescribeInstances.
	type batchKey struct {
		fp     string
		region string
	}
	batches := make(map[batchKey][]db.Instance)
	for _, inst := range instances {
		if !inst.Enabled {
			continue
		}
		if _, ok := instCreds[inst.ID]; !ok {
			continue
		}
		bk := batchKey{fp: inst.KeyFingerprint, region: inst.RegionID}
		batches[bk] = append(batches[bk], inst)
	}

	for bk, group := range batches {
		creds := instCreds[group[0].ID]
		ids := make([]string, 0, len(group))
		for _, inst := range group {
			ids = append(ids, inst.InstanceID)
		}

		ecsList, err := e.ali.DescribeInstances(ctx, creds, bk.region, ids)
		if err != nil {
			for i := range group {
				inst := group[i]
				_ = e.store.UpdateInstanceTelemetry(inst.ID, db.InstanceTelemetryUpdate{
					Status:     inst.LastStatus,
					PublicIP:   inst.LastPublicIP,
					PrivateIP:  inst.LastPrivateIP,
					Spec:       inst.LastSpec,
					OS:         inst.LastOS,
					ChargeType: inst.LastChargeType,
					TrafficGB:  inst.LastTrafficGB,
					FlowType:   inst.LastFlowType,
					CheckedAt:  now.Format(time.RFC3339),
					LastError:  "查询实例失败: " + err.Error(),
				})
				e.recordErrorOnce(ctx, &inst, fmt.Sprintf("ecs:%d", inst.ID), fmt.Sprintf("查询实例 %s(%s) 失败: %v", inst.InstanceID, inst.RegionID, err))
			}
			continue
		}

		byID := make(map[string]aliyun.ECSInstanceInfo, len(ecsList))
		for _, info := range ecsList {
			byID[info.InstanceID] = info
		}

		summary := cdtResults[bk.fp]
		cdtErr := cdtErrors[bk.fp]

		for i := range group {
			inst := group[i]
			e.evaluateInstance(ctx, &inst, creds, byID, summary, cdtErr, now)
		}
	}

	// Periodic pruning (once per hour) to keep SQLite small.
	if time.Since(e.lastPrune) >= time.Hour {
		e.store.PruneOldData(30, 30, 2000)
		e.lastPrune = time.Now()
	}
}

func (e *Engine) evaluateInstance(
	ctx context.Context,
	inst *db.Instance,
	creds aliyun.Credentials,
	byID map[string]aliyun.ECSInstanceInfo,
	summary *aliyun.CDTSummary,
	cdtErr error,
	now time.Time,
) {
	flowType := aliyun.FlowTypeLabel(inst.RegionID)
	trafficGB := inst.LastTrafficGB
	if summary != nil {
		if aliyun.IsDomesticRegion(inst.RegionID) {
			trafficGB = summary.DomesticGB
		} else {
			trafficGB = summary.OverseasGB
		}
	}

	info, found := byID[inst.InstanceID]
	if !found {
		msg := fmt.Sprintf("未在地域 %s 找到实例 %s（可能已释放或 ID/凭证不匹配）", inst.RegionID, inst.InstanceID)
		_ = e.store.UpdateInstanceTelemetry(inst.ID, db.InstanceTelemetryUpdate{
			Status:     "NotFound",
			PublicIP:   inst.LastPublicIP,
			PrivateIP:  inst.LastPrivateIP,
			Spec:       inst.LastSpec,
			OS:         inst.LastOS,
			ChargeType: inst.LastChargeType,
			TrafficGB:  trafficGB,
			FlowType:   flowType,
			CheckedAt:  now.Format(time.RFC3339),
			LastError:  msg,
		})
		e.recordErrorOnce(ctx, inst, fmt.Sprintf("notfound:%d", inst.ID), msg)
		return
	}

	e.clearError(fmt.Sprintf("ecs:%d", inst.ID))
	e.clearError(fmt.Sprintf("notfound:%d", inst.ID))

	spec := fmt.Sprintf("%dC%.1fG · %s", info.CPU, float64(info.MemoryMB)/1024.0, info.InstanceType)
	lastErr := ""
	if cdtErr != nil {
		lastErr = "CDT 流量查询异常: " + cdtErr.Error()
	}
	_ = e.store.UpdateInstanceTelemetry(inst.ID, db.InstanceTelemetryUpdate{
		Status:     info.Status,
		PublicIP:   strings.Join(info.PublicIPs, ", "),
		PrivateIP:  strings.Join(info.PrivateIPs, ", "),
		Spec:       spec,
		OS:         info.OSName,
		ChargeType: info.ChargeType,
		TrafficGB:  trafficGB,
		FlowType:   flowType,
		CheckedAt:  now.Format(time.RFC3339),
		LastError:  lastErr,
	})

	if cdtErr != nil || summary == nil {
		e.pushRingLog(inst.ID, inst.InstanceID, inst.RegionID, "warn", "check", "CDT 流量查询失败，本轮跳过自动启停策略")
		return
	}

	currentMonth := now.Format("2006-01")
	warnedMonth := inst.WarnedMonth
	fusedMonth := inst.FusedMonth

	if inst.AutomationMode == "paused" {
		e.pushRingLog(inst.ID, inst.InstanceID, inst.RegionID, "info", "check",
			fmt.Sprintf("仅监控流量 · 状态 %s · %s流量 %.2f/%.2f GB", info.Status, flowType, trafficGB, inst.TrafficThresholdGB))
		return
	}

	// Skip automated actions while ECS is transitioning.
	if info.Status == "Starting" || info.Status == "Stopping" {
		e.pushRingLog(inst.ID, inst.InstanceID, inst.RegionID, "info", "check",
			fmt.Sprintf("实例处于过渡态 %s，等待稳定后操作", info.Status))
		return
	}

	// 1. Traffic threshold circuit breaker.
	if inst.TrafficThresholdGB > 0 && trafficGB >= inst.TrafficThresholdGB {
		if info.Status == "Running" {
			if err := e.ali.StopInstances(ctx, creds, inst.RegionID, []string{inst.InstanceID}, inst.ForceStop, inst.StoppedMode); err != nil {
				e.recordErrorOnce(ctx, inst, fmt.Sprintf("stop:%d", inst.ID), fmt.Sprintf("流量超限自动关机失败: %v", err))
				return
			}
			e.clearError(fmt.Sprintf("stop:%d", inst.ID))
			msg := fmt.Sprintf(
				"ECS实例 %s(%s) %s流量 %.2f GB ≥ 阈值 %.2f GB，已触发熔断关机",
				inst.InstanceID, inst.RegionID, flowType, trafficGB, inst.TrafficThresholdGB,
			)
			e.recordAudit(ctx, inst, "action", "fuse", msg, false)
			_ = e.notifier.Dispatch(ctx, notify.Event{
				Type:       "fuse",
				Title:      fmt.Sprintf("CDT 流量熔断关机 · %s", inst.RegionID),
				Body:       msg,
				InstanceID: inst.InstanceID,
				RegionID:   inst.RegionID,
				FlowType:   flowType,
				TrafficGB:  trafficGB,
				LimitGB:    inst.TrafficThresholdGB,
			})
			fusedMonth = currentMonth
			_ = e.store.MarkInstanceMonthFlags(inst.ID, warnedMonth, fusedMonth)
		} else {
			e.pushRingLog(inst.ID, inst.InstanceID, inst.RegionID, "warn", "check",
				fmt.Sprintf("%s流量 %.2f ≥ %.2f GB，保持关机 (%s)", flowType, trafficGB, inst.TrafficThresholdGB, info.Status))
		}
		return
	}

	// 2. Normal traffic zone.
	if inst.AutomationMode == "fuse_only" {
		e.pushRingLog(inst.ID, inst.InstanceID, inst.RegionID, "info", "check",
			fmt.Sprintf("仅超限关机 · 状态 %s · %s流量 %.2f/%.2f GB", info.Status, flowType, trafficGB, inst.TrafficThresholdGB))
		return
	}

	within, err := IsWithinSchedule(now, inst.EnableScheduleControl, inst.ScheduleStart, inst.ScheduleStop)
	if err != nil {
		e.recordErrorOnce(ctx, inst, fmt.Sprintf("sched:%d", inst.ID), "定时配置无效: "+err.Error())
		return
	}

	if within {
		if info.Status == "Stopped" {
			if err := e.ali.StartInstances(ctx, creds, inst.RegionID, []string{inst.InstanceID}); err != nil {
				e.recordErrorOnce(ctx, inst, fmt.Sprintf("start:%d", inst.ID), fmt.Sprintf("自动开机失败: %v", err))
				return
			}
			e.clearError(fmt.Sprintf("start:%d", inst.ID))
			msg := fmt.Sprintf(
				"ECS实例 %s(%s) 开机完成，当前%s流量: %.2f/%.2f GB",
				inst.InstanceID, inst.RegionID, flowType, trafficGB, inst.TrafficThresholdGB,
			)
			e.recordAudit(ctx, inst, "action", "start", msg, false)
			_ = e.notifier.Dispatch(ctx, notify.Event{
				Type:       "start",
				Title:      fmt.Sprintf("ECS 自动开机 · %s", inst.RegionID),
				Body:       msg,
				InstanceID: inst.InstanceID,
				RegionID:   inst.RegionID,
				FlowType:   flowType,
				TrafficGB:  trafficGB,
				LimitGB:    inst.TrafficThresholdGB,
			})
		} else {
			e.pushRingLog(inst.ID, inst.InstanceID, inst.RegionID, "info", "check",
				fmt.Sprintf("运行窗内保持运行 (%s) · %s流量 %.2f/%.2f GB", info.Status, flowType, trafficGB, inst.TrafficThresholdGB))
		}
		return
	}

	// Outside schedule window.
	if info.Status == "Running" {
		if err := e.ali.StopInstances(ctx, creds, inst.RegionID, []string{inst.InstanceID}, inst.ForceStop, inst.StoppedMode); err != nil {
			e.recordErrorOnce(ctx, inst, fmt.Sprintf("stop:%d", inst.ID), fmt.Sprintf("定时关机失败: %v", err))
			return
		}
		e.clearError(fmt.Sprintf("stop:%d", inst.ID))
		msg := fmt.Sprintf(
			"ECS实例 %s(%s) 已到计划关机时间并停止，当前%s流量: %.2f/%.2f GB",
			inst.InstanceID, inst.RegionID, flowType, trafficGB, inst.TrafficThresholdGB,
		)
		e.recordAudit(ctx, inst, "action", "stop", msg, false)
		_ = e.notifier.Dispatch(ctx, notify.Event{
			Type:       "stop",
			Title:      fmt.Sprintf("ECS 定时关机 · %s", inst.RegionID),
			Body:       msg,
			InstanceID: inst.InstanceID,
			RegionID:   inst.RegionID,
			FlowType:   flowType,
			TrafficGB:  trafficGB,
			LimitGB:    inst.TrafficThresholdGB,
		})
	} else {
		e.pushRingLog(inst.ID, inst.InstanceID, inst.RegionID, "info", "check",
			fmt.Sprintf("计划停机窗内保持关机 (%s) · %s流量 %.2f/%.2f GB", info.Status, flowType, trafficGB, inst.TrafficThresholdGB))
	}
}

// recordErrorOnce avoids flooding audit_logs and Bark notifications with identical repeated 60s errors.
func (e *Engine) recordErrorOnce(ctx context.Context, inst *db.Instance, key, msg string) {
	e.errMu.Lock()
	prev := e.lastErrBy[key]
	isNew := prev != msg
	e.lastErrBy[key] = msg
	e.errMu.Unlock()

	if isNew {
		e.recordAudit(ctx, inst, "error", "error", msg, true)
	} else {
		var refID int64
		var aliyunID, regionID string
		if inst != nil {
			refID = inst.ID
			aliyunID = inst.InstanceID
			regionID = inst.RegionID
		}
		e.pushRingLog(refID, aliyunID, regionID, "error", "error", msg)
	}
}

func (e *Engine) clearError(key string) {
	e.errMu.Lock()
	delete(e.lastErrBy, key)
	e.errMu.Unlock()
}

// RecordManualAction writes an immediate ring + audit entry for user-initiated actions from the UI.
func (e *Engine) RecordManualAction(inst *db.Instance, level, event, msg string) {
	var refID int64
	var aliyunID, regionID string
	if inst != nil {
		refID = inst.ID
		aliyunID = inst.InstanceID
		regionID = inst.RegionID
	}
	e.pushRingLog(refID, aliyunID, regionID, level, event, msg)
	_ = e.store.AddAuditLog(db.AuditLog{
		InstanceIDRef:    refID,
		AliyunInstanceID: aliyunID,
		RegionID:         regionID,
		Level:            level,
		Event:            event,
		Message:          msg,
	})
}

// ParseTimeMinutes parses an "H:MM" or "HH:MM" string into minutes since midnight.
func ParseTimeMinutes(raw string) (int, error) {
	parts := strings.Split(strings.TrimSpace(raw), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("时间格式需为 HH:MM: %q", raw)
	}
	h, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
	m, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err1 != nil || err2 != nil || h < 0 || h >= 24 || m < 0 || m >= 60 {
		return 0, fmt.Errorf("时间范围需在 00:00–23:59 之间: %q", raw)
	}
	return h*60 + m, nil
}

// IsWithinSchedule checks whether now falls inside the active schedule window, supporting overnight ranges.
func IsWithinSchedule(now time.Time, enabled bool, startStr, stopStr string) (bool, error) {
	if !enabled {
		return true, nil
	}
	startMin, err := ParseTimeMinutes(startStr)
	if err != nil {
		return false, err
	}
	stopMin, err := ParseTimeMinutes(stopStr)
	if err != nil {
		return false, err
	}
	if startMin == stopMin {
		return true, nil
	}
	curMin := now.Hour()*60 + now.Minute()
	if stopMin < startMin {
		return curMin >= startMin || curMin < stopMin, nil
	}
	return curMin >= startMin && curMin < stopMin, nil
}

func nextAlignedTick(now time.Time, intervalSec int) time.Time {
	if intervalSec < 10 {
		intervalSec = 10
	}
	unix := now.Unix()
	nextUnix := unix - (unix % int64(intervalSec)) + int64(intervalSec)
	return time.Unix(nextUnix, 0)
}
