package aliyun

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
)

func TestIsDomesticRegion(t *testing.T) {
	cases := []struct {
		region string
		want   bool
	}{
		{"cn-shanghai", true},
		{"cn-hangzhou", true},
		{"CN-BEIJING", true},
		{"cn-hongkong", false},
		{"cn-taiwan", false},
		{"CN-HONGKONG", false},
		{"ap-southeast-1", false},
		{"us-west-1", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := IsDomesticRegion(tc.region); got != tc.want {
			t.Errorf("IsDomesticRegion(%q) = %v, want %v", tc.region, got, tc.want)
		}
	}
}

func TestPercentEncodeAndSignRPC(t *testing.T) {
	if got := percentEncode("a b*c~d/"); got != "a%20b%2Ac~d%2F" {
		t.Fatalf("percentEncode unexpected: %q", got)
	}

	params := map[string]string{
		"Format":           "JSON",
		"Version":          "2021-08-13",
		"AccessKeyId":      "testid",
		"SignatureMethod":  "HMAC-SHA1",
		"Timestamp":        "2026-01-01T00:00:00Z",
		"SignatureVersion": "1.0",
		"SignatureNonce":   "nonce123",
		"Action":           "ListCdtInternetTraffic",
	}
	sig1 := signRPC("POST", params, "testsecret")
	sig2 := signRPC("POST", params, "testsecret")
	if sig1 == "" || sig1 != sig2 {
		t.Fatalf("signRPC deterministic check failed: %q vs %q", sig1, sig2)
	}
}

func TestCallRPCSuccessAndError(t *testing.T) {
	var seenForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		seenForm = r.PostForm
		if seenForm.Get("Action") == "FailAction" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"Code":"InvalidInstanceId.NotFound","Message":"not found","RequestId":"req-1"}`))
			return
		}
		_, _ = w.Write([]byte(`{"RequestId":"req-ok","TrafficDetails":[{"BusinessRegionId":"cn-hongkong","Traffic":1073741824}]}`))
	}))
	defer srv.Close()

	client := NewClient()
	creds := Credentials{AccessKeyID: "ak123", AccessKeySecret: "sk456"}

	var resp listCdtTrafficResponse
	if err := client.CallRPC(context.Background(), creds, srv.URL, "2021-08-13", "ListCdtInternetTraffic", nil, &resp); err != nil {
		t.Fatalf("CallRPC unexpected error: %v", err)
	}
	if seenForm.Get("Signature") == "" || seenForm.Get("AccessKeyId") != "ak123" {
		t.Fatalf("missing signed form params: %+v", seenForm)
	}
	if len(resp.TrafficDetails) != 1 || resp.TrafficDetails[0].Traffic != 1073741824 {
		t.Fatalf("unexpected decoded response: %+v", resp)
	}

	err := client.CallRPC(context.Background(), creds, srv.URL, "2014-05-26", "FailAction", nil, nil)
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "InvalidInstanceId.NotFound" || apiErr.HTTPStatus != 404 {
		t.Fatalf("expected structured APIError, got %T %v", err, err)
	}
}

func TestNewClientConfiguresProxyAndTimeouts(t *testing.T) {
	client := NewClient()
	tr, ok := client.httpClient.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("expected *http.Transport, got %T", client.httpClient.Transport)
	}
	if tr.Proxy == nil {
		t.Fatal("expected Transport.Proxy to be configured")
	}
	if reflect.ValueOf(tr.Proxy).Pointer() != reflect.ValueOf(http.ProxyFromEnvironment).Pointer() {
		t.Fatal("expected Transport.Proxy to use http.ProxyFromEnvironment")
	}
	if tr.DialContext == nil {
		t.Fatal("expected Transport.DialContext to be configured")
	}
	if tr.TLSHandshakeTimeout <= 0 {
		t.Fatalf("expected positive TLSHandshakeTimeout, got %v", tr.TLSHandshakeTimeout)
	}
}
