package stt

// Device is a GPU as reported by the graphics API, in enumeration order.
type Device struct {
	Name     string
	Discrete bool
}

// pickGPU returns the index of the device whisper should use: the first
// discrete GPU, so a desktop's integrated GPU (e.g. a Ryzen iGPU next to a
// Radeon card) isn't picked just because it is listed first. -1 means no
// preference (no discrete GPU; whisper then takes the first device).
func pickGPU(devices []Device) int {
	for i, d := range devices {
		if d.Discrete {
			return i
		}
	}
	return -1
}
