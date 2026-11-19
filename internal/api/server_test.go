package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"ecs-cdt-panel/internal/aliyun"
	"ecs-cdt-panel/internal/auth"
	"ecs-cdt-panel/internal/config"
	"ecs-cdt-panel/internal/crypto"
	"ecs-cdt-panel/internal/db"
	"ecs-cdt-panel/internal/notify"
	"ecs-cdt-panel/internal/scheduler"
	"ecs-cdt-panel/web"
)

func TestAPIWorkflow(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Port:                 "8080",
		DataDir:              dir,
		PanelUsername:        "admin",
		PanelPassword:        "pass1234",
		SessionSecret:        "test-session-secret-0123456789abcdef",
		EncryptionKeyRaw:     "test-enc-key-0123456789abcdef",
		DefaultCheckInterval: 60,
	}
	cipher := crypto.NewCipher(cfg.EncryptionKeyRaw)
	store, err := db.Open(dir, cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	staticFS, err := web.StaticFS()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	aliClient := aliyun.NewClient()
	notifier := notify.NewSender(store)
	engine := scheduler.NewEngine(store, aliClient, notifier, cfg.DefaultCheckInterval)
	defer engine.Wait()
	defer cancel()
	authMgr := auth.NewManager(cfg)
	handler := NewServer(ctx, cfg, store, aliClient, notifier, engine, authMgr, staticFS)

	// 1. Health endpoint works unauthenticated
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("health expected 200, got %d", rec.Code)
	}

	// 2. Dashboard requires auth
	req = httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("dashboard unauth expected 401, got %d", rec.Code)
	}

	// 3. Login and obtain session cookie
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"pass1234"}`))
	loginReq.RemoteAddr = "127.0.0.1:50000"
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("login expected 200, got %d", loginRec.Code)
	}
	cookies := loginRec.Result().Cookies()

	// 4. Create account first, then create instance linked to account
	accBody := `{"name":"测试账号","access_key_id":"LTAI5tTestKey0001","access_key_secret":"Secret0001"}`
	accReq := httptest.NewRequest(http.MethodPost, "/api/accounts?defer_check=1", bytes.NewBufferString(accBody))
	accReq.AddCookie(cookies[0])
	accRec := httptest.NewRecorder()
	handler.ServeHTTP(accRec, accReq)
	if accRec.Code != http.StatusCreated {
		t.Fatalf("create account expected 201, got %d: %s", accRec.Code, accRec.Body.String())
	}

	createBody := `{
		"name": "测试HK",
		"account_id": 1,
		"region_id": "cn-hongkong",
		"instance_id": "i-test001",
		"traffic_threshold_gb": 190,
		"automation_mode": "full",
		"enable_schedule_control": true,
		"schedule_start": "08:00",
		"schedule_stop": "00:00",
		"enabled": true
	}`
	instReq := httptest.NewRequest(http.MethodPost, "/api/instances?defer_check=1", bytes.NewBufferString(createBody))
	instReq.AddCookie(cookies[0])
	instRec := httptest.NewRecorder()
	handler.ServeHTTP(instRec, instReq)
	if instRec.Code != http.StatusCreated {
		t.Fatalf("create instance expected 201, got %d: %s", instRec.Code, instRec.Body.String())
	}

	var created struct {
		Instance db.Instance `json:"instance"`
	}
	if err := json.Unmarshal(instRec.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	// 5. Fetch instance logs
	logReq := httptest.NewRequest(http.MethodGet, "/api/instances/1/logs?level=all", nil)
	logReq.AddCookie(cookies[0])
	logRec := httptest.NewRecorder()
	handler.ServeHTTP(logRec, logReq)
	if logRec.Code != http.StatusOK {
		t.Fatalf("instance logs expected 200, got %d", logRec.Code)
	}
	if created.Instance.ID != 1 || created.Instance.StoppedMode != "StopCharging" {
		t.Fatalf("unexpected created instance: %+v", created.Instance)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return fn(r)
}

func TestStopInstanceStoppedMode(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Port:                 "8080",
		DataDir:              dir,
		PanelUsername:        "admin",
		PanelPassword:        "pass1234",
		SessionSecret:        "test-session-secret-0123456789abcdef",
		EncryptionKeyRaw:     "test-enc-key-0123456789abcdef",
		DefaultCheckInterval: 60,
	}
	cipher := crypto.NewCipher(cfg.EncryptionKeyRaw)
	store, err := db.Open(dir, cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	staticFS, err := web.StaticFS()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	var seenModes []string
	aliClient := aliyun.NewClient()
	aliClient.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			raw, _ := io.ReadAll(r.Body)
			vals, _ := url.ParseQuery(string(raw))
			seenModes = append(seenModes, vals.Get("StoppedMode"))
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"RequestId":"req-stop"}`)),
			}, nil
		}),
	})

	notifier := notify.NewSender(store)
	engine := scheduler.NewEngine(store, aliClient, notifier, cfg.DefaultCheckInterval)
	defer engine.Wait()
	defer cancel()
	authMgr := auth.NewManager(cfg)
	handler := NewServer(ctx, cfg, store, aliClient, notifier, engine, authMgr, staticFS)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"pass1234"}`))
	loginReq.RemoteAddr = "127.0.0.1:50001"
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	cookies := loginRec.Result().Cookies()

	accReq := httptest.NewRequest(http.MethodPost, "/api/accounts?defer_check=1", bytes.NewBufferString(`{"name":"主账号","access_key_id":"LTAI5tTestKey0001","access_key_secret":"Secret0001"}`))
	accReq.AddCookie(cookies[0])
	accRec := httptest.NewRecorder()
	handler.ServeHTTP(accRec, accReq)
	if accRec.Code != http.StatusCreated {
		t.Fatalf("create account expected 201, got %d", accRec.Code)
	}

	instReq := httptest.NewRequest(http.MethodPost, "/api/instances?defer_check=1", bytes.NewBufferString(`{"name":"HK","account_id":1,"region_id":"cn-hongkong","instance_id":"i-stop01","traffic_threshold_gb":190,"automation_mode":"paused","stopped_mode":"StopCharging","enabled":true}`))
	instReq.AddCookie(cookies[0])
	instRec := httptest.NewRecorder()
	handler.ServeHTTP(instRec, instReq)
	if instRec.Code != http.StatusCreated {
		t.Fatalf("create instance expected 201, got %d", instRec.Code)
	}

	for _, tc := range []struct {
		body     string
		wantCode int
		wantMode string
	}{
		{body: `{}`, wantCode: http.StatusOK, wantMode: "StopCharging"},
		{body: `{"stopped_mode":"KeepCharging"}`, wantCode: http.StatusOK, wantMode: "KeepCharging"},
		{body: `{"stopped_mode":"StopCharging"}`, wantCode: http.StatusOK, wantMode: "StopCharging"},
		{body: `{"stopped_mode":"InvalidMode"}`, wantCode: http.StatusBadRequest, wantMode: ""},
	} {
		req := httptest.NewRequest(http.MethodPost, "/api/instances/1/stop", bytes.NewBufferString(tc.body))
		req.AddCookie(cookies[0])
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != tc.wantCode {
			t.Fatalf("body=%s expected %d, got %d: %s", tc.body, tc.wantCode, rec.Code, rec.Body.String())
		}
	}

	wantSeen := []string{"StopCharging", "KeepCharging", "StopCharging"}
	if len(seenModes) != len(wantSeen) {
		t.Fatalf("expected %v StopInstances calls, got %v", wantSeen, seenModes)
	}
	for i := range wantSeen {
		if seenModes[i] != wantSeen[i] {
			t.Fatalf("call %d expected StoppedMode=%q, got %q", i, wantSeen[i], seenModes[i])
		}
	}
}

func TestAccountDiscoverAllRegions(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Port:                 "8080",
		DataDir:              dir,
		PanelUsername:        "admin",
		PanelPassword:        "pass1234",
		SessionSecret:        "test-session-secret-0123456789abcdef",
		EncryptionKeyRaw:     "test-enc-key-0123456789abcdef",
		DefaultCheckInterval: 60,
	}
	cipher := crypto.NewCipher(cfg.EncryptionKeyRaw)
	store, err := db.Open(dir, cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	staticFS, err := web.StaticFS()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	aliClient := aliyun.NewClient()
	aliClient.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
			raw, _ := io.ReadAll(r.Body)
			vals, _ := url.ParseQuery(string(raw))
			action := vals.Get("Action")
			var payload string
			switch action {
			case "DescribeRegions":
				payload = `{"RequestId":"r1","Regions":{"Region":[{"RegionId":"cn-hongkong","LocalName":"香港"},{"RegionId":"ap-southeast-1","LocalName":"新加坡"}]}}`
			case "DescribeInstances":
				switch vals.Get("RegionId") {
				case "cn-hongkong":
					payload = `{"RequestId":"r2","TotalCount":1,"PageNumber":1,"PageSize":100,"Instances":{"Instance":[{"InstanceId":"i-hk01","InstanceName":"hk-box","RegionId":"cn-hongkong","Status":"Running"}]}}`
				case "ap-southeast-1":
					payload = `{"RequestId":"r3","TotalCount":1,"PageNumber":1,"PageSize":100,"Instances":{"Instance":[{"InstanceId":"i-sg02","InstanceName":"sg-box","RegionId":"ap-southeast-1","Status":"Stopped"}]}}`
				default:
					payload = `{"RequestId":"r4","TotalCount":0,"PageNumber":1,"PageSize":100,"Instances":{"Instance":[]}}`
				}
			default:
				payload = `{"RequestId":"r0"}`
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(payload)),
			}, nil
		}),
	})

	notifier := notify.NewSender(store)
	engine := scheduler.NewEngine(store, aliClient, notifier, cfg.DefaultCheckInterval)
	defer engine.Wait()
	defer cancel()
	authMgr := auth.NewManager(cfg)
	handler := NewServer(ctx, cfg, store, aliClient, notifier, engine, authMgr, staticFS)

	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"pass1234"}`))
	loginReq.RemoteAddr = "127.0.0.1:50002"
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	cookies := loginRec.Result().Cookies()

	accReq := httptest.NewRequest(http.MethodPost, "/api/accounts?defer_check=1", bytes.NewBufferString(`{"name":"主账号","access_key_id":"LTAI5tTestKey0001","access_key_secret":"Secret0001"}`))
	accReq.AddCookie(cookies[0])
	accRec := httptest.NewRecorder()
	handler.ServeHTTP(accRec, accReq)
	if accRec.Code != http.StatusCreated {
		t.Fatalf("create account expected 201, got %d", accRec.Code)
	}

	discReq := httptest.NewRequest(http.MethodGet, "/api/accounts/1/discover", nil)
	discReq.AddCookie(cookies[0])
	discRec := httptest.NewRecorder()
	handler.ServeHTTP(discRec, discReq)
	if discRec.Code != http.StatusOK {
		t.Fatalf("discover all regions expected 200, got %d: %s", discRec.Code, discRec.Body.String())
	}

	var discResp struct {
		Instances []aliyun.ECSInstanceInfo `json:"instances"`
	}
	if err := json.Unmarshal(discRec.Body.Bytes(), &discResp); err != nil {
		t.Fatal(err)
	}
	if len(discResp.Instances) != 2 {
		t.Fatalf("expected 2 instances across regions, got %+v", discResp.Instances)
	}
	if discResp.Instances[0].RegionID != "ap-southeast-1" || discResp.Instances[1].RegionID != "cn-hongkong" {
		t.Fatalf("unexpected sorted order/regions: %+v", discResp.Instances)
	}
}

