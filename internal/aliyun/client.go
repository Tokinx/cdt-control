package aliyun

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"
)

// Credentials identifies an Alibaba Cloud RAM/Account AccessKey pair.
type Credentials struct {
	AccessKeyID     string
	AccessKeySecret string
}

// APIError represents a structured error response returned by Alibaba Cloud OpenAPI.
type APIError struct {
	HTTPStatus int
	Code       string `json:"Code"`
	Message    string `json:"Message"`
	RequestID  string `json:"RequestId"`
	HostID     string `json:"HostId"`
}

func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("HTTP %d %s: %s (RequestId: %s)", e.HTTPStatus, e.Code, e.Message, e.RequestID)
	}
	return fmt.Sprintf("HTTP %d: %s", e.HTTPStatus, e.Message)
}

// Client calls Alibaba Cloud RPC OpenAPIs without heavy SDK dependencies.
type Client struct {
	httpClient *http.Client
}

// NewClient constructs a reusable lightweight HTTP RPC client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				MaxIdleConns:        20,
				MaxIdleConnsPerHost: 5,
				IdleConnTimeout:     60 * time.Second,
			},
		},
	}
}

// SetHTTPClient replaces the underlying HTTP client, primarily for testing.
func (c *Client) SetHTTPClient(httpClient *http.Client) {
	if httpClient != nil {
		c.httpClient = httpClient
	}
}

// CallRPC executes a signed Alibaba Cloud RPC v2 request and unmarshals JSON into out.
func (c *Client) CallRPC(
	ctx context.Context,
	creds Credentials,
	endpoint string,
	version string,
	action string,
	params map[string]string,
	out any,
) error {
	if strings.TrimSpace(creds.AccessKeyID) == "" || strings.TrimSpace(creds.AccessKeySecret) == "" {
		return fmt.Errorf("missing Alibaba Cloud AccessKey credentials")
	}

	nonce, err := randomNonce()
	if err != nil {
		return err
	}

	all := make(map[string]string, len(params)+8)
	for k, v := range params {
		if v != "" {
			all[k] = v
		}
	}
	all["Format"] = "JSON"
	all["Version"] = version
	all["AccessKeyId"] = strings.TrimSpace(creds.AccessKeyID)
	all["SignatureMethod"] = "HMAC-SHA1"
	all["Timestamp"] = time.Now().UTC().Format("2006-01-02T15:04:05Z")
	all["SignatureVersion"] = "1.0"
	all["SignatureNonce"] = nonce
	all["Action"] = action

	signature := signRPC("POST", all, strings.TrimSpace(creds.AccessKeySecret))
	all["Signature"] = signature

	form := buildEncodedQuery(all)
	targetURL := endpoint
	if !strings.HasPrefix(targetURL, "https://") && !strings.HasPrefix(targetURL, "http://") {
		targetURL = "https://" + targetURL
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, targetURL, strings.NewReader(form))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request %s (%s): %w", action, endpoint, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("read response %s: %w", action, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		apiErr := &APIError{HTTPStatus: resp.StatusCode}
		if jsonErr := json.Unmarshal(body, apiErr); jsonErr != nil || apiErr.Code == "" {
			apiErr.Message = strings.TrimSpace(string(body))
		}
		return apiErr
	}

	if out != nil {
		if err := json.Unmarshal(body, out); err != nil {
			return fmt.Errorf("decode %s response: %w", action, err)
		}
	}
	return nil
}

// signRPC computes the Alibaba Cloud POP RPC HMAC-SHA1 signature.
func signRPC(method string, params map[string]string, secret string) string {
	canonicalized := buildEncodedQuery(params)
	stringToSign := strings.ToUpper(method) + "&" + percentEncode("/") + "&" + percentEncode(canonicalized)

	mac := hmac.New(sha1.New, []byte(secret+"&"))
	mac.Write([]byte(stringToSign))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func buildEncodedQuery(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var sb strings.Builder
	for i, k := range keys {
		if i > 0 {
			sb.WriteByte('&')
		}
		sb.WriteString(percentEncode(k))
		sb.WriteByte('=')
		sb.WriteString(percentEncode(params[k]))
	}
	return sb.String()
}

// percentEncode applies RFC 3986 encoding required by Alibaba Cloud OpenAPI signatures.
func percentEncode(s string) string {
	encoded := url.QueryEscape(s)
	encoded = strings.ReplaceAll(encoded, "+", "%20")
	encoded = strings.ReplaceAll(encoded, "*", "%2A")
	encoded = strings.ReplaceAll(encoded, "%7E", "~")
	return encoded
}

func randomNonce() (string, error) {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
