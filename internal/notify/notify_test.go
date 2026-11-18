package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"ecs-cdt-panel/internal/crypto"
	"ecs-cdt-panel/internal/db"
)

func TestDispatchBarkAndWebhook(t *testing.T) {
	var barkHits atomic.Int32
	var webhookHits atomic.Int32

	barkSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("bark decode: %v", err)
		}
		if body["group"] != "DevOps" {
			t.Errorf("expected bark group DevOps, got %v", body["group"])
		}
		barkHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer barkSrv.Close()

	hookSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("webhook decode: %v", err)
		}
		if body["event"] != "fuse" {
			t.Errorf("expected event fuse, got %v", body["event"])
		}
		webhookHits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer hookSrv.Close()

	store, err := db.Open(t.TempDir(), crypto.NewCipher("notify-test-secret"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	if err := store.SaveNotificationSettings(db.NotificationSettings{
		BarkEnabled:    true,
		BarkEndpoint:   barkSrv.URL,
		BarkGroup:      "DevOps",
		WebhookEnabled: true,
		WebhookURL:     hookSrv.URL,
		NotifyOnFuse:   true,
		NotifyOnStart:  false,
	}); err != nil {
		t.Fatal(err)
	}

	sender := NewSender(store)
	// Disabled event should not fire
	if err := sender.Dispatch(context.Background(), Event{Type: "start", Title: "Start"}); err != nil {
		t.Fatal(err)
	}
	if barkHits.Load() != 0 || webhookHits.Load() != 0 {
		t.Fatalf("expected 0 hits for disabled event, got bark=%d hook=%d", barkHits.Load(), webhookHits.Load())
	}

	// Enabled event should fire both
	if err := sender.Dispatch(context.Background(), Event{Type: "fuse", Title: "Fuse", Body: "Stopped"}); err != nil {
		t.Fatal(err)
	}
	if barkHits.Load() != 1 || webhookHits.Load() != 1 {
		t.Fatalf("expected 1 hit each, got bark=%d hook=%d", barkHits.Load(), webhookHits.Load())
	}
}
