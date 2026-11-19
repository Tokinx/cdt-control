package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"ecs-cdt-panel/internal/aliyun"
	"ecs-cdt-panel/internal/crypto"
)

// Account represents a saved Alibaba Cloud AccessKey credential profile.
type Account struct {
	ID              int64   `json:"id"`
	Name            string  `json:"name"`
	AccessKeyID     string  `json:"access_key_id"`
	AccessKeyMasked string  `json:"access_key_masked"`
	HasSecret       bool    `json:"has_secret"`
	KeyFingerprint  string  `json:"key_fingerprint"`
	DomesticQuotaGB float64 `json:"domestic_quota_gb"`
	OverseasQuotaGB float64 `json:"overseas_quota_gb"`
	Enabled         bool    `json:"enabled"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`

	accessKeySecretEnc string
}

// AccountInput carries create/update payload for an Alibaba Cloud account.
type AccountInput struct {
	Name            string
	AccessKeyID     string
	AccessKeySecret string // optional on update to keep existing secret
	DomesticQuotaGB float64
	OverseasQuotaGB float64
	Enabled         bool
}

// Instance represents a monitored ECS instance with CDT policy and optional standalone AccessKey.
type Instance struct {
	ID                    int64   `json:"id"`
	Name                  string  `json:"name"`
	AccountID             *int64  `json:"account_id"`
	AccountName           string  `json:"account_name,omitempty"`
	UseCustomKey          bool    `json:"use_custom_key"`
	AccessKeyID           string  `json:"access_key_id,omitempty"`
	AccessKeyMasked       string  `json:"access_key_masked"`
	HasSecret             bool    `json:"has_secret"`
	KeyFingerprint        string  `json:"key_fingerprint"`
	RegionID              string  `json:"region_id"`
	InstanceID            string  `json:"instance_id"`
	TrafficThresholdGB    float64 `json:"traffic_threshold_gb"`
	WarnPercent           int     `json:"warn_percent"`
	AutomationMode        string  `json:"automation_mode"` // full | fuse_only | paused
	EnableScheduleControl bool    `json:"enable_schedule_control"`
	ScheduleStart         string  `json:"schedule_start"`
	ScheduleStop          string  `json:"schedule_stop"`
	ForceStop             bool    `json:"force_stop"`
	StoppedMode           string  `json:"stopped_mode"` // KeepCharging | StopCharging
	Enabled               bool    `json:"enabled"`

	LastStatus     string  `json:"last_status"`
	LastPublicIP   string  `json:"last_public_ip"`
	LastPrivateIP  string  `json:"last_private_ip"`
	LastSpec       string  `json:"last_spec"`
	LastOS         string  `json:"last_os"`
	LastChargeType string  `json:"last_charge_type"`
	LastTrafficGB  float64 `json:"last_traffic_gb"`
	LastFlowType   string  `json:"last_flow_type"`
	LastCheckedAt  string  `json:"last_checked_at"`
	LastError      string  `json:"last_error"`
	WarnedMonth    string  `json:"warned_month"`
	FusedMonth     string  `json:"fused_month"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`

	accessKeySecretEnc string
}

// InstanceInput carries create/update fields for an Instance.
type InstanceInput struct {
	Name                  string
	AccountID             *int64
	AccessKeyID           string
	AccessKeySecret       string // optional on update to keep existing secret
	RegionID              string
	InstanceID            string
	TrafficThresholdGB    float64
	WarnPercent           int
	AutomationMode        string
	EnableScheduleControl bool
	ScheduleStart         string
	ScheduleStop          string
	ForceStop             bool
	StoppedMode           string
	Enabled               bool
}

// InstanceTelemetryUpdate captures runtime fields updated by each poll cycle.
type InstanceTelemetryUpdate struct {
	Status     string
	PublicIP   string
	PrivateIP  string
	Spec       string
	OS         string
	ChargeType string
	TrafficGB  float64
	FlowType   string
	CheckedAt  string
	LastError  string
}

// CDTLatest holds the latest CDT traffic summary for one AccessKey fingerprint.
type CDTLatest struct {
	KeyFingerprint  string                 `json:"key_fingerprint"`
	AccessKeyMasked string                 `json:"access_key_masked"`
	Label           string                 `json:"label"`
	DomesticBytes   int64                  `json:"domestic_bytes"`
	OverseasBytes   int64                  `json:"overseas_bytes"`
	DomesticGB      float64                `json:"domestic_gb"`
	OverseasGB      float64                `json:"overseas_gb"`
	DomesticQuotaGB float64                `json:"domestic_quota_gb"`
	OverseasQuotaGB float64                `json:"overseas_quota_gb"`
	Details         []aliyun.RegionTraffic `json:"details"`
	LastError       string                 `json:"last_error"`
	UpdatedAt       string                 `json:"updated_at"`
}

