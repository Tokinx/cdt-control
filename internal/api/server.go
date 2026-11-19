package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"ecs-cdt-panel/internal/aliyun"
	"ecs-cdt-panel/internal/auth"
	"ecs-cdt-panel/internal/config"
	"ecs-cdt-panel/internal/db"
	"ecs-cdt-panel/internal/notify"
	"ecs-cdt-panel/internal/scheduler"
)

// Server wires HTTP API handlers and embedded static assets.
type Server struct {
	ctx         context.Context
	cfg         *config.Config
	store       *db.Store
	ali         *aliyun.Client
	notifier    *notify.Sender
	engine      *scheduler.Engine
	authMgr     *auth.Manager
	staticFS    fs.FS
	assetHashes map[string]string
	indexHTML   []byte
}

// NewServer constructs the panel HTTP handler.
func NewServer(
	ctx context.Context,
	cfg *config.Config,
	store *db.Store,
	ali *aliyun.Client,
	notifier *notify.Sender,
	engine *scheduler.Engine,
	authMgr *auth.Manager,
	staticFS fs.FS,
) http.Handler {
	if ctx == nil {
		ctx = context.Background()
	}
	assetHashes, indexHTML := buildStaticManifest(staticFS)
	s := &Server{
		ctx:         ctx,
		cfg:         cfg,
		store:       store,
		ali:         ali,
		notifier:    notifier,
		engine:      engine,
		authMgr:     authMgr,
		staticFS:    staticFS,
		assetHashes: assetHashes,
		indexHTML:   indexHTML,
	}

	mux := http.NewServeMux()

	// Public / Auth endpoints
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("POST /api/auth/login", s.authMgr.HandleLogin)
	mux.HandleFunc("POST /api/auth/logout", s.authMgr.HandleLogout)
	mux.HandleFunc("GET /api/auth/me", s.authMgr.HandleMe)

	// Protected API endpoints
	mux.HandleFunc("GET /api/dashboard", s.authMgr.RequireAuth(s.handleDashboard))
	mux.HandleFunc("GET /api/snapshots", s.authMgr.RequireAuth(s.handleSnapshots))
	mux.HandleFunc("POST /api/scheduler/trigger", s.authMgr.RequireAuth(s.handleTriggerScheduler))
	mux.HandleFunc("POST /api/credentials/test", s.authMgr.RequireAuth(s.handleTestCredentials))

	// Accounts
	mux.HandleFunc("GET /api/accounts", s.authMgr.RequireAuth(s.handleListAccounts))
	mux.HandleFunc("POST /api/accounts", s.authMgr.RequireAuth(s.handleCreateAccount))
	mux.HandleFunc("PUT /api/accounts/{id}", s.authMgr.RequireAuth(s.handleUpdateAccount))
	mux.HandleFunc("DELETE /api/accounts/{id}", s.authMgr.RequireAuth(s.handleDeleteAccount))
	mux.HandleFunc("POST /api/accounts/{id}/test", s.authMgr.RequireAuth(s.handleTestAccount))
	mux.HandleFunc("GET /api/accounts/{id}/regions", s.authMgr.RequireAuth(s.handleAccountRegions))
	mux.HandleFunc("GET /api/accounts/{id}/discover", s.authMgr.RequireAuth(s.handleAccountDiscover))

	// Instances
	mux.HandleFunc("GET /api/instances", s.authMgr.RequireAuth(s.handleListInstances))
	mux.HandleFunc("POST /api/instances", s.authMgr.RequireAuth(s.handleCreateInstance))
	mux.HandleFunc("GET /api/instances/{id}", s.authMgr.RequireAuth(s.handleGetInstance))
	mux.HandleFunc("PUT /api/instances/{id}", s.authMgr.RequireAuth(s.handleUpdateInstance))
	mux.HandleFunc("DELETE /api/instances/{id}", s.authMgr.RequireAuth(s.handleDeleteInstance))
	mux.HandleFunc("POST /api/instances/{id}/start", s.authMgr.RequireAuth(s.handleStartInstance))
	mux.HandleFunc("POST /api/instances/{id}/stop", s.authMgr.RequireAuth(s.handleStopInstance))
	mux.HandleFunc("POST /api/instances/{id}/mode", s.authMgr.RequireAuth(s.handleSetInstanceMode))
	mux.HandleFunc("GET /api/instances/{id}/logs", s.authMgr.RequireAuth(s.handleInstanceLogs))
	mux.HandleFunc("DELETE /api/instances/{id}/logs", s.authMgr.RequireAuth(s.handleClearInstanceLogs))

	// Global logs & Settings
	mux.HandleFunc("GET /api/logs", s.authMgr.RequireAuth(s.handleGlobalLogs))
	mux.HandleFunc("DELETE /api/logs", s.authMgr.RequireAuth(s.handleClearGlobalLogs))
	mux.HandleFunc("GET /api/settings", s.authMgr.RequireAuth(s.handleGetSettings))
	mux.HandleFunc("PUT /api/settings/scheduler", s.authMgr.RequireAuth(s.handleUpdateScheduler))
	mux.HandleFunc("PUT /api/settings/notifications", s.authMgr.RequireAuth(s.handleUpdateNotifications))
	mux.HandleFunc("POST /api/settings/notifications/test", s.authMgr.RequireAuth(s.handleTestNotifications))

	// Static SPA assets
	fileServer := http.FileServer(http.FS(s.staticFS))
	mux.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "API 接口不存在"})
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.Header().Set("Cache-Control", "no-store")
			if len(s.indexHTML) > 0 {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if r.Method != http.MethodHead {
					_, _ = w.Write(s.indexHTML)
				}
				return
			}
			http.ServeFileFS(w, r, s.staticFS, "index.html")
			return
		}
		assetName := strings.TrimPrefix(path.Clean("/"+r.URL.Path), "/")
		if hash, ok := s.assetHashes[assetName]; ok {
			etag := `"` + hash + `"`
			w.Header().Set("ETag", etag)
			if r.URL.Query().Get("v") == hash {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-store")
			}
			if etagMatches(r.Header.Get("If-None-Match"), etag) {
				w.WriteHeader(http.StatusNotModified)
				return
			}
		} else {
			w.Header().Set("Cache-Control", "no-store")
		}
		fileServer.ServeHTTP(w, r)
	})

	return securityHeaders(mux)
}

