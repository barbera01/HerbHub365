package tts

// Voice represents a resolved Kokoro voice string and its default speed.
type Voice struct {
	VoiceString string
	Speed       float64
}

// friendlyVoices maps short friendly names to their Kokoro voice strings.
var friendlyVoices = map[string]Voice{
	// Eve: warm British female blend
	"eve": {
		VoiceString: "bf_lily(7)+bf_emma(2)+af_bella(1)+af_heart(1)",
		Speed:       0.95,
	},
	// Rowan: warm British male blend
	"rowan": {
		VoiceString: "bm_daniel(7)+bm_lewis(3)",
		Speed:       0.95,
	},
}

// ResolveVoice takes either a friendly name ("eve", "rowan") or a raw Kokoro
// voice string and returns the resolved Voice. If the name is not a known
// friendly name it is returned as-is with speed 1.0.
// If overrideSpeed > 0 it takes precedence over the voice default.
func ResolveVoice(name string, overrideSpeed float64) Voice {
	if v, ok := friendlyVoices[name]; ok {
		if overrideSpeed > 0 {
			v.Speed = overrideSpeed
		}
		return v
	}
	// Raw voice string passthrough.
	speed := overrideSpeed
	if speed <= 0 {
		speed = 1.0
	}
	return Voice{VoiceString: name, Speed: speed}
}
