package app

import (
	"reflect"
	"testing"
	"time"

	"github.com/OpeniPod/OpeniPod/internal/music"
)

func TestSelectingEqualizerCyclesAndRequestsEffects(t *testing.T) {
	for _, test := range []struct {
		from, want EqualizerPreset
	}{
		{EqualizerOff, EqualizerBassBoost},
		{EqualizerBassBoost, EqualizerVocal},
		{EqualizerVocal, EqualizerOff},
	} {
		t.Run(test.from.String(), func(t *testing.T) {
			state := AppState{
				Screen:     ScreenSettings,
				Navigation: NavigationState{SettingsIndex: 1},
				Settings:   SettingsState{Volume: 70, Equalizer: test.from},
				Player: PlayerState{
					Track: music.Track{ID: "current"}, HasTrack: true,
					Status: StatusPaused, Position: 30 * time.Second,
				},
			}
			got, commands := Reduce(state, SelectPressed{})
			want := state
			want.Settings.Equalizer = test.want
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("selected equalizer state = %+v, want %+v", got, want)
			}
			wantCommands := []command{setEqualizer{test.want}, saveSettings{}}
			if !reflect.DeepEqual(commands, wantCommands) {
				t.Fatalf("commands = %#v, want %#v", commands, wantCommands)
			}
		})
	}
}

func TestSettingsLoadedNormalizesEqualizer(t *testing.T) {
	for _, preset := range []EqualizerPreset{EqualizerOff, EqualizerBassBoost, EqualizerVocal, "unknown"} {
		t.Run(string(preset), func(t *testing.T) {
			got, commands := Reduce(AppState{}, SettingsLoaded{Settings: SettingsState{Volume: 70, Equalizer: preset}})
			want := preset
			if preset == "unknown" {
				want = EqualizerOff
			}
			if got.Settings != (SettingsState{Volume: 70, Equalizer: want}) || len(commands) != 0 {
				t.Fatalf("settings = %+v, commands = %#v", got.Settings, commands)
			}
		})
	}
}

func TestVolumeChangePreservesEqualizer(t *testing.T) {
	state := AppState{Settings: SettingsState{Volume: 70, Equalizer: EqualizerVocal}}
	got, commands := Reduce(state, VolumeChanged{Delta: 10})
	want := SettingsState{Volume: 80, Equalizer: EqualizerVocal}
	if got.Settings != want || !reflect.DeepEqual(commands, []command{setVolume{80}, saveSettings{}}) {
		t.Fatalf("settings = %+v, commands = %#v, want %+v", got.Settings, commands, want)
	}
}
