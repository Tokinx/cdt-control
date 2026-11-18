package db

import "fmt"

const schemaSQL = `
CREATE TABLE IF NOT EXISTS accounts (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL,
	access_key_id TEXT NOT NULL,
	access_key_secret_enc TEXT NOT NULL,
	key_fingerprint TEXT NOT NULL,
	domestic_quota_gb REAL NOT NULL DEFAULT 20,
	overseas_quota_gb REAL NOT NULL DEFAULT 180,
	enabled INTEGER NOT NULL DEFAULT 1,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS instances (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	name TEXT NOT NULL DEFAULT '',
	account_id INTEGER REFERENCES accounts(id) ON DELETE SET NULL,
	access_key_id TEXT NOT NULL DEFAULT '',
	access_key_secret_enc TEXT NOT NULL DEFAULT '',
	key_fingerprint TEXT NOT NULL DEFAULT '',
	region_id TEXT NOT NULL,
	instance_id TEXT NOT NULL,
	traffic_threshold_gb REAL NOT NULL DEFAULT 190,
	warn_percent INTEGER NOT NULL DEFAULT 0,
	automation_mode TEXT NOT NULL DEFAULT 'full',
	enable_schedule_control INTEGER NOT NULL DEFAULT 0,
	schedule_start TEXT NOT NULL DEFAULT '08:00',
	schedule_stop TEXT NOT NULL DEFAULT '00:00',
	force_stop INTEGER NOT NULL DEFAULT 0,
	stopped_mode TEXT NOT NULL DEFAULT 'StopCharging',
	enabled INTEGER NOT NULL DEFAULT 1,
	last_status TEXT NOT NULL DEFAULT 'Unknown',
	last_public_ip TEXT NOT NULL DEFAULT '',
	last_private_ip TEXT NOT NULL DEFAULT '',
	last_spec TEXT NOT NULL DEFAULT '',
	last_os TEXT NOT NULL DEFAULT '',
	last_charge_type TEXT NOT NULL DEFAULT '',
	last_traffic_gb REAL NOT NULL DEFAULT 0,
	last_flow_type TEXT NOT NULL DEFAULT '',
	last_checked_at TEXT NOT NULL DEFAULT '',
	last_error TEXT NOT NULL DEFAULT '',
	warned_month TEXT NOT NULL DEFAULT '',
	fused_month TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL,
	UNIQUE(region_id, instance_id, key_fingerprint)
);

CREATE TABLE IF NOT EXISTS cdt_latest (
	key_fingerprint TEXT PRIMARY KEY,
	access_key_masked TEXT NOT NULL,
	label TEXT NOT NULL DEFAULT '',
	domestic_bytes INTEGER NOT NULL DEFAULT 0,
	overseas_bytes INTEGER NOT NULL DEFAULT 0,
	domestic_gb REAL NOT NULL DEFAULT 0,
	overseas_gb REAL NOT NULL DEFAULT 0,
	domestic_quota_gb REAL NOT NULL DEFAULT 20,
	overseas_quota_gb REAL NOT NULL DEFAULT 180,
	details_json TEXT NOT NULL DEFAULT '[]',
	last_error TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS traffic_snapshots (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	key_fingerprint TEXT NOT NULL,
	domestic_gb REAL NOT NULL,
	overseas_gb REAL NOT NULL,
	recorded_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_snapshots_fp_time ON traffic_snapshots(key_fingerprint, recorded_at);

CREATE TABLE IF NOT EXISTS audit_logs (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	instance_id_ref INTEGER NOT NULL DEFAULT 0,
	aliyun_instance_id TEXT NOT NULL DEFAULT '',
	region_id TEXT NOT NULL DEFAULT '',
	level TEXT NOT NULL,
	event TEXT NOT NULL,
	message TEXT NOT NULL,
	created_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_logs_inst_time ON audit_logs(instance_id_ref, created_at);
CREATE INDEX IF NOT EXISTS idx_logs_time ON audit_logs(created_at);

CREATE TABLE IF NOT EXISTS settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`

func (s *Store) migrate() error {
	if _, err := s.db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("exec schema: %w", err)
	}
	return nil
}