func TestClearLogsEndpoints(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Port:                 "8080",
		DataDir:              dir,
		PanelUsername:        "admin",
		PanelPassword:        "pass1234",
		SessionSecret:        "test-session-secret-0123456789abcdef",
		EncryptionKeyRaw:     "test-enc-key-0123456789abcdef",
		DefaultCheckInterval: 60,
	}
	cipher := crypto.NewCipher(cfg.EncryptionKeyRaw)
	store, err := db.Open(dir, cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	staticFS, err := web.StaticFS()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())

	aliClient := aliyun.NewClient()
	notifier := notify.NewSender(store)
	engine := scheduler.NewEngine(store, aliClient, notifier, cfg.DefaultCheckInterval)
	defer engine.Wait()
	defer cancel()
	authMgr := auth.NewManager(cfg)
	handler := NewServer(ctx, cfg, store, aliClient, notifier, engine, authMgr, staticFS)

	// Unauthenticated clear must be rejected
	unauthReq := httptest.NewRequest(http.MethodDelete, "/api/logs?scope=all", nil)
	unauthRec := httptest.NewRecorder()
	handler.ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated DELETE /api/logs expected 401, got %d", unauthRec.Code)
	}

	// Login
	loginReq := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewBufferString(`{"username":"admin","password":"pass1234"}`))
	loginReq.RemoteAddr = "127.0.0.1:50003"
	loginRec := httptest.NewRecorder()
	handler.ServeHTTP(loginRec, loginReq)
	cookies := loginRec.Result().Cookies()

	// Create account + 2 instances (each emits audit + realtime logs)
	accReq := httptest.NewRequest(http.MethodPost, "/api/accounts?defer_check=1", bytes.NewBufferString(`{"name":"主账号","access_key_id":"LTAI5tTestKey0001","access_key_secret":"Secret0001"}`))
	accReq.AddCookie(cookies[0])
	accRec := httptest.NewRecorder()
	handler.ServeHTTP(accRec, accReq)
	if accRec.Code != http.StatusCreated {
		t.Fatalf("create account expected 201, got %d", accRec.Code)
	}

	for _, id := range []string{"i-log01", "i-log02"} {
		body := `{"name":"` + id + `","account_id":1,"region_id":"cn-hongkong","instance_id":"` + id + `","traffic_threshold_gb":190,"automation_mode":"paused","enabled":true}`
		req := httptest.NewRequest(http.MethodPost, "/api/instances?defer_check=1", bytes.NewBufferString(body))
		req.AddCookie(cookies[0])
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusCreated {
			t.Fatalf("create instance %s expected 201, got %d", id, rec.Code)
		}
	}

	// Non-existent instance returns 404
	notFoundReq := httptest.NewRequest(http.MethodDelete, "/api/instances/999/logs?scope=all", nil)
	notFoundReq.AddCookie(cookies[0])
	notFoundRec := httptest.NewRecorder()
	handler.ServeHTTP(notFoundRec, notFoundReq)
	if notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown instance, got %d", notFoundRec.Code)
	}

	// Invalid scope or level returns 400
	badScopeReq := httptest.NewRequest(http.MethodDelete, "/api/logs?scope=invalid", nil)
	badScopeReq.AddCookie(cookies[0])
	badScopeRec := httptest.NewRecorder()
	handler.ServeHTTP(badScopeRec, badScopeReq)
	if badScopeRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid scope, got %d", badScopeRec.Code)
	}

	badLevelReq := httptest.NewRequest(http.MethodDelete, "/api/logs?scope=realtime&level=bogus", nil)
	badLevelReq.AddCookie(cookies[0])
	badLevelRec := httptest.NewRecorder()
	handler.ServeHTTP(badLevelRec, badLevelReq)
	if badLevelRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid level, got %d", badLevelRec.Code)
	}

	// Add an error log to instance 1 & instance 2 to verify level-filtered clearing
	inst1Model, _ := store.GetInstance(1)
	inst2Model, _ := store.GetInstance(2)
	engine.RecordManualAction(inst1Model, "error", "error", "inst1 error msg")
	engine.RecordManualAction(inst2Model, "error", "error", "inst2 error msg")

	fetchLogs := func(path string) (auditCount int, realtimeCount int) {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(cookies[0])
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s expected 200, got %d", path, rec.Code)
		}
		var out struct {
			Audit    []db.AuditLog       `json:"audit"`
			Realtime []scheduler.RingLog `json:"realtime"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return len(out.Audit), len(out.Realtime)
	}

	// Clear only error realtime logs for instance 1 -> info realtime & all audit must remain
	clrErrRTReq := httptest.NewRequest(http.MethodDelete, "/api/instances/1/logs?scope=realtime&level=error", nil)
	clrErrRTReq.AddCookie(cookies[0])
	clrErrRTRec := httptest.NewRecorder()
	handler.ServeHTTP(clrErrRTRec, clrErrRTReq)
	if clrErrRTRec.Code != http.StatusOK {
		t.Fatalf("clear inst 1 error realtime expected 200, got %d: %s", clrErrRTRec.Code, clrErrRTRec.Body.String())
	}
	a1, r1 := fetchLogs("/api/instances/1/logs?level=all")
	if a1 != 2 || r1 != 1 {
		t.Fatalf("expected inst 1 audit=2 and realtime=1 (info kept), got audit=%d realtime=%d", a1, r1)
	}
	a2, r2 := fetchLogs("/api/instances/2/logs?level=all")
	if a2 != 2 || r2 != 2 {
		t.Fatalf("expected inst 2 logs untouched (2, 2), got audit=%d realtime=%d", a2, r2)
	}

	// Clear remaining realtime logs for instance 1
	clrRTReq := httptest.NewRequest(http.MethodDelete, "/api/instances/1/logs?scope=realtime&level=all", nil)
	clrRTReq.AddCookie(cookies[0])
	clrRTRec := httptest.NewRecorder()
	handler.ServeHTTP(clrRTRec, clrRTReq)
	if clrRTRec.Code != http.StatusOK {
		t.Fatalf("clear instance 1 realtime expected 200, got %d: %s", clrRTRec.Code, clrRTRec.Body.String())
	}
	a1, r1 = fetchLogs("/api/instances/1/logs?level=all")
	if a1 != 2 || r1 != 0 {
		t.Fatalf("after clearing inst 1 realtime, expected audit=2 and realtime=0, got audit=%d realtime=%d", a1, r1)
	}

	// Clear only error audit logs for instance 1 -> info audit must remain
	clrErrAuditReq := httptest.NewRequest(http.MethodDelete, "/api/instances/1/logs?scope=audit&level=error", nil)
	clrErrAuditReq.AddCookie(cookies[0])
	clrErrAuditRec := httptest.NewRecorder()
	handler.ServeHTTP(clrErrAuditRec, clrErrAuditReq)
	if clrErrAuditRec.Code != http.StatusOK {
		t.Fatalf("clear inst 1 error audit expected 200, got %d", clrErrAuditRec.Code)
	}
	a1, r1 = fetchLogs("/api/instances/1/logs?level=all")
	if a1 != 1 || r1 != 0 {
		t.Fatalf("expected inst 1 audit=1 (info kept) and realtime=0, got audit=%d realtime=%d", a1, r1)
	}

	// Clear remaining audit logs for instance 1
	clrAuditReq := httptest.NewRequest(http.MethodDelete, "/api/instances/1/logs?scope=audit", nil)
	clrAuditReq.AddCookie(cookies[0])
	clrAuditRec := httptest.NewRecorder()
	handler.ServeHTTP(clrAuditRec, clrAuditReq)
	if clrAuditRec.Code != http.StatusOK {
		t.Fatalf("clear instance 1 audit expected 200, got %d", clrAuditRec.Code)
	}
	a1, r1 = fetchLogs("/api/instances/1/logs?level=all")
	if a1 != 0 || r1 != 0 {
		t.Fatalf("expected inst 1 logs all empty, got audit=%d realtime=%d", a1, r1)
	}

	// Clear global error audit logs -> info audit must remain
	clrGErrAuditReq := httptest.NewRequest(http.MethodDelete, "/api/logs?scope=audit&level=error", nil)
	clrGErrAuditReq.AddCookie(cookies[0])
	clrGErrAuditRec := httptest.NewRecorder()
	handler.ServeHTTP(clrGErrAuditRec, clrGErrAuditReq)
	if clrGErrAuditRec.Code != http.StatusOK {
		t.Fatalf("clear global error audit expected 200, got %d", clrGErrAuditRec.Code)
	}
	gaErr, _ := fetchLogs("/api/logs?level=error")
	gaInfo, _ := fetchLogs("/api/logs?level=info")
	if gaErr != 0 || gaInfo == 0 {
		t.Fatalf("expected global error audit=0 and info audit>0, got err=%d info=%d", gaErr, gaInfo)
	}

	// Clear global realtime logs only
	clrGRTReq := httptest.NewRequest(http.MethodDelete, "/api/logs?scope=realtime", nil)
	clrGRTReq.AddCookie(cookies[0])
	clrGRTRec := httptest.NewRecorder()
	handler.ServeHTTP(clrGRTRec, clrGRTReq)
	if clrGRTRec.Code != http.StatusOK {
		t.Fatalf("clear global realtime expected 200, got %d", clrGRTRec.Code)
	}
	ga, gr := fetchLogs("/api/logs?level=all")
	if ga == 0 || gr != 0 {
		t.Fatalf("expected global audit>0 and realtime=0, got audit=%d realtime=%d", ga, gr)
	}

	// Clear global all logs (default scope & level)
	clrAllReq := httptest.NewRequest(http.MethodDelete, "/api/logs", nil)
	clrAllReq.AddCookie(cookies[0])
	clrAllRec := httptest.NewRecorder()
	handler.ServeHTTP(clrAllRec, clrAllReq)
	if clrAllRec.Code != http.StatusOK {
		t.Fatalf("clear global all expected 200, got %d", clrAllRec.Code)
	}
	ga, gr = fetchLogs("/api/logs?level=all")
	if ga != 0 || gr != 0 {
		t.Fatalf("expected all global logs empty, got audit=%d realtime=%d", ga, gr)
	}
}

func TestStaticAssetCacheBustingAndHeaders(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		Port:                 "8080",
		DataDir:              dir,
		PanelUsername:        "admin",
		PanelPassword:        "pass1234",
		SessionSecret:        "test-session-secret-0123456789abcdef",
		EncryptionKeyRaw:     "test-enc-key-0123456789abcdef",
		DefaultCheckInterval: 60,
	}
	cipher := crypto.NewCipher(cfg.EncryptionKeyRaw)
	store, err := db.Open(dir, cipher)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	staticFS, err := web.StaticFS()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	aliClient := aliyun.NewClient()
	notifier := notify.NewSender(store)
	engine := scheduler.NewEngine(store, aliClient, notifier, cfg.DefaultCheckInterval)
	defer engine.Wait()
	authMgr := auth.NewManager(cfg)
	handler := NewServer(ctx, cfg, store, aliClient, notifier, engine, authMgr, staticFS)

	// 1. Index HTML must not be cached and must embed versioned asset URLs.
	indexReq := httptest.NewRequest(http.MethodGet, "/", nil)
	indexRec := httptest.NewRecorder()
	handler.ServeHTTP(indexRec, indexReq)
	if indexRec.Code != http.StatusOK {
		t.Fatalf("GET / expected 200, got %d", indexRec.Code)
	}
	if cc := indexRec.Header().Get("Cache-Control"); cc != "no-store" {
		t.Fatalf("GET / Cache-Control expected no-store, got %q", cc)
	}
	html := indexRec.Body.String()
	cssMatch := regexp.MustCompile(`href="(/app\.css\?v=[0-9a-f]{12})"`).FindStringSubmatch(html)
	if len(cssMatch) != 2 {
		t.Fatalf("expected versioned /app.css?v=... link in index.html")
	}
	jsMatch := regexp.MustCompile(`src="(/app\.js\?v=[0-9a-f]{12})"`).FindStringSubmatch(html)
	if len(jsMatch) != 2 {
		t.Fatalf("expected versioned /app.js?v=... script in index.html")
	}

	// 2. Versioned asset requests get immutable caching and ETag + 304 support.
	for _, assetURL := range []string{cssMatch[1], jsMatch[1]} {
		req := httptest.NewRequest(http.MethodGet, assetURL, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s expected 200, got %d", assetURL, rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
			t.Fatalf("GET %s expected immutable Cache-Control, got %q", assetURL, cc)
		}
		etag := rec.Header().Get("ETag")
		if etag == "" {
			t.Fatalf("GET %s expected non-empty ETag", assetURL)
		}

		condReq := httptest.NewRequest(http.MethodGet, assetURL, nil)
		condReq.Header.Set("If-None-Match", etag)
		condRec := httptest.NewRecorder()
		handler.ServeHTTP(condRec, condReq)
		if condRec.Code != http.StatusNotModified {
			t.Fatalf("conditional GET %s expected 304, got %d", assetURL, condRec.Code)
		}
	}

	// 3. Unversioned or stale-version asset requests must disable caching.
	for _, staleURL := range []string{"/app.js", "/app.js?v=stale0000000", "/app.css"} {
		req := httptest.NewRequest(http.MethodGet, staleURL, nil)
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s expected 200, got %d", staleURL, rec.Code)
		}
		if cc := rec.Header().Get("Cache-Control"); cc != "no-store" {
			t.Fatalf("GET %s expected Cache-Control no-store, got %q", staleURL, cc)
		}
	}
}
