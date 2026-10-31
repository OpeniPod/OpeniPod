package app

// EqualizerPreset selects a fixed tone profile. The zero value bypasses EQ,
// including when loading settings files written before equalizer support.
type EqualizerPreset string

const (
	EqualizerOff       EqualizerPreset = ""
	EqualizerBassBoost EqualizerPreset = "bass_boost"
	EqualizerVocal     EqualizerPreset = "vocal"
)

func (p EqualizerPreset) Valid() bool {
	return p == EqualizerOff || p == EqualizerBassBoost || p == EqualizerVocal
}

func (p EqualizerPreset) String() string {
	switch p {
	case EqualizerBassBoost:
		return "Bass boost"
	case EqualizerVocal:
		return "Vocal"
	default:
		return "Off"
	}
}