func buildStaticManifest(staticFS fs.FS) (map[string]string, []byte) {
	hashes := make(map[string]string)
	if staticFS == nil {
		return hashes, nil
	}
	_ = fs.WalkDir(staticFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || p == "index.html" {
			return nil
		}
		data, readErr := fs.ReadFile(staticFS, p)
		if readErr != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		hashes[p] = hex.EncodeToString(sum[:6])
		return nil
	})
	rawIndex, err := fs.ReadFile(staticFS, "index.html")
	if err != nil {
		return hashes, nil
	}
	html := string(rawIndex)
	if cssHash := hashes["app.css"]; cssHash != "" {
		html = strings.ReplaceAll(html, `href="/app.css"`, `href="/app.css?v=`+cssHash+`"`)
	}
	if jsHash := hashes["app.js"]; jsHash != "" {
		html = strings.ReplaceAll(html, `src="/app.js"`, `src="/app.js?v=`+jsHash+`"`)
	}
	return hashes, []byte(html)
}

func etagMatches(headerVal, etag string) bool {
	if headerVal == "" {
		return false
	}
	for _, part := range strings.Split(headerVal, ",") {
		candidate := strings.TrimSpace(part)
		if candidate == "*" || candidate == etag || candidate == "W/"+etag {
			return true
		}
	}
	return false
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		next.ServeHTTP(w, r)
	})
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	cdtCards, err := s.store.ListCDTLatest()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if cdtCards == nil {
		cdtCards = []db.CDTLatest{}
	}
	instances, err := s.store.ListInstances()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if instances == nil {
		instances = []db.Instance{}
	}
	accounts, err := s.store.ListAccounts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"scheduler":      s.engine.Status(),
		"cdt_cards":      cdtCards,
		"instances":      instances,
		"accounts_count": len(accounts),
	})
}

