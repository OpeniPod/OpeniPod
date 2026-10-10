package app_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/app"
	"github.com/OpeniPod/OpeniPod/internal/library"
	"github.com/OpeniPod/OpeniPod/internal/player"
)

type recordingStorage struct {
	saves []app.SettingsState
	saved chan app.SettingsState
	err   error
}

func (s *recordingStorage) LoadSettings(context.Context) (app.SettingsState, error) {
	return app.DefaultSettings(), nil
}
func (s *recordingStorage) SaveSettings(ctx context.Context, settings app.SettingsState) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.saves = append(s.saves, settings)
	if s.saved != nil {
		s.saved <- settings
	}
	return s.err
}

type inputFunc func(context.Context, chan<- app.Event) error

func (f inputFunc) Run(ctx context.Context, events chan<- app.Event) error { return f(ctx, events) }

type renderFunc func(app.AppState) error

func (f renderFunc) Render(s app.AppState) error { return f(s) }

func TestSettingsDebounceRestartsAfterLastChange(t *testing.T) {
	store := &recordingStorage{saved: make(chan app.SettingsState, 10)}
	rendered := make(chan int, 20)
	ui := renderFunc(func(s app.AppState) error { rendered <- s.Settings.Volume; return nil })
	input := inputFunc(func(ctx context.Context, events chan<- app.Event) error {
		send := func(e app.Event) error {
			select {
			case events <- e:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		waitVolume := func(want int) error {
			for {
				select {
				case got := <-rendered:
					if got == want {
						return nil
					}
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		pause := func(d time.Duration) error {
			timer := time.NewTimer(d)
			defer timer.Stop()
			select {
			case <-timer.C:
				return nil
			case <-store.saved:
				return errors.New("settings saved before debounce elapsed")
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := send(app.VolumeChanged{Delta: 10}); err != nil {
			return err
		}
		if err := waitVolume(60); err != nil {
			return err
		}
		if err := pause(300 * time.Millisecond); err != nil {
			return err
		}
		if err := send(app.VolumeChanged{Delta: 10}); err != nil {
			return err
		}
		if err := waitVolume(70); err != nil {
			return err
		}
		if err := pause(300 * time.Millisecond); err != nil {
			return err
		}
		select {
		case got := <-store.saved:
			if got.Volume != 70 {
				return errors.New("saved an intermediate volume")
			}
		case <-ctx.Done():
			return ctx.Err()
		}
		return send(app.QuitRequested{})
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	application := app.New(player.NewFake(), library.NewFake(), store, input, ui)
	if err := application.Run(ctx); err != nil {
		t.Fatal(err)
	}
	if ctx.Err() != nil {
		t.Fatal("timed out waiting for timer save")
	}
	if len(store.saves) != 1 || store.saves[0].Volume != 70 {
		t.Fatalf("saves = %+v", store.saves)
	}
}

func TestSettingsDoNotSaveUnchangedState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		events []app.Event
	}{
		{"renders only", []app.Event{app.Scroll{Delta: 1}, app.SelectPressed{}, app.BackPressed{}}},
		{"returned to saved value", []app.Event{app.VolumeChanged{Delta: 10}, app.VolumeChanged{Delta: -10}}},
		{"no change", []app.Event{app.VolumeChanged{Delta: 0}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &recordingStorage{}
			application := app.New(player.NewFake(), library.NewFake(), store, settingsInput{events: tc.events}, silentUI{})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			if err := application.Run(ctx); err != nil {
				t.Fatal(err)
			}
			if len(store.saves) != 0 {
				t.Fatalf("unexpected saves: %+v", store.saves)
			}
		})
	}
}

func TestSettingsFlushOnShutdown(t *testing.T) {
	saveErr := errors.New("disk unavailable")
	renderErr := errors.New("display unavailable")
	for _, mode := range []string{"quit", "EOF", "cancel", "save error", "render error"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			store := &recordingStorage{}
			if mode == "save error" {
				store.err = saveErr
			}
			ui := renderFunc(func(s app.AppState) error {
				if s.Settings.Volume == 70 {
					if mode == "cancel" {
						cancel()
					}
					if mode == "render error" {
						return renderErr
					}
				}
				return nil
			})
			input := inputFunc(func(ctx context.Context, events chan<- app.Event) error {
				if err := (settingsInput{events: []app.Event{app.VolumeChanged{Delta: 10}, app.VolumeChanged{Delta: 10}}}).Run(ctx, events); err != nil {
					return err
				}
				if mode == "cancel" || mode == "render error" {
					<-ctx.Done()
					return ctx.Err()
				}
				if mode == "quit" || mode == "save error" {
					select {
					case events <- app.QuitRequested{}:
					case <-ctx.Done():
						return ctx.Err()
					}
				}
				return nil
			})
			application := app.New(player.NewFake(), library.NewFake(), store, input, ui)
			err := application.Run(ctx)
			switch mode {
			case "save error":
				if !errors.Is(err, saveErr) {
					t.Fatalf("error = %v", err)
				}
			case "render error":
				if !errors.Is(err, renderErr) {
					t.Fatalf("error = %v", err)
				}
			default:
				if err != nil {
					t.Fatal(err)
				}
			}
			if len(store.saves) != 1 || store.saves[0].Volume != 70 {
				t.Fatalf("saves = %+v", store.saves)
			}
		})
	}
}
