package storage

import (
	"context"

	"github.com/OpeniPod/OpeniPod/internal/app"
)

// Memory stores settings for one process lifetime.
type Memory struct {
	settings app.SettingsState
}

func NewMemory(volume int) *Memory {
	return &Memory{settings: app.SettingsState{Volume: volume}}
}

func (m *Memory) LoadSettings(ctx context.Context) (app.SettingsState, error) {
	if err := ctx.Err(); err != nil {
		return app.SettingsState{}, err
	}
	return m.settings, nil
}

func (m *Memory) SaveSettings(ctx context.Context, settings app.SettingsState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.settings = settings
	return nil
}