// TrafficSnapshot represents one historical traffic sample point.
type TrafficSnapshot struct {
	DomesticGB float64 `json:"domestic_gb"`
	OverseasGB float64 `json:"overseas_gb"`
	RecordedAt string  `json:"recorded_at"`
}

// AuditLog represents a persisted state change, action, warning, or error log.
type AuditLog struct {
	ID               int64  `json:"id"`
	InstanceIDRef    int64  `json:"instance_id_ref"`
	AliyunInstanceID string `json:"aliyun_instance_id"`
	RegionID         string `json:"region_id"`
	Level            string `json:"level"`
	Event            string `json:"event"`
	Message          string `json:"message"`
	CreatedAt        string `json:"created_at"`
}

// SchedulerSettings defines global polling loop settings.
type SchedulerSettings struct {
	IntervalSeconds int  `json:"interval_seconds"`
	Enabled         bool `json:"enabled"`
}

// NotificationSettings defines Bark and generic Webhook notification configuration.
type NotificationSettings struct {
	BarkEnabled  bool `json:"bark_enabled"`
	BarkEndpoint string `json:"bark_endpoint,omitempty"`
	BarkMasked   string `json:"bark_masked"`
	HasBark      bool   `json:"has_bark"`
	BarkGroup    string `json:"bark_group"`
	BarkSound    string `json:"bark_sound"`

	WebhookEnabled bool   `json:"webhook_enabled"`
	WebhookURL     string `json:"webhook_url,omitempty"`
	WebhookMasked  string `json:"webhook_masked"`
	HasWebhook     bool   `json:"has_webhook"`

	NotifyOnStart bool `json:"notify_on_start"`
	NotifyOnStop  bool `json:"notify_on_stop"`
	NotifyOnWarn  bool `json:"notify_on_warn"`
	NotifyOnFuse  bool `json:"notify_on_fuse"`
	NotifyOnError bool `json:"notify_on_error"`
}

type notificationPersisted struct {
	BarkEnabled     bool   `json:"bark_enabled"`
	BarkEndpointEnc string `json:"bark_endpoint_enc"`
	BarkGroup       string `json:"bark_group"`
	BarkSound       string `json:"bark_sound"`
	WebhookEnabled  bool   `json:"webhook_enabled"`
	WebhookURLEnc   string `json:"webhook_url_enc"`
	NotifyOnStart   bool   `json:"notify_on_start"`
	NotifyOnStop    bool   `json:"notify_on_stop"`
	NotifyOnWarn    bool   `json:"notify_on_warn"`
	NotifyOnFuse    bool   `json:"notify_on_fuse"`
	NotifyOnError   bool   `json:"notify_on_error"`
}

