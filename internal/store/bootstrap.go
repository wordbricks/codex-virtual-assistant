package store

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/siisee11/CodexVirtualAssistant/internal/config"
)

func EnsureScaffold(cfg config.Config) error {
	dirs := []string{cfg.DataDir, cfg.EffectiveProjectsDir(), cfg.ArtifactDir, filepath.Dir(cfg.DatabasePath)}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("ensure directory %s: %w", dir, err)
		}
	}

	file, err := os.OpenFile(cfg.DatabasePath, os.O_CREATE, 0o644)
	if err != nil {
		return fmt.Errorf("ensure database file: %w", err)
	}
	return file.Close()
}
