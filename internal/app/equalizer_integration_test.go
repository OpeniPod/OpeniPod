package app_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/player"
	"github.com/OpeniPod/OpeniPod/internal/storage"
)

func TestEqualizerPersistsAndAppliesAcrossAppRestarts(t *testing.T) {
	for _, test := range []struct {
		name    string
		presses int
		want    app.EqualizerPreset
	}{
		{"bass boost", 1, app.EqualizerBassBoost},
		{"vocal", 2, app.EqualizerVocal},
		{"back to off", 3, app.EqualizerOff},
	} {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "settings.json")
			events := []app.Event{
				app.SelectPressed{}, app.SelectPressed{}, // Start the first track.
				app.BackPressed{}, app.Scroll{Delta: 2}, app.SelectPressed{},
				app.Scroll{Delta: 1},
			}
			for i := 0; i < test.presses; i++ {
				events = append(events, app.SelectPressed{})
			}
			events = append(events, app.VolumeChanged{Delta: 20}, app.QuitRequested{})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			firstPlayer := player.NewFake()
			first := app.New(firstPlayer, library.NewFake(), storage.NewFile(path), settingsInput{events: events}, silentUI{})
			if err := first.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if firstPlayer.Equalizer != test.want || firstPlayer.Volume != 70 || !firstPlayer.Playing || firstPlayer.Track.ID == "" {
				t.Fatalf("player after equalizer selection = %+v", firstPlayer)
			}
			settings, err := storage.NewFile(path).LoadSettings(ctx)
			if err != nil || settings != (app.SettingsState{Volume: 70, Equalizer: test.want}) {
				t.Fatalf("saved settings = %+v, %v", settings, err)
			}
			secondPlayer := player.NewFake()
			second := app.New(secondPlayer, library.NewFake(), storage.NewFile(path), settingsInput{
				events: []app.Event{app.QuitRequested{}},
			}, silentUI{})
			if err := second.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if secondPlayer.Equalizer != test.want || secondPlayer.Volume != 70 {
				t.Fatalf("restored player settings: equalizer=%q volume=%d", secondPlayer.Equalizer, secondPlayer.Volume)
			}
		})
	}
}

type failingEqualizerPlayer struct {
	*player.Fake
	calls  int
	failOn int
	err    error
	closed bool
}

func (p *failingEqualizerPlayer) SetEqualizer(ctx context.Context, preset app.EqualizerPreset) error {
	p.calls++
	if p.calls == p.failOn {
		return p.err
	}
	return p.Fake.SetEqualizer(ctx, preset)
}

func (p *failingEqualizerPlayer) Close() error {
	p.closed = true
	return nil
}

func TestEqualizerPlayerErrorsHaveContextAndClosePlayer(t *testing.T) {
	for _, test := range []struct {
		name   string
		failOn int
		want   string
	}{
		{"startup", 1, "set initial equalizer"},
		{"menu change", 2, "set equalizer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			sentinel := errors.New("equalizer unavailable")
			p := &failingEqualizerPlayer{Fake: player.NewFake(), failOn: test.failOn, err: sentinel}
			application := app.New(p, library.NewFake(), storage.NewMemory(50), settingsInput{events: []app.Event{
				app.Scroll{Delta: 2}, app.SelectPressed{}, app.Scroll{Delta: 1}, app.SelectPressed{},
			}}, silentUI{})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			err := application.Run(ctx)
			if !errors.Is(err, sentinel) || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error = %v, want wrapped equalizer error with %q", err, test.want)
			}
			if !p.closed {
				t.Fatal("player was not closed after equalizer failure")
			}
		})
	}
}
