package tts

type Voice struct {
	VoiceString string
	Speed       float64
}

var friendlyVoices = map[string]Voice{
	"eve": {
		VoiceString: "bf_lily(7)+bf_emma(2)+af_bella(1)+af_heart(1)",
		Speed:       0.95,
	},
	"rowan": {
		VoiceString: "bm_daniel(7)+bm_lewis(3)",
		Speed:       0.95,
	},
}

func ResolveVoice(name string, overrideSpeed float64) Voice {
	if v, ok := friendlyVoices[name]; ok {
		if overrideSpeed > 0 {
			v.Speed = overrideSpeed
		}
		return v
	}
	speed := overrideSpeed
	if speed <= 0 {
		speed = 1.0
	}
	return Voice{VoiceString: name, Speed: speed}
}
