package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OpeniPod/OpeniPod/internal/app"
)

// File stores the complete SettingsState as compact JSON at a caller-chosen path.
type File struct {
	path string
}

var _ app.Storage = (*File)(nil)

func NewFile(path string) *File { return &File{path: path} }

func (f *File) LoadSettings(ctx context.Context) (app.SettingsState, error) {
	if err := ctx.Err(); err != nil {
		return app.SettingsState{}, err
	}
	data, err := os.ReadFile(f.path)
	if os.IsNotExist(err) {
		return app.DefaultSettings(), nil
	}
	if err != nil {
		return app.SettingsState{}, fmt.Errorf("read settings %q: %w", f.path, err)
	}
	if err := ctx.Err(); err != nil {
		return app.SettingsState{}, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 || data[0] != '{' {
		return app.SettingsState{}, fmt.Errorf("decode settings %q: expected JSON object", f.path)
	}
	settings := app.DefaultSettings()
	if err := json.Unmarshal(data, &settings); err != nil {
		return app.SettingsState{}, fmt.Errorf("decode settings %q: %w", f.path, err)
	}
	return settings, nil
}

func (f *File) SaveSettings(ctx context.Context, settings app.SettingsState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode settings: %w", err)
	}
	dir := filepath.Dir(f.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create settings directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return fmt.Errorf("create temporary settings file: %w", err)
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return fmt.Errorf("write settings: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return fmt.Errorf("sync settings: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close settings: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmp.Name(), f.path); err != nil {
		return fmt.Errorf("replace settings %q: %w", f.path, err)
	}
	return nil
}