func nowStr() string {
	return time.Now().Format(time.RFC3339)
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ListAccounts returns all configured Aliyun accounts with secrets masked.
func (s *Store) ListAccounts() ([]Account, error) {
	rows, err := s.db.Query(`
		SELECT id, name, access_key_id, access_key_secret_enc, key_fingerprint,
		       domestic_quota_gb, overseas_quota_gb, enabled, created_at, updated_at
		FROM accounts
		ORDER BY id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Account
	for rows.Next() {
		var a Account
		var enabled int
		if err := rows.Scan(
			&a.ID, &a.Name, &a.AccessKeyID, &a.accessKeySecretEnc, &a.KeyFingerprint,
			&a.DomesticQuotaGB, &a.OverseasQuotaGB, &enabled, &a.CreatedAt, &a.UpdatedAt,
		); err != nil {
			return nil, err
		}
		a.Enabled = enabled == 1
		a.HasSecret = a.accessKeySecretEnc != ""
		a.AccessKeyMasked = crypto.MaskSecret(a.AccessKeyID)
		out = append(out, a)
	}
	return out, rows.Err()
}

// GetAccount fetches one account by ID.
func (s *Store) GetAccount(id int64) (*Account, error) {
	var a Account
	var enabled int
	err := s.db.QueryRow(`
		SELECT id, name, access_key_id, access_key_secret_enc, key_fingerprint,
		       domestic_quota_gb, overseas_quota_gb, enabled, created_at, updated_at
		FROM accounts WHERE id = ?
	`, id).Scan(
		&a.ID, &a.Name, &a.AccessKeyID, &a.accessKeySecretEnc, &a.KeyFingerprint,
		&a.DomesticQuotaGB, &a.OverseasQuotaGB, &enabled, &a.CreatedAt, &a.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	a.Enabled = enabled == 1
	a.HasSecret = a.accessKeySecretEnc != ""
	a.AccessKeyMasked = crypto.MaskSecret(a.AccessKeyID)
	return &a, nil
}

// GetAccountCredentials returns decrypted Aliyun credentials for an account ID.
func (s *Store) GetAccountCredentials(id int64) (aliyun.Credentials, *Account, error) {
	acc, err := s.GetAccount(id)
	if err != nil {
		return aliyun.Credentials{}, nil, err
	}
	secret, err := s.cipher.Decrypt(acc.accessKeySecretEnc)
	if err != nil {
		return aliyun.Credentials{}, nil, fmt.Errorf("decrypt account %d secret: %w", id, err)
	}
	return aliyun.Credentials{
		AccessKeyID:     acc.AccessKeyID,
		AccessKeySecret: secret,
	}, acc, nil
}

// CreateAccount creates a new Aliyun credential account.
func (s *Store) CreateAccount(in AccountInput) (*Account, error) {
	akID := strings.TrimSpace(in.AccessKeyID)
	akSecret := strings.TrimSpace(in.AccessKeySecret)
	if strings.TrimSpace(in.Name) == "" || akID == "" || akSecret == "" {
		return nil, errors.New("账号名称、AccessKey ID 和 AccessKey Secret 均不能为空")
	}
	encSecret, err := s.cipher.Encrypt(akSecret)
	if err != nil {
		return nil, err
	}
	if in.DomesticQuotaGB <= 0 {
		in.DomesticQuotaGB = 20
	}
	if in.OverseasQuotaGB <= 0 {
		in.OverseasQuotaGB = 180
	}

	now := nowStr()
	fp := crypto.KeyFingerprint(akID)
	res, err := s.db.Exec(`
		INSERT INTO accounts (name, access_key_id, access_key_secret_enc, key_fingerprint,
		                      domestic_quota_gb, overseas_quota_gb, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, strings.TrimSpace(in.Name), akID, encSecret, fp, in.DomesticQuotaGB, in.OverseasQuotaGB, boolToInt(in.Enabled), now, now)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetAccount(id)
}

// UpdateAccount updates an account and refreshes linked instances' fingerprints.
func (s *Store) UpdateAccount(id int64, in AccountInput) (*Account, error) {
	existing, err := s.GetAccount(id)
	if err != nil {
		return nil, err
	}
	akID := strings.TrimSpace(in.AccessKeyID)
	if strings.TrimSpace(in.Name) == "" || akID == "" {
		return nil, errors.New("账号名称和 AccessKey ID 不能为空")
	}

	encSecret := existing.accessKeySecretEnc
	if strings.TrimSpace(in.AccessKeySecret) != "" {
		encSecret, err = s.cipher.Encrypt(strings.TrimSpace(in.AccessKeySecret))
		if err != nil {
			return nil, err
		}
	}
	if in.DomesticQuotaGB <= 0 {
		in.DomesticQuotaGB = 20
	}
	if in.OverseasQuotaGB <= 0 {
		in.OverseasQuotaGB = 180
	}

	fp := crypto.KeyFingerprint(akID)
	now := nowStr()
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE accounts
		SET name = ?, access_key_id = ?, access_key_secret_enc = ?, key_fingerprint = ?,
		    domestic_quota_gb = ?, overseas_quota_gb = ?, enabled = ?, updated_at = ?
		WHERE id = ?
	`, strings.TrimSpace(in.Name), akID, encSecret, fp, in.DomesticQuotaGB, in.OverseasQuotaGB, boolToInt(in.Enabled), now, id)
	if err != nil {
		return nil, err
	}

	_, err = tx.Exec(`
		UPDATE instances
		SET key_fingerprint = ?, updated_at = ?
		WHERE account_id = ? AND access_key_id = ''
	`, fp, now, id)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	_ = s.CleanupOrphanedCDTLatest()
	return s.GetAccount(id)
}

// DeleteAccount removes an account; instances bound to it are rejected if still referenced unless detached.
func (s *Store) DeleteAccount(id int64) error {
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(1) FROM instances WHERE account_id = ?`, id).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("仍有 %d 个实例关联此账号，请先修改或删除关联实例", count)
	}
	res, err := s.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	_ = s.CleanupOrphanedCDTLatest()
	return nil
}

