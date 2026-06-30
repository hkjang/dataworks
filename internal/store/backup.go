package store

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Backup copies the database file to a timestamped backup file if SQLite is used.
func (s *SQLStore) Backup(ctx context.Context, driver, dsn string) (string, error) {
	if driver != "sqlite" {
		// For non-SQLite databases like PostgreSQL, we assume administrative backups are configured.
		return "PostgreSQL (backup skipped / managed externally)", nil
	}

	srcFile := dsn
	if _, err := os.Stat(srcFile); os.IsNotExist(err) {
		return "no database file to backup yet", nil
	}

	dir := filepath.Dir(srcFile)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("create backup dir: %w", err)
	}

	backupName := fmt.Sprintf("%s.bak.%d", filepath.Base(srcFile), time.Now().Unix())
	destFile := filepath.Join(dir, backupName)

	src, err := os.Open(srcFile)
	if err != nil {
		return "", fmt.Errorf("open src db file: %w", err)
	}
	defer src.Close()

	dest, err := os.Create(destFile)
	if err != nil {
		return "", fmt.Errorf("create backup db file: %w", err)
	}
	defer dest.Close()

	if _, err := io.Copy(dest, src); err != nil {
		return "", fmt.Errorf("copy db file: %w", err)
	}

	return destFile, nil
}
