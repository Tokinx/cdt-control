package db

import (
	"database/sql"
	"fmt"
	"path/filepath"

	"ecs-cdt-panel/internal/crypto"
	_ "modernc.org/sqlite"
)

// Store wraps SQLite persistence and field-level encryption.
type Store struct {
	db     *sql.DB
	cipher *crypto.Cipher
}

// Open initializes the SQLite database in low-memory WAL mode and applies schema migrations.
func Open(dataDir string, cipher *crypto.Cipher) (*Store, error) {
	dbPath := filepath.Join(dataDir, "panel.db")
	dsn := fmt.Sprintf(
		"file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=synchronous(NORMAL)&_pragma=cache_size(-1000)&_pragma=foreign_keys(ON)&_pragma=temp_store(FILE)",
		dbPath,
	)

	sqlDB, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open sqlite %q: %w", dbPath, err)
	}

	// Single connection keeps RSS memory minimal and eliminates SQLITE_BUSY writer contention.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}

	s := &Store{
		db:     sqlDB,
		cipher: cipher,
	}
	if err := s.migrate(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("migrate sqlite: %w", err)
	}

	return s, nil
}

// Close closes the underlying SQLite database handle.
func (s *Store) Close() error {
	return s.db.Close()
}