// ListInstances returns all configured instances joined with their linked account name.
func (s *Store) ListInstances() ([]Instance, error) {
	rows, err := s.db.Query(`
		SELECT i.id, i.name, i.account_id, COALESCE(a.name, ''), i.access_key_id, COALESCE(a.access_key_id, ''),
		       i.access_key_secret_enc, COALESCE(a.access_key_secret_enc, ''), i.key_fingerprint,
		       i.region_id, i.instance_id, i.traffic_threshold_gb, i.warn_percent, i.automation_mode,
		       i.enable_schedule_control, i.schedule_start, i.schedule_stop, i.force_stop, i.stopped_mode, i.enabled,
		       i.last_status, i.last_public_ip, i.last_private_ip, i.last_spec, i.last_os, i.last_charge_type,
		       i.last_traffic_gb, i.last_flow_type, i.last_checked_at, i.last_error, i.warned_month, i.fused_month,
		       i.created_at, i.updated_at
		FROM instances i
		LEFT JOIN accounts a ON a.id = i.account_id
		ORDER BY i.id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Instance
	for rows.Next() {
		inst, err := scanInstanceRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, inst)
	}
	return out, rows.Err()
}

// GetInstance returns a single instance by local ID.
func (s *Store) GetInstance(id int64) (*Instance, error) {
	row := s.db.QueryRow(`
		SELECT i.id, i.name, i.account_id, COALESCE(a.name, ''), i.access_key_id, COALESCE(a.access_key_id, ''),
		       i.access_key_secret_enc, COALESCE(a.access_key_secret_enc, ''), i.key_fingerprint,
		       i.region_id, i.instance_id, i.traffic_threshold_gb, i.warn_percent, i.automation_mode,
		       i.enable_schedule_control, i.schedule_start, i.schedule_stop, i.force_stop, i.stopped_mode, i.enabled,
		       i.last_status, i.last_public_ip, i.last_private_ip, i.last_spec, i.last_os, i.last_charge_type,
		       i.last_traffic_gb, i.last_flow_type, i.last_checked_at, i.last_error, i.warned_month, i.fused_month,
		       i.created_at, i.updated_at
		FROM instances i
		LEFT JOIN accounts a ON a.id = i.account_id
		WHERE i.id = ?
	`, id)
	inst, err := scanInstanceRow(row)
	if err != nil {
		return nil, err
	}
	return &inst, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanInstanceRow(scanner rowScanner) (Instance, error) {
	var inst Instance
	var accountID sql.NullInt64
	var accName, instAK, accAK, instSecEnc, accSecEnc string
	var schedCtrl, forceStop, enabled int

	err := scanner.Scan(
		&inst.ID, &inst.Name, &accountID, &accName, &instAK, &accAK,
		&instSecEnc, &accSecEnc, &inst.KeyFingerprint,
		&inst.RegionID, &inst.InstanceID, &inst.TrafficThresholdGB, &inst.WarnPercent, &inst.AutomationMode,
		&schedCtrl, &inst.ScheduleStart, &inst.ScheduleStop, &forceStop, &inst.StoppedMode, &enabled,
		&inst.LastStatus, &inst.LastPublicIP, &inst.LastPrivateIP, &inst.LastSpec, &inst.LastOS, &inst.LastChargeType,
		&inst.LastTrafficGB, &inst.LastFlowType, &inst.LastCheckedAt, &inst.LastError, &inst.WarnedMonth, &inst.FusedMonth,
		&inst.CreatedAt, &inst.UpdatedAt,
	)
	if err != nil {
		return Instance{}, err
	}
	if accountID.Valid {
		v := accountID.Int64
		inst.AccountID = &v
		inst.AccountName = accName
	}
	inst.EnableScheduleControl = schedCtrl == 1
	inst.ForceStop = forceStop == 1
	inst.Enabled = enabled == 1
	inst.UseCustomKey = instAK != ""

	effectiveAK := instAK
	effectiveSec := instSecEnc
	if !inst.UseCustomKey && accAK != "" {
		effectiveAK = accAK
		effectiveSec = accSecEnc
	}
	inst.AccessKeyID = effectiveAK
	inst.AccessKeyMasked = crypto.MaskSecret(effectiveAK)
	inst.HasSecret = effectiveSec != ""
	inst.accessKeySecretEnc = effectiveSec
	return inst, nil
}

// ResolveInstanceCredentials resolves effective Aliyun credentials for an instance.
func (s *Store) ResolveInstanceCredentials(inst *Instance) (aliyun.Credentials, error) {
	if inst.UseCustomKey {
		secret, err := s.cipher.Decrypt(inst.accessKeySecretEnc)
		if err != nil {
			return aliyun.Credentials{}, fmt.Errorf("decrypt instance %d secret: %w", inst.ID, err)
		}
		if inst.AccessKeyID == "" || secret == "" {
			return aliyun.Credentials{}, errors.New("实例独立 AccessKey 配置不完整")
		}
		return aliyun.Credentials{
			AccessKeyID:     inst.AccessKeyID,
			AccessKeySecret: secret,
		}, nil
	}
	if inst.AccountID != nil {
		creds, _, err := s.GetAccountCredentials(*inst.AccountID)
		return creds, err
	}
	return aliyun.Credentials{}, errors.New("实例未关联阿里云账号或独立 AccessKey")
}

// CreateInstance validates and persists a new ECS+CDT monitored instance.
func (s *Store) CreateInstance(in InstanceInput) (*Instance, error) {
	regionID := strings.TrimSpace(in.RegionID)
	instanceID := strings.TrimSpace(in.InstanceID)
	if regionID == "" || instanceID == "" {
		return nil, errors.New("地域 RegionId 和实例 InstanceId 不能为空")
	}

	if in.AccountID == nil || *in.AccountID <= 0 {
		return nil, errors.New("请先在「账号」中添加并选择关联的阿里云账号")
	}
	acc, err := s.GetAccount(*in.AccountID)
	if err != nil {
		return nil, fmt.Errorf("关联账号不存在: %w", err)
	}
	accountIDVal := acc.ID
	fp := acc.KeyFingerprint
	akID, encSecret := "", ""

	mode, stoppedMode, warnPct, err := normalizePolicyInput(&in)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = instanceID
	}

	now := nowStr()
	res, err := s.db.Exec(`
		INSERT INTO instances (
			name, account_id, access_key_id, access_key_secret_enc, key_fingerprint,
			region_id, instance_id, traffic_threshold_gb, warn_percent, automation_mode,
			enable_schedule_control, schedule_start, schedule_stop, force_stop, stopped_mode, enabled,
			created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		name, accountIDVal, akID, encSecret, fp,
		regionID, instanceID, in.TrafficThresholdGB, warnPct, mode,
		boolToInt(in.EnableScheduleControl), strings.TrimSpace(in.ScheduleStart), strings.TrimSpace(in.ScheduleStop),
		boolToInt(in.ForceStop), stoppedMode, boolToInt(in.Enabled),
		now, now,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return s.GetInstance(id)
}

// UpdateInstance updates an instance's credentials, policy, and switches.
func (s *Store) UpdateInstance(id int64, in InstanceInput) (*Instance, error) {
	if _, err := s.GetInstance(id); err != nil {
		return nil, err
	}

	regionID := strings.TrimSpace(in.RegionID)
	instanceID := strings.TrimSpace(in.InstanceID)
	if regionID == "" || instanceID == "" {
		return nil, errors.New("地域 RegionId 和实例 InstanceId 不能为空")
	}

	if in.AccountID == nil || *in.AccountID <= 0 {
		return nil, errors.New("请选择关联的阿里云账号")
	}
	acc, err := s.GetAccount(*in.AccountID)
	if err != nil {
		return nil, fmt.Errorf("关联账号不存在: %w", err)
	}
	accountIDVal := acc.ID
	fp := acc.KeyFingerprint
	akID, encSecret := "", ""

	mode, stoppedMode, warnPct, err := normalizePolicyInput(&in)
	if err != nil {
		return nil, err
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = instanceID
	}

	now := nowStr()
	_, err = s.db.Exec(`
		UPDATE instances
		SET name = ?, account_id = ?, access_key_id = ?, access_key_secret_enc = ?, key_fingerprint = ?,
		    region_id = ?, instance_id = ?, traffic_threshold_gb = ?, warn_percent = ?, automation_mode = ?,
		    enable_schedule_control = ?, schedule_start = ?, schedule_stop = ?, force_stop = ?, stopped_mode = ?, enabled = ?,
		    updated_at = ?
		WHERE id = ?
	`,
		name, accountIDVal, akID, encSecret, fp,
		regionID, instanceID, in.TrafficThresholdGB, warnPct, mode,
		boolToInt(in.EnableScheduleControl), strings.TrimSpace(in.ScheduleStart), strings.TrimSpace(in.ScheduleStop),
		boolToInt(in.ForceStop), stoppedMode, boolToInt(in.Enabled),
		now, id,
	)
	if err != nil {
		return nil, err
	}
	_ = s.CleanupOrphanedCDTLatest()
	return s.GetInstance(id)
}

func normalizePolicyInput(in *InstanceInput) (mode string, stoppedMode string, warnPct int, err error) {
	if in.TrafficThresholdGB < 0 {
		return "", "", 0, errors.New("流量阈值不能为负数")
	}
	warnPct = in.WarnPercent
	if warnPct < 0 || warnPct > 99 {
		return "", "", 0, errors.New("预警百分比需在 0–99 之间")
	}
	switch in.AutomationMode {
	case "", "full":
		mode = "full"
	case "fuse_only", "paused":
		mode = in.AutomationMode
	default:
		return "", "", 0, fmt.Errorf("不支持的自动化模式: %s", in.AutomationMode)
	}
	switch in.StoppedMode {
	case "", "StopCharging":
		stoppedMode = "StopCharging"
	case "KeepCharging":
		stoppedMode = "KeepCharging"
	default:
		return "", "", 0, fmt.Errorf("不支持的停机计费模式: %s", in.StoppedMode)
	}
	if strings.TrimSpace(in.ScheduleStart) == "" {
		in.ScheduleStart = "08:00"
	}
	if strings.TrimSpace(in.ScheduleStop) == "" {
		in.ScheduleStop = "00:00"
	}
	return mode, stoppedMode, warnPct, nil
}

// SetInstanceAutomationMode updates only automation_mode (e.g. quick Pause/Resume toggle).
func (s *Store) SetInstanceAutomationMode(id int64, mode string) error {
	if mode != "full" && mode != "fuse_only" && mode != "paused" {
		return fmt.Errorf("无效自动化模式: %s", mode)
	}
	res, err := s.db.Exec(`UPDATE instances SET automation_mode = ?, updated_at = ? WHERE id = ?`, mode, nowStr(), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteInstance removes an instance and cleans orphaned CDT cache if unused.
func (s *Store) DeleteInstance(id int64) error {
	res, err := s.db.Exec(`DELETE FROM instances WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	_ = s.CleanupOrphanedCDTLatest()
	return nil
}

// UpdateInstanceTelemetry persists latest queried ECS metadata, traffic, and error state.
func (s *Store) UpdateInstanceTelemetry(id int64, u InstanceTelemetryUpdate) error {
	_, err := s.db.Exec(`
		UPDATE instances
		SET last_status = ?, last_public_ip = ?, last_private_ip = ?, last_spec = ?,
		    last_os = ?, last_charge_type = ?, last_traffic_gb = ?, last_flow_type = ?,
		    last_checked_at = ?, last_error = ?
		WHERE id = ?
	`,
		u.Status, u.PublicIP, u.PrivateIP, u.Spec,
		u.OS, u.ChargeType, u.TrafficGB, u.FlowType,
		u.CheckedAt, u.LastError, id,
	)
	return err
}

// MarkInstanceMonthFlags records monthly warning/fuse notification markers so alerts don't repeat every minute.
func (s *Store) MarkInstanceMonthFlags(id int64, warnedMonth, fusedMonth string) error {
	_, err := s.db.Exec(`UPDATE instances SET warned_month = ?, fused_month = ? WHERE id = ?`, warnedMonth, fusedMonth, id)
	return err
}

// UpsertCDTLatest updates the current CDT summary row for one credential fingerprint.
func (s *Store) UpsertCDTLatest(item CDTLatest) error {
	detailsBytes, _ := json.Marshal(item.Details)
	_, err := s.db.Exec(`
		INSERT INTO cdt_latest (
			key_fingerprint, access_key_masked, label, domestic_bytes, overseas_bytes,
			domestic_gb, overseas_gb, domestic_quota_gb, overseas_quota_gb,
			details_json, last_error, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(key_fingerprint) DO UPDATE SET
			access_key_masked = excluded.access_key_masked,
			label = excluded.label,
			domestic_bytes = excluded.domestic_bytes,
			overseas_bytes = excluded.overseas_bytes,
			domestic_gb = excluded.domestic_gb,
			overseas_gb = excluded.overseas_gb,
			domestic_quota_gb = excluded.domestic_quota_gb,
			overseas_quota_gb = excluded.overseas_quota_gb,
			details_json = excluded.details_json,
			last_error = excluded.last_error,
			updated_at = excluded.updated_at
	`,
		item.KeyFingerprint, item.AccessKeyMasked, item.Label, item.DomesticBytes, item.OverseasBytes,
		item.DomesticGB, item.OverseasGB, item.DomesticQuotaGB, item.OverseasQuotaGB,
		string(detailsBytes), item.LastError, item.UpdatedAt,
	)
	return err
}

// ListCDTLatest returns latest CDT traffic summaries for all active credentials.
func (s *Store) ListCDTLatest() ([]CDTLatest, error) {
	rows, err := s.db.Query(`
		SELECT key_fingerprint, access_key_masked, label, domestic_bytes, overseas_bytes,
		       domestic_gb, overseas_gb, domestic_quota_gb, overseas_quota_gb,
		       details_json, last_error, updated_at
		FROM cdt_latest
		ORDER BY label ASC, key_fingerprint ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []CDTLatest
	for rows.Next() {
		var item CDTLatest
		var detailsJSON string
		if err := rows.Scan(
			&item.KeyFingerprint, &item.AccessKeyMasked, &item.Label,
			&item.DomesticBytes, &item.OverseasBytes,
			&item.DomesticGB, &item.OverseasGB,
			&item.DomesticQuotaGB, &item.OverseasQuotaGB,
			&detailsJSON, &item.LastError, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(detailsJSON), &item.Details)
		if item.Details == nil {
			item.Details = []aliyun.RegionTraffic{}
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

// CleanupOrphanedCDTLatest removes cached CDT cards no longer tied to any account or instance.
func (s *Store) CleanupOrphanedCDTLatest() error {
	_, err := s.db.Exec(`
		DELETE FROM cdt_latest
		WHERE key_fingerprint NOT IN (
			SELECT key_fingerprint FROM accounts WHERE key_fingerprint != ''
			UNION
			SELECT key_fingerprint FROM instances WHERE key_fingerprint != ''
		)
	`)
	return err
}

// InsertTrafficSnapshotIfNeeded records a traffic sample at most once per minInterval unless traffic moved >= 0.01GB.
func (s *Store) InsertTrafficSnapshotIfNeeded(fp string, domesticGB, overseasGB float64, now time.Time, minInterval time.Duration) error {
	var lastDom, lastOvs float64
	var lastTimeStr string
	err := s.db.QueryRow(`
		SELECT domestic_gb, overseas_gb, recorded_at
		FROM traffic_snapshots
		WHERE key_fingerprint = ?
		ORDER BY id DESC LIMIT 1
	`, fp).Scan(&lastDom, &lastOvs, &lastTimeStr)

	if err == nil {
		lastTime, parseErr := time.Parse(time.RFC3339, lastTimeStr)
		delta := absFloat(domesticGB-lastDom) + absFloat(overseasGB-lastOvs)
		if parseErr == nil && now.Sub(lastTime) < minInterval && delta < 0.05 {
			return nil
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	_, err = s.db.Exec(`
		INSERT INTO traffic_snapshots (key_fingerprint, domestic_gb, overseas_gb, recorded_at)
		VALUES (?, ?, ?, ?)
	`, fp, domesticGB, overseasGB, now.Format(time.RFC3339))
	return err
}

// QueryTrafficSnapshots fetches traffic snapshots for a key fingerprint since a given time.
func (s *Store) QueryTrafficSnapshots(fp string, since time.Time, maxPoints int) ([]TrafficSnapshot, error) {
	if maxPoints <= 0 {
		maxPoints = 120
	}
	rows, err := s.db.Query(`
		SELECT domestic_gb, overseas_gb, recorded_at
		FROM traffic_snapshots
		WHERE key_fingerprint = ? AND recorded_at >= ?
		ORDER BY id ASC
	`, fp, since.Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var all []TrafficSnapshot
	for rows.Next() {
		var pt TrafficSnapshot
		if err := rows.Scan(&pt.DomesticGB, &pt.OverseasGB, &pt.RecordedAt); err != nil {
			return nil, err
		}
		all = append(all, pt)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(all) <= maxPoints {
		return all, nil
	}
	// Uniform downsampling keeping first and latest sample.
	sampled := make([]TrafficSnapshot, 0, maxPoints)
	step := float64(len(all)-1) / float64(maxPoints-1)
	for i := 0; i < maxPoints-1; i++ {
		idx := int(float64(i) * step)
		sampled = append(sampled, all[idx])
	}
	sampled = append(sampled, all[len(all)-1])
	return sampled, nil
}

// PruneOldData enforces retention caps on snapshots and audit logs so SQLite stays tiny.
func (s *Store) PruneOldData(snapshotKeepDays, logKeepDays, maxLogs int) {
	now := time.Now()
	if snapshotKeepDays > 0 {
		cutoff := now.AddDate(0, 0, -snapshotKeepDays).Format(time.RFC3339)
		_, _ = s.db.Exec(`DELETE FROM traffic_snapshots WHERE recorded_at < ?`, cutoff)
	}
	if logKeepDays > 0 {
		cutoff := now.AddDate(0, 0, -logKeepDays).Format(time.RFC3339)
		_, _ = s.db.Exec(`DELETE FROM audit_logs WHERE created_at < ?`, cutoff)
	}
	if maxLogs > 0 {
		_, _ = s.db.Exec(`
			DELETE FROM audit_logs
			WHERE id NOT IN (SELECT id FROM audit_logs ORDER BY id DESC LIMIT ?)
		`, maxLogs)
	}
}

// AddAuditLog inserts an audit log row.
func (s *Store) AddAuditLog(entry AuditLog) error {
	if entry.CreatedAt == "" {
		entry.CreatedAt = nowStr()
	}
	_, err := s.db.Exec(`
		INSERT INTO audit_logs (instance_id_ref, aliyun_instance_id, region_id, level, event, message, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, entry.InstanceIDRef, entry.AliyunInstanceID, entry.RegionID, entry.Level, entry.Event, entry.Message, entry.CreatedAt)
	return err
}

func buildAuditLogWhere(instanceIDRef int64, level string) (string, []any) {
	var conds []string
	var args []any
	if instanceIDRef > 0 {
		conds = append(conds, "instance_id_ref = ?")
		args = append(args, instanceIDRef)
	}
	if level != "" && level != "all" {
		conds = append(conds, "level = ?")
		args = append(args, level)
	}
	if len(conds) == 0 {
		return "", nil
	}
	return "WHERE " + strings.Join(conds, " AND "), args
}

// ListAuditLogs retrieves persisted audit logs filtered optionally by instanceIDRef and level.
func (s *Store) ListAuditLogs(instanceIDRef int64, level string, limit int) ([]AuditLog, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	where, args := buildAuditLogWhere(instanceIDRef, level)
	args = append(args, limit)

	query := fmt.Sprintf(`
		SELECT id, instance_id_ref, aliyun_instance_id, region_id, level, event, message, created_at
		FROM audit_logs
		%s
		ORDER BY id DESC
		LIMIT ?
	`, where)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []AuditLog
	for rows.Next() {
		var l AuditLog
		if err := rows.Scan(&l.ID, &l.InstanceIDRef, &l.AliyunInstanceID, &l.RegionID, &l.Level, &l.Event, &l.Message, &l.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// ClearAuditLogs deletes persisted audit logs filtered optionally by instanceIDRef and level.
func (s *Store) ClearAuditLogs(instanceIDRef int64, level string) (int64, error) {
	where, args := buildAuditLogWhere(instanceIDRef, level)
	query := "DELETE FROM audit_logs"
	if where != "" {
		query += " " + where
	}
	res, err := s.db.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// GetSchedulerSettings returns persisted scheduler config or defaults.
func (s *Store) GetSchedulerSettings(defaultInterval int) (SchedulerSettings, error) {
	cfg := SchedulerSettings{
		IntervalSeconds: defaultInterval,
		Enabled:         true,
	}
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'scheduler'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return cfg, err
	}
	if cfg.IntervalSeconds < 10 {
		cfg.IntervalSeconds = 10
	}
	return cfg, nil
}

// SaveSchedulerSettings persists scheduler interval and enable toggle.
func (s *Store) SaveSchedulerSettings(cfg SchedulerSettings) error {
	if cfg.IntervalSeconds < 10 {
		cfg.IntervalSeconds = 10
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO settings (key, value) VALUES ('scheduler', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, string(raw))
	return err
}

// GetNotificationSettings loads notification config; includeSecrets decrypts URLs for backend sending.
func (s *Store) GetNotificationSettings(includeSecrets bool) (NotificationSettings, error) {
	out := NotificationSettings{
		BarkGroup:     "DevOps",
		NotifyOnStart: true,
		NotifyOnStop:  true,
		NotifyOnWarn:  true,
		NotifyOnFuse:  true,
		NotifyOnError: true,
	}
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = 'notifications'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	var p notificationPersisted
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		return out, err
	}

	barkPlain, _ := s.cipher.Decrypt(p.BarkEndpointEnc)
	webhookPlain, _ := s.cipher.Decrypt(p.WebhookURLEnc)

	out.BarkEnabled = p.BarkEnabled
	out.HasBark = barkPlain != ""
	out.BarkMasked = maskURL(barkPlain)
	out.BarkGroup = p.BarkGroup
	out.BarkSound = p.BarkSound

	out.WebhookEnabled = p.WebhookEnabled
	out.HasWebhook = webhookPlain != ""
	out.WebhookMasked = maskURL(webhookPlain)

	out.NotifyOnStart = p.NotifyOnStart
	out.NotifyOnStop = p.NotifyOnStop
	out.NotifyOnWarn = p.NotifyOnWarn
	out.NotifyOnFuse = p.NotifyOnFuse
	out.NotifyOnError = p.NotifyOnError

	if includeSecrets {
		out.BarkEndpoint = barkPlain
		out.WebhookURL = webhookPlain
	}
	return out, nil
}

// SaveNotificationSettings persists encrypted notification settings, preserving existing URLs if omitted.
func (s *Store) SaveNotificationSettings(in NotificationSettings) error {
	existing, err := s.GetNotificationSettings(true)
	if err != nil {
		return err
	}

	barkEndpoint := strings.TrimSpace(in.BarkEndpoint)
	if barkEndpoint == "" && in.HasBark {
		barkEndpoint = existing.BarkEndpoint
	}
	webhookURL := strings.TrimSpace(in.WebhookURL)
	if webhookURL == "" && in.HasWebhook {
		webhookURL = existing.WebhookURL
	}

	barkEnc, err := s.cipher.Encrypt(barkEndpoint)
	if err != nil {
		return err
	}
	webhookEnc, err := s.cipher.Encrypt(webhookURL)
	if err != nil {
		return err
	}

	p := notificationPersisted{
		BarkEnabled:     in.BarkEnabled,
		BarkEndpointEnc: barkEnc,
		BarkGroup:       strings.TrimSpace(in.BarkGroup),
		BarkSound:       strings.TrimSpace(in.BarkSound),
		WebhookEnabled:  in.WebhookEnabled,
		WebhookURLEnc:   webhookEnc,
		NotifyOnStart:   in.NotifyOnStart,
		NotifyOnStop:    in.NotifyOnStop,
		NotifyOnWarn:    in.NotifyOnWarn,
		NotifyOnFuse:    in.NotifyOnFuse,
		NotifyOnError:   in.NotifyOnError,
	}
	if p.BarkGroup == "" {
		p.BarkGroup = "DevOps"
	}

	raw, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO settings (key, value) VALUES ('notifications', ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, string(raw))
	return err
}

func maskURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if len(raw) <= 18 {
		return "****"
	}
	return raw[:14] + "****" + raw[len(raw)-4:]
}

func absFloat(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