func (s *Server) handleSnapshots(w http.ResponseWriter, r *http.Request) {
	fp := strings.TrimSpace(r.URL.Query().Get("fp"))
	if fp == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "缺少 fp 参数"})
		return
	}
	rangeParam := r.URL.Query().Get("range")
	now := time.Now()
	var since time.Time
	switch rangeParam {
	case "7d":
		since = now.AddDate(0, 0, -7)
	case "30d":
		since = now.AddDate(0, 0, -30)
	default:
		since = now.Add(-24 * time.Hour)
	}
	pts, err := s.store.QueryTrafficSnapshots(fp, since, 96)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if pts == nil {
		pts = []db.TrafficSnapshot{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"points": pts})
}

func (s *Server) handleTriggerScheduler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("sync") == "1" {
		checkCtx, cancel := context.WithTimeout(s.ctx, 8*time.Second)
		defer cancel()
		s.engine.RunNowSync(checkCtx)
		writeJSON(w, http.StatusOK, map[string]any{"triggered": true, "synced": true})
		return
	}
	started := s.engine.TriggerNow(s.ctx)
	writeJSON(w, http.StatusOK, map[string]any{"triggered": started})
}

func (s *Server) runImmediateCheck(deferCheck bool) {
	if deferCheck {
		return
	}
	checkCtx, cancel := context.WithTimeout(s.ctx, 6*time.Second)
	defer cancel()
	s.engine.RunNowSync(checkCtx)
}

