package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"ecs-cdt-panel/internal/db"
)

// Event represents a notification trigger event from the CDT panel.
type Event struct {
	Type       string  `json:"type"` // start | stop | warn | fuse | error | test
	Title      string  `json:"title"`
	Body       string  `json:"body"`
	InstanceID string  `json:"instance_id,omitempty"`
	RegionID   string  `json:"region_id,omitempty"`
	FlowType   string  `json:"flow_type,omitempty"`
	TrafficGB  float64 `json:"traffic_gb,omitempty"`
	LimitGB    float64 `json:"limit_gb,omitempty"`
	Timestamp  string  `json:"timestamp"`
}

// Sender dispatches Bark and Webhook notifications according to stored settings.
type Sender struct {
	store      *db.Store
	httpClient *http.Client
}

// NewSender initializes a notification dispatcher.
func NewSender(store *db.Store) *Sender {
	return &Sender{
		store: store,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// Dispatch checks event subscriptions and sends enabled notification channels.
func (s *Sender) Dispatch(ctx context.Context, ev Event) error {
	cfg, err := s.store.GetNotificationSettings(true)
	if err != nil {
		return err
	}
	if ev.Timestamp == "" {
		ev.Timestamp = time.Now().Format(time.RFC3339)
	}

	if ev.Type != "test" {
		switch ev.Type {
		case "start":
			if !cfg.NotifyOnStart {
				return nil
			}
		case "stop":
			if !cfg.NotifyOnStop {
				return nil
			}
		case "warn":
			if !cfg.NotifyOnWarn {
				return nil
			}
		case "fuse":
			if !cfg.NotifyOnFuse {
				return nil
			}
		case "error":
			if !cfg.NotifyOnError {
				return nil
			}
		}
	}

	var errs []string
	if cfg.BarkEnabled && cfg.BarkEndpoint != "" {
		if err := s.sendBark(ctx, cfg, ev); err != nil {
			errs = append(errs, "Bark: "+err.Error())
		}
	}
	if cfg.WebhookEnabled && cfg.WebhookURL != "" {
		if err := s.sendWebhook(ctx, cfg.WebhookURL, ev); err != nil {
			errs = append(errs, "Webhook: "+err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s", strings.Join(errs, "; "))
	}
	return nil
}

func (s *Sender) sendBark(ctx context.Context, cfg db.NotificationSettings, ev Event) error {
	endpoint := strings.TrimRight(strings.TrimSpace(cfg.BarkEndpoint), "/")
	if endpoint == "" {
		return nil
	}

	payload := map[string]any{
		"title": ev.Title,
		"body":  ev.Body,
		"group": cfg.BarkGroup,
	}
	if cfg.BarkSound != "" {
		payload["sound"] = cfg.BarkSound
	}
	if ev.Type == "fuse" || ev.Type == "error" {
		payload["level"] = "timeSensitive"
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	// Bark supports POST JSON to /push or direct endpoint URL.
	// Use GET query fallback if endpoint ends with a device key path (standard api.day.app/<key>),
	// and POST JSON to endpoint directly which Bark v2 servers accept natively.
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		return nil
	}

	// Fallback to GET query format (compatible with legacy Bark endpoints/proxies).
	q := url.Values{}
	q.Set("group", cfg.BarkGroup)
	q.Set("title", ev.Title)
	q.Set("body", ev.Body)
	if cfg.BarkSound != "" {
		q.Set("sound", cfg.BarkSound)
	}
	getReq, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint+"?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	getResp, err := s.httpClient.Do(getReq)
	if err != nil {
		return err
	}
	defer getResp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(getResp.Body, 1024))
	if getResp.StatusCode < 200 || getResp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", getResp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}

func (s *Sender) sendWebhook(ctx context.Context, webhookURL string, ev Event) error {
	payload := map[string]any{
		"event":       ev.Type,
		"title":       ev.Title,
		"body":        ev.Body,
		"msgtype":     "text",
		"text":        map[string]string{"content": ev.Title + "\n" + ev.Body},
		"instance_id": ev.InstanceID,
		"region_id":   ev.RegionID,
		"flow_type":   ev.FlowType,
		"traffic_gb":  ev.TrafficGB,
		"limit_gb":    ev.LimitGB,
		"timestamp":   ev.Timestamp,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimSpace(webhookURL), bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}
