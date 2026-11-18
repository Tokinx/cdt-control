package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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
