package db

import (
	"testing"
	"time"

	"ecs-cdt-panel/internal/aliyun"
	"ecs-cdt-panel/internal/crypto"
)

func TestStoreAccountsInstancesAndCDT(t *testing.T) {
	dir := t.TempDir()
	cipher := crypto.NewCipher("test-master-secret-123456")
	store, err := Open(dir, cipher)
	if err != nil {
		t.Fatalf("Open store: %v", err)
	}
	defer store.Close()

	// Create saved account
	acc, err := store.CreateAccount(AccountInput{
		Name:            "主账号",
		AccessKeyID:     "LTAI5tMainAccountKey123",
		AccessKeySecret: "SecretMain123456",
		DomesticQuotaGB: 20,
		OverseasQuotaGB: 180,
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	if acc.AccessKeyMasked == acc.AccessKeyID {
		t.Fatalf("expected masked key, got %q", acc.AccessKeyMasked)
	}

	// Instance 1 linked to saved account
	inst1, err := store.CreateInstance(InstanceInput{
		Name:               "香港节点",
		AccountID:          &acc.ID,
		RegionID:           "cn-hongkong",
		InstanceID:         "i-hk001",
		TrafficThresholdGB: 190,
		AutomationMode:     "full",
		ScheduleStart:      "08:00",
		ScheduleStop:       "00:00",
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("CreateInstance (account): %v", err)
	}
	if inst1.StoppedMode != "StopCharging" {
		t.Fatalf("expected default StoppedMode StopCharging, got %q", inst1.StoppedMode)
	}
	creds1, err := store.ResolveInstanceCredentials(inst1)
	if err != nil || creds1.AccessKeySecret != "SecretMain123456" {
		t.Fatalf("ResolveInstanceCredentials(inst1) = (%+v, %v)", creds1, err)
	}

	// Second saved account & instance
	acc2, err := store.CreateAccount(AccountInput{
		Name:            "新加坡账号",
		AccessKeyID:     "LTAI5tOtherAccountKey99",
		AccessKeySecret: "SecretOther9999",
		Enabled:         true,
	})
	if err != nil {
		t.Fatalf("CreateAccount 2: %v", err)
	}
	inst2, err := store.CreateInstance(InstanceInput{
		Name:               "新加坡节点",
		AccountID:          &acc2.ID,
		RegionID:           "ap-southeast-1",
		InstanceID:         "i-sg002",
		TrafficThresholdGB: 150,
		AutomationMode:     "fuse_only",
		StoppedMode:        "KeepCharging",
		Enabled:            true,
	})
	if err != nil {
		t.Fatalf("CreateInstance (account 2): %v", err)
	}
	if inst2.KeyFingerprint == inst1.KeyFingerprint {
		t.Fatalf("expected separate account key fingerprint, got %+v", inst2)
	}
	creds2, err := store.ResolveInstanceCredentials(inst2)
	if err != nil || creds2.AccessKeyID != "LTAI5tOtherAccountKey99" || creds2.AccessKeySecret != "SecretOther9999" {
		t.Fatalf("ResolveInstanceCredentials(inst2) = (%+v, %v)", creds2, err)
	}

	// Creating instance without account_id should be rejected
	if _, err := store.CreateInstance(InstanceInput{
		RegionID:   "cn-hongkong",
		InstanceID: "i-noacc",
	}); err == nil {
		t.Fatal("expected CreateInstance without account_id to fail")
	}

	// Account delete should fail while inst1 references it
	if err := store.DeleteAccount(acc.ID); err == nil {
		t.Fatal("expected DeleteAccount to reject while referenced by inst1")
	}

	// CDT latest + snapshots
	now := time.Now()
	if err := store.UpsertCDTLatest(CDTLatest{
		KeyFingerprint:  inst2.KeyFingerprint,
		AccessKeyMasked: inst2.AccessKeyMasked,
		Label:           "独立凭证 · 新加坡",
		OverseasGB:      12.34,
		Details:         []aliyun.RegionTraffic{{BusinessRegionID: "ap-southeast-1", TrafficGB: 12.34}},
		UpdatedAt:       now.Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("UpsertCDTLatest: %v", err)
	}
	if err := store.InsertTrafficSnapshotIfNeeded(inst2.KeyFingerprint, 0, 12.34, now, 5*time.Minute); err != nil {
		t.Fatalf("InsertTrafficSnapshotIfNeeded: %v", err)
	}
	pts, err := store.QueryTrafficSnapshots(inst2.KeyFingerprint, now.Add(-time.Hour), 50)
	if err != nil || len(pts) != 1 {
		t.Fatalf("QueryTrafficSnapshots = (%d, %v)", len(pts), err)
	}

	// Notification settings secret preservation
	if err := store.SaveNotificationSettings(NotificationSettings{
		BarkEnabled:  true,
		BarkEndpoint: "https://api.day.app/SecretToken123456",
		BarkGroup:    "CDT",
		NotifyOnFuse: true,
	}); err != nil {
		t.Fatalf("SaveNotificationSettings: %v", err)
	}
	maskedCfg, err := store.GetNotificationSettings(false)
	if err != nil || !maskedCfg.HasBark || maskedCfg.BarkEndpoint != "" {
		t.Fatalf("expected masked notification config, got %+v, err=%v", maskedCfg, err)
	}
	// Save again without re-entering endpoint preserves secret
	if err := store.SaveNotificationSettings(NotificationSettings{
		BarkEnabled:  true,
		HasBark:      true,
		BarkGroup:    "CDT-Updated",
		NotifyOnFuse: true,
	}); err != nil {
		t.Fatalf("SaveNotificationSettings preserve: %v", err)
	}
	fullCfg, err := store.GetNotificationSettings(true)
	if err != nil || fullCfg.BarkEndpoint != "https://api.day.app/SecretToken123456" || fullCfg.BarkGroup != "CDT-Updated" {
		t.Fatalf("expected preserved bark endpoint, got %+v, err=%v", fullCfg, err)
	}
}
