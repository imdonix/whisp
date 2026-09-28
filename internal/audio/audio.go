// Package audio records 16 kHz mono float32 audio from the microphone.
package audio

import (
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"sync"

	"github.com/gen2brain/malgo"
)

const SampleRate = 16000

type Recorder struct {
	ctx    *malgo.AllocatedContext
	device *malgo.Device

	mu        sync.Mutex
	recording bool
	buf       []float32
}

// New opens the microphone. mic is an optional device-name substring.
// The device stream runs continuously; samples are only kept while recording,
// which avoids clipping the first syllable on device start-up.
func New(mic string) (*Recorder, error) {
	return open(nil, func(ctx *malgo.AllocatedContext, cfg *malgo.DeviceConfig) error {
		if mic == "" {
			return nil
		}
		infos, err := ctx.Devices(malgo.Capture)
		if err != nil {
			return err
		}
		for _, info := range infos {
			if strings.Contains(strings.ToLower(info.Name()), strings.ToLower(mic)) {
				id := info.ID
				cfg.Capture.DeviceID = id.Pointer()
				return nil
			}
		}
		return fmt.Errorf("no capture device matching %q", mic)
	})
}

// NewMonitor captures what is currently played on the default output device
// (speakers or headset), via PulseAudio's @DEFAULT_MONITOR@ source.
func NewMonitor() (*Recorder, error) {
	// Only PulseAudio (PipeWire's pulse layer) exposes output monitors.
	return open([]malgo.Backend{malgo.BackendPulseaudio}, func(_ *malgo.AllocatedContext, cfg *malgo.DeviceConfig) error {
		var id malgo.DeviceID
		copy(id[:], "@DEFAULT_MONITOR@")
		cfg.Capture.DeviceID = id.Pointer()
		return nil
	})
}

func open(backends []malgo.Backend, selectDevice func(*malgo.AllocatedContext, *malgo.DeviceConfig) error) (*Recorder, error) {
	ctx, err := malgo.InitContext(backends, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("audio context: %w", err)
	}
	r := &Recorder{ctx: ctx}

	cfg := malgo.DefaultDeviceConfig(malgo.Capture)
	cfg.Capture.Format = malgo.FormatF32
	cfg.Capture.Channels = 1
	cfg.SampleRate = SampleRate

	if err := selectDevice(ctx, &cfg); err != nil {
		r.Close()
		return nil, err
	}
	dev, err := malgo.InitDevice(ctx.Context, cfg, malgo.DeviceCallbacks{Data: r.onData})
	if err != nil {
		r.Close()
		return nil, fmt.Errorf("audio device: %w", err)
	}
	r.device = dev
	if err := dev.Start(); err != nil {
		r.Close()
		return nil, fmt.Errorf("audio start: %w", err)
	}
	return r, nil
}

func (r *Recorder) onData(_, in []byte, frames uint32) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.recording {
		return
	}
	for i := 0; i+4 <= len(in); i += 4 {
		r.buf = append(r.buf, math.Float32frombits(binary.LittleEndian.Uint32(in[i:])))
	}
}

func (r *Recorder) Start() {
	r.mu.Lock()
	r.buf = r.buf[:0]
	r.recording = true
	r.mu.Unlock()
}

// Take returns the samples captured so far and clears the buffer, while
// recording continues. Used for long recordings transcribed in chunks.
func (r *Recorder) Take() []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]float32, len(r.buf))
	copy(out, r.buf)
	r.buf = r.buf[:0]
	return out
}

// Stop ends recording and returns the captured samples.
func (r *Recorder) Stop() []float32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.recording = false
	out := make([]float32, len(r.buf))
	copy(out, r.buf)
	return out
}

func (r *Recorder) Close() {
	if r.device != nil {
		r.device.Uninit()
	}
	if r.ctx != nil {
		r.ctx.Uninit()
		r.ctx.Free()
	}
}
