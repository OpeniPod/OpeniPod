package storage_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/storage"
)

func TestFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config", "settings.json")
	ctx := context.Background()
	for _, volume := range []int{70, 0} {
		want := app.DefaultSettings()
		want.Volume = volume
		if err := storage.NewFile(path).SaveSettings(ctx, want); err != nil {
			t.Fatal(err)
		}
		got, err := storage.NewFile(path).LoadSettings(ctx)
		if err != nil || got != want {
			t.Fatalf("load = %+v, %v; want %+v", got, err, want)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"volume":0}` {
		t.Fatalf("unexpected JSON: %s", data)
	}
}

func TestFileMissingUsesDefaultsWithoutWriting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	got, err := storage.NewFile(path).LoadSettings(context.Background())
	if err != nil || got != app.DefaultSettings() {
		t.Fatalf("load = %+v, %v", got, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("missing file was created: %v", err)
	}
}

func TestFileLoadJSON(t *testing.T) {
	for _, tc := range []struct {
		name string
		data string
		want app.SettingsState
		bad  bool
	}{
		{"old file", `{}`, app.DefaultSettings(), false},
		{"explicit zero", `{"volume":0}`, app.SettingsState{Volume: 0}, false},
		{"unknown field", `{"volume":30,"future":true}`, app.SettingsState{Volume: 30}, false},
		{"truncated", `{"volume":30`, app.SettingsState{}, true},
		{"wrong type", `{"volume":"loud"}`, app.SettingsState{}, true},
		{"empty", ``, app.SettingsState{}, true},
		{"null", `null`, app.SettingsState{}, true},
		{"array", `[]`, app.SettingsState{}, true},
		{"trailing data", `{"volume":30}{}`, app.SettingsState{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			if err := os.WriteFile(path, []byte(tc.data), 0600); err != nil {
				t.Fatal(err)
			}
			got, err := storage.NewFile(path).LoadSettings(context.Background())
			if (err != nil) != tc.bad || got != tc.want {
				t.Fatalf("load = %+v, %v; want %+v, error=%t", got, err, tc.want, tc.bad)
			}
			data, err := os.ReadFile(path)
			if err != nil || string(data) != tc.data {
				t.Fatalf("load changed file: %s, %v", data, err)
			}
		})
	}
}

func TestFileCancelledOperationsPreserveSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	f := storage.NewFile(path)
	want := app.DefaultSettings()
	if err := f.SaveSettings(context.Background(), want); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := f.LoadSettings(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("load error = %v", err)
	}
	if err := f.SaveSettings(ctx, app.SettingsState{Volume: 90}); !errors.Is(err, context.Canceled) {
		t.Fatalf("save error = %v", err)
	}
	got, err := f.LoadSettings(context.Background())
	if err != nil || got != want {
		t.Fatalf("previous settings lost: %+v, %v", got, err)
	}
}

func TestFileReadError(t *testing.T) {
	if _, err := storage.NewFile(t.TempDir()).LoadSettings(context.Background()); err == nil {
		t.Fatal("reading a directory should fail")
	}
}

func TestFileFailedReplacementCleansTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if err := storage.NewFile(path).SaveSettings(context.Background(), app.DefaultSettings()); err == nil {
		t.Fatal("replacing a directory should fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "settings.json" || !entries[0].IsDir() {
		t.Fatalf("unexpected files after failed save: %v", entries)
	}
}