func (s *Server) handleTestCredentials(w http.ResponseWriter, r *http.Request) {
	var body struct {
		AccountID       *int64 `json:"account_id"`
		AccessKeyID     string `json:"access_key_id"`
		AccessKeySecret string `json:"access_key_secret"`
		RegionID        string `json:"region_id"`
		InstanceID      string `json:"instance_id"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式无效"})
		return
	}

	var creds aliyun.Credentials
	if body.AccountID != nil && *body.AccountID > 0 {
		var err error
		creds, _, err = s.store.GetAccountCredentials(*body.AccountID)
		if err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
	} else {
		creds = aliyun.Credentials{
			AccessKeyID:     strings.TrimSpace(body.AccessKeyID),
			AccessKeySecret: strings.TrimSpace(body.AccessKeySecret),
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	cdtSummary, err := s.ali.ListCdtInternetTraffic(ctx, creds)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CDT 凭证验证失败: " + err.Error()})
		return
	}

	var ecsInfo *aliyun.ECSInstanceInfo
	if strings.TrimSpace(body.RegionID) != "" && strings.TrimSpace(body.InstanceID) != "" {
		list, err := s.ali.DescribeInstances(ctx, creds, strings.TrimSpace(body.RegionID), []string{strings.TrimSpace(body.InstanceID)})
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CDT 连通成功，但查询 ECS 实例失败: " + err.Error()})
			return
		}
		if len(list) == 0 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "CDT 连通成功，但未在该地域找到指定 ECS 实例"})
			return
		}
		ecsInfo = &list[0]
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"domestic_gb": cdtSummary.DomesticGB,
		"overseas_gb": cdtSummary.OverseasGB,
		"instance":    ecsInfo,
	})
}

func (s *Server) handleListAccounts(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListAccounts()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if items == nil {
		items = []db.Account{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": items})
}

type accountPayload struct {
	Name            string  `json:"name"`
	AccessKeyID     string  `json:"access_key_id"`
	AccessKeySecret string  `json:"access_key_secret"`
	DomesticQuotaGB float64 `json:"domestic_quota_gb"`
	OverseasQuotaGB float64 `json:"overseas_quota_gb"`
	Enabled         *bool   `json:"enabled"`
}

func (s *Server) handleCreateAccount(w http.ResponseWriter, r *http.Request) {
	var body accountPayload
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式无效"})
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	acc, err := s.store.CreateAccount(db.AccountInput{
		Name:            body.Name,
		AccessKeyID:     body.AccessKeyID,
		AccessKeySecret: body.AccessKeySecret,
		DomesticQuotaGB: body.DomesticQuotaGB,
		OverseasQuotaGB: body.OverseasQuotaGB,
		Enabled:         enabled,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.engine.RecordManualAction(nil, "info", "config", fmt.Sprintf("新增阿里云账号凭证: %s (%s)", acc.Name, acc.AccessKeyMasked))
	s.runImmediateCheck(r.URL.Query().Get("defer_check") == "1")
	writeJSON(w, http.StatusCreated, map[string]any{"account": acc})
}

func (s *Server) handleUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body accountPayload
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式无效"})
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	acc, err := s.store.UpdateAccount(id, db.AccountInput{
		Name:            body.Name,
		AccessKeyID:     body.AccessKeyID,
		AccessKeySecret: body.AccessKeySecret,
		DomesticQuotaGB: body.DomesticQuotaGB,
		OverseasQuotaGB: body.OverseasQuotaGB,
		Enabled:         enabled,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "账号不存在"})
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.engine.RecordManualAction(nil, "info", "config", fmt.Sprintf("更新阿里云账号凭证: %s (%s)", acc.Name, acc.AccessKeyMasked))
	s.runImmediateCheck(r.URL.Query().Get("defer_check") == "1")
	writeJSON(w, http.StatusOK, map[string]any{"account": acc})
}

func (s *Server) handleDeleteAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if err := s.store.DeleteAccount(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "账号不存在"})
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.engine.RecordManualAction(nil, "info", "config", fmt.Sprintf("删除阿里云账号凭证 #%d", id))
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleTestAccount(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	creds, acc, err := s.store.GetAccountCredentials(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	summary, err := s.ali.ListCdtInternetTraffic(ctx, creds)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":          true,
		"name":        acc.Name,
		"domestic_gb": summary.DomesticGB,
		"overseas_gb": summary.OverseasGB,
	})
}

func (s *Server) handleAccountRegions(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	creds, _, err := s.store.GetAccountCredentials(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	regions, err := s.ali.DescribeRegions(ctx, creds)
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"regions": regions})
}

func (s *Server) handleAccountDiscover(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	creds, _, err := s.store.GetAccountCredentials(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeErr(w, http.StatusNotFound, err)
		} else {
			writeErr(w, http.StatusInternalServerError, err)
		}
		return
	}
	regionID := strings.TrimSpace(r.URL.Query().Get("region_id"))
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()

	var list []aliyun.ECSInstanceInfo
	if regionID != "" {
		list, err = s.ali.DescribeInstances(ctx, creds, regionID, nil)
	} else {
		list, err = s.ali.DiscoverAllInstances(ctx, creds)
	}
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if list == nil {
		list = []aliyun.ECSInstanceInfo{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": list})
}

func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	items, err := s.store.ListInstances()
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if items == nil {
		items = []db.Instance{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"instances": items})
}

type instancePayload struct {
	Name                  string  `json:"name"`
	AccountID             *int64  `json:"account_id"`
	AccessKeyID           string  `json:"access_key_id"`
	AccessKeySecret       string  `json:"access_key_secret"`
	RegionID              string  `json:"region_id"`
	InstanceID            string  `json:"instance_id"`
	TrafficThresholdGB    float64 `json:"traffic_threshold_gb"`
	WarnPercent           int     `json:"warn_percent"`
	AutomationMode        string  `json:"automation_mode"`
	EnableScheduleControl bool    `json:"enable_schedule_control"`
	ScheduleStart         string  `json:"schedule_start"`
	ScheduleStop          string  `json:"schedule_stop"`
	ForceStop             bool    `json:"force_stop"`
	StoppedMode           string  `json:"stopped_mode"`
	Enabled               *bool   `json:"enabled"`
}

func validateScheduleInputs(enabled bool, startStr, stopStr string) error {
	if !enabled && strings.TrimSpace(startStr) == "" && strings.TrimSpace(stopStr) == "" {
		return nil
	}
	if strings.TrimSpace(startStr) != "" {
		if _, err := scheduler.ParseTimeMinutes(startStr); err != nil {
			return fmt.Errorf("开机时间无效: %w", err)
		}
	}
	if strings.TrimSpace(stopStr) != "" {
		if _, err := scheduler.ParseTimeMinutes(stopStr); err != nil {
			return fmt.Errorf("关机时间无效: %w", err)
		}
	}
	return nil
}

func (s *Server) handleCreateInstance(w http.ResponseWriter, r *http.Request) {
	var body instancePayload
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式无效"})
		return
	}
	if err := validateScheduleInputs(body.EnableScheduleControl, body.ScheduleStart, body.ScheduleStop); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	inst, err := s.store.CreateInstance(db.InstanceInput{
		Name:                  body.Name,
		AccountID:             body.AccountID,
		AccessKeyID:           body.AccessKeyID,
		AccessKeySecret:       body.AccessKeySecret,
		RegionID:              body.RegionID,
		InstanceID:            body.InstanceID,
		TrafficThresholdGB:    body.TrafficThresholdGB,
		WarnPercent:           body.WarnPercent,
		AutomationMode:        body.AutomationMode,
		EnableScheduleControl: body.EnableScheduleControl,
		ScheduleStart:         body.ScheduleStart,
		ScheduleStop:          body.ScheduleStop,
		ForceStop:             body.ForceStop,
		StoppedMode:           body.StoppedMode,
		Enabled:               enabled,
	})
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.engine.RecordManualAction(inst, "info", "config", fmt.Sprintf("添加监控实例 %s (%s)", inst.InstanceID, inst.RegionID))
	s.runImmediateCheck(r.URL.Query().Get("defer_check") == "1")
	inst, _ = s.store.GetInstance(inst.ID)
	writeJSON(w, http.StatusCreated, map[string]any{"instance": inst})
}

func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	inst, err := s.store.GetInstance(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "实例不存在"})
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}

	cdtCards, _ := s.store.ListCDTLatest()
	var matchedCDT *db.CDTLatest
	for i := range cdtCards {
		if cdtCards[i].KeyFingerprint == inst.KeyFingerprint {
			matchedCDT = &cdtCards[i]
			break
		}
	}

	snapshots, _ := s.store.QueryTrafficSnapshots(inst.KeyFingerprint, time.Now().AddDate(0, 0, -7), 96)
	if snapshots == nil {
		snapshots = []db.TrafficSnapshot{}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"instance":  inst,
		"cdt":       matchedCDT,
		"snapshots": snapshots,
	})
}

func (s *Server) handleUpdateInstance(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body instancePayload
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式无效"})
		return
	}
	if err := validateScheduleInputs(body.EnableScheduleControl, body.ScheduleStart, body.ScheduleStop); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	enabled := true
	if body.Enabled != nil {
		enabled = *body.Enabled
	}
	inst, err := s.store.UpdateInstance(id, db.InstanceInput{
		Name:                  body.Name,
		AccountID:             body.AccountID,
		AccessKeyID:           body.AccessKeyID,
		AccessKeySecret:       body.AccessKeySecret,
		RegionID:              body.RegionID,
		InstanceID:            body.InstanceID,
		TrafficThresholdGB:    body.TrafficThresholdGB,
		WarnPercent:           body.WarnPercent,
		AutomationMode:        body.AutomationMode,
		EnableScheduleControl: body.EnableScheduleControl,
		ScheduleStart:         body.ScheduleStart,
		ScheduleStop:          body.ScheduleStop,
		ForceStop:             body.ForceStop,
		StoppedMode:           body.StoppedMode,
		Enabled:               enabled,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "实例不存在"})
			return
		}
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.engine.RecordManualAction(inst, "info", "config", fmt.Sprintf("更新实例策略 %s (%s)", inst.InstanceID, inst.RegionID))
	s.runImmediateCheck(r.URL.Query().Get("defer_check") == "1")
	inst, _ = s.store.GetInstance(inst.ID)
	writeJSON(w, http.StatusOK, map[string]any{"instance": inst})
}

func (s *Server) handleDeleteInstance(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	inst, _ := s.store.GetInstance(id)
	if err := s.store.DeleteInstance(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "实例不存在"})
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if inst != nil {
		s.engine.RecordManualAction(nil, "info", "config", fmt.Sprintf("删除监控实例 %s (%s)", inst.InstanceID, inst.RegionID))
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleStartInstance(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	inst, err := s.store.GetInstance(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	creds, err := s.store.ResolveInstanceCredentials(inst)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.ali.StartInstances(ctx, creds, inst.RegionID, []string{inst.InstanceID}); err != nil {
		s.engine.RecordManualAction(inst, "error", "error", "手动开机失败: "+err.Error())
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	_ = s.store.UpdateInstanceTelemetry(inst.ID, db.InstanceTelemetryUpdate{
		Status:     "Starting",
		PublicIP:   inst.LastPublicIP,
		PrivateIP:  inst.LastPrivateIP,
		Spec:       inst.LastSpec,
		OS:         inst.LastOS,
		ChargeType: inst.LastChargeType,
		TrafficGB:  inst.LastTrafficGB,
		FlowType:   inst.LastFlowType,
		CheckedAt:  time.Now().Format(time.RFC3339),
		LastError:  "",
	})
	s.engine.RecordManualAction(inst, "action", "manual_start", fmt.Sprintf("用户手动触发开机: %s (%s)", inst.InstanceID, inst.RegionID))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "Starting"})
}

func (s *Server) handleStopInstance(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		PauseAutomation bool   `json:"pause_automation"`
		ForceStop       *bool  `json:"force_stop"`
		StoppedMode     string `json:"stopped_mode"`
	}
	_ = decodeJSON(w, r, &body)

	stoppedMode := strings.TrimSpace(body.StoppedMode)
	if stoppedMode != "" && stoppedMode != "KeepCharging" && stoppedMode != "StopCharging" {
		writeErr(w, http.StatusBadRequest, fmt.Errorf("不支持的停机模式: %s", stoppedMode))
		return
	}

	inst, err := s.store.GetInstance(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	creds, err := s.store.ResolveInstanceCredentials(inst)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}

	if stoppedMode == "" {
		stoppedMode = inst.StoppedMode
	}
	force := inst.ForceStop
	if body.ForceStop != nil {
		force = *body.ForceStop
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	if err := s.ali.StopInstances(ctx, creds, inst.RegionID, []string{inst.InstanceID}, force, stoppedMode); err != nil {
		s.engine.RecordManualAction(inst, "error", "error", "手动关机失败: "+err.Error())
		writeErr(w, http.StatusBadGateway, err)
		return
	}

	if body.PauseAutomation {
		_ = s.store.SetInstanceAutomationMode(inst.ID, "paused")
	}
	_ = s.store.UpdateInstanceTelemetry(inst.ID, db.InstanceTelemetryUpdate{
		Status:     "Stopping",
		PublicIP:   inst.LastPublicIP,
		PrivateIP:  inst.LastPrivateIP,
		Spec:       inst.LastSpec,
		OS:         inst.LastOS,
		ChargeType: inst.LastChargeType,
		TrafficGB:  inst.LastTrafficGB,
		FlowType:   inst.LastFlowType,
		CheckedAt:  time.Now().Format(time.RFC3339),
		LastError:  "",
	})

	msg := fmt.Sprintf("用户手动触发关机 (%s): %s (%s)", stoppedMode, inst.InstanceID, inst.RegionID)
	if body.PauseAutomation {
		msg += "（已同时切换为「仅监控流量」）"
	}
	s.engine.RecordManualAction(inst, "action", "manual_stop", msg)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "status": "Stopping", "stopped_mode": stoppedMode})
}

func (s *Server) handleSetInstanceMode(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	var body struct {
		AutomationMode string `json:"automation_mode"`
	}
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式无效"})
		return
	}
	inst, err := s.store.GetInstance(id)
	if err != nil {
		writeErr(w, http.StatusNotFound, err)
		return
	}
	if err := s.store.SetInstanceAutomationMode(id, body.AutomationMode); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	s.engine.RecordManualAction(inst, "info", "config", fmt.Sprintf("切换托管模式为 %s", body.AutomationMode))
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "automation_mode": body.AutomationMode})
}

func (s *Server) handleInstanceLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	level := r.URL.Query().Get("level")
	auditLogs, err := s.store.ListAuditLogs(id, level, 150)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if auditLogs == nil {
		auditLogs = []db.AuditLog{}
	}
	realtimeLogs := s.engine.GetRecentLogs(id, 100)
	if realtimeLogs == nil {
		realtimeLogs = []scheduler.RingLog{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"audit":    auditLogs,
		"realtime": realtimeLogs,
	})
}

func (s *Server) handleGlobalLogs(w http.ResponseWriter, r *http.Request) {
	level := r.URL.Query().Get("level")
	auditLogs, err := s.store.ListAuditLogs(0, level, 150)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	if auditLogs == nil {
		auditLogs = []db.AuditLog{}
	}
	realtimeLogs := s.engine.GetRecentLogs(0, 150)
	if realtimeLogs == nil {
		realtimeLogs = []scheduler.RingLog{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"audit":    auditLogs,
		"realtime": realtimeLogs,
	})
}

func (s *Server) handleClearInstanceLogs(w http.ResponseWriter, r *http.Request) {
	id, err := parseIDParam(r, "id")
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	if _, err := s.store.GetInstance(id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "实例不存在"})
			return
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.handleClearLogsScoped(w, r, id)
}

func (s *Server) handleClearGlobalLogs(w http.ResponseWriter, r *http.Request) {
	s.handleClearLogsScoped(w, r, 0)
}

func (s *Server) handleClearLogsScoped(w http.ResponseWriter, r *http.Request, instanceIDRef int64) {
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = "all"
	}
	if scope != "all" && scope != "realtime" && scope != "audit" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("不支持的日志清除范围: %s", scope)})
		return
	}

	level := strings.TrimSpace(r.URL.Query().Get("level"))
	if level == "" {
		level = "all"
	}
	switch level {
	case "all", "action", "warn", "error", "info":
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": fmt.Sprintf("不支持的日志级别: %s", level)})
		return
	}

	var clearedRealtime int
	var clearedAudit int64
	if scope == "realtime" || scope == "all" {
		clearedRealtime = s.engine.ClearRecentLogs(instanceIDRef, level)
	}
	if scope == "audit" || scope == "all" {
		var err error
		clearedAudit, err = s.store.ClearAuditLogs(instanceIDRef, level)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":               true,
		"scope":            scope,
		"level":            level,
		"cleared_realtime": clearedRealtime,
		"cleared_audit":    clearedAudit,
	})
}

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	schedCfg, err := s.store.GetSchedulerSettings(s.cfg.DefaultCheckInterval)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	notifyCfg, err := s.store.GetNotificationSettings(false)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"scheduler":     schedCfg,
		"notifications": notifyCfg,
	})
}

func (s *Server) handleUpdateScheduler(w http.ResponseWriter, r *http.Request) {
	var body db.SchedulerSettings
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式无效"})
		return
	}
	if err := s.store.SaveSchedulerSettings(body); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	s.engine.NotifyConfigChanged()
	s.engine.RecordManualAction(nil, "info", "config", fmt.Sprintf("更新巡检调度设置: 启用=%v, 间隔=%ds", body.Enabled, body.IntervalSeconds))
	writeJSON(w, http.StatusOK, map[string]any{"scheduler": body})
}

func (s *Server) handleUpdateNotifications(w http.ResponseWriter, r *http.Request) {
	var body db.NotificationSettings
	if err := decodeJSON(w, r, &body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "请求格式无效"})
		return
	}
	if err := s.store.SaveNotificationSettings(body); err != nil {
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	saved, _ := s.store.GetNotificationSettings(false)
	s.engine.RecordManualAction(nil, "info", "config", "更新通知渠道配置")
	writeJSON(w, http.StatusOK, map[string]any{"notifications": saved})
}

func (s *Server) handleTestNotifications(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	err := s.notifier.Dispatch(ctx, notify.Event{
		Type:  "test",
		Title: "ECS CDT 面板测试通知",
		Body:  fmt.Sprintf("通知通道配置正常（发送时间 %s）", time.Now().Format("15:04:05")),
	})
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func parseIDParam(r *http.Request, name string) (int64, error) {
	raw := r.PathValue(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, fmt.Errorf("无效 ID: %q", raw)
	}
	return id, nil
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	return json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(dst)
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]string{"error": err.Error()})
}
