package meeting

import (
	"math"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/go-audio/wav"
)

func tone(n int, amp float32) []float32 {
	s := make([]float32, n)
	for i := range s {
		s[i] = amp * float32(math.Sin(float64(i)*2*math.Pi*220/rate))
	}
	return s
}

func TestMixerAlignsAndPads(t *testing.T) {
	var m Mixer
	if out := m.Add(tone(100, 0.1), nil); len(out) != 0 {
		t.Fatalf("mixed %d samples before the second source arrived", len(out))
	}
	if out := m.Add(nil, tone(60, 0.1)); len(out) != 60 {
		t.Fatalf("got %d samples, want 60", len(out))
	}
	// A source that stops delivering (suspended output) must not stall the other.
	if out := m.Add(tone(2*rate, 0.1), nil); len(out) != 2*rate+40 {
		t.Fatalf("got %d samples, want %d", len(out), 2*rate+40)
	}
	if out := m.Flush(); len(out) != 0 {
		t.Fatalf("flush returned %d samples", len(out))
	}
}

func TestChunkerCutsAtPause(t *testing.T) {
	var c Chunker
	speech := append(tone(10*rate, 0.3), make([]float32, rate)...) // 10 s speech, 1 s pause
	chunks := c.Add(append(speech, tone(5*rate, 0.3)...))
	if len(chunks) != 1 {
		t.Fatalf("got %d chunks, want 1", len(chunks))
	}
	if n := len(chunks[0]); n < 10*rate || n > 11*rate {
		t.Fatalf("chunk is %.2fs, want a cut inside the pause", float64(n)/rate)
	}
	if n := len(c.Flush()); n < 5*rate {
		t.Fatalf("remainder %d samples", n)
	}
}

func TestChunkerForcesCutWithoutPause(t *testing.T) {
	var c Chunker
	chunks := c.Add(tone(60*rate, 0.3))
	if len(chunks) != 2 || len(chunks[0]) != maxChunk {
		t.Fatalf("got %d chunks", len(chunks))
	}
}

func noise(n int, amp float32) []float32 {
	r := rand.New(rand.NewSource(1))
	s := make([]float32, n)
	for i := range s {
		s[i] = amp * float32(r.NormFloat64())
	}
	return s
}

// jfk returns whisper.cpp's speech sample scaled by gain, on top of room noise.
func jfk(t *testing.T, gain float32, noiseRMS float32) []float32 {
	f, err := os.Open("../../third_party/whisper.cpp/samples/jfk.wav")
	if err != nil {
		t.Skip(err)
	}
	defer f.Close()
	buf, err := wav.NewDecoder(f).FullPCMBuffer()
	if err != nil {
		t.Fatal(err)
	}
	s := noise(len(buf.Data), noiseRMS)
	for i, v := range buf.Data {
		s[i] += gain * float32(v) / 32768
	}
	return s
}

func TestHasSpeech(t *testing.T) {
	if HasSpeech(make([]float32, 10*rate)) {
		t.Error("digital silence detected as speech")
	}
	// Room noise as measured on the reference laptop mic (RMS ~0.003).
	if HasSpeech(noise(10*rate, 0.003)) {
		t.Error("room noise detected as speech")
	}
	if HasSpeech(noise(10*rate, 0.02)) {
		t.Error("loud steady noise (fan) detected as speech")
	}
	if !HasSpeech(append(make([]float32, 5*rate), tone(rate, 0.2)...)) {
		t.Error("loud sound after silence not detected")
	}
	if !HasSpeech(jfk(t, 1, 0.003)) {
		t.Error("normal speech not detected")
	}
	if !HasSpeech(jfk(t, 0.1, 0.003)) {
		t.Error("quiet speech (20 dB down) not detected")
	}
}

func TestChunkerCutsAtPauseInQuietSpeech(t *testing.T) {
	var c Chunker
	speech := jfk(t, 0.1, 0.003) // 11 s, quiet
	pause := noise(rate, 0.003)
	chunks := c.Add(append(append(speech, pause...), speech...))
	if len(chunks) == 0 {
		t.Fatal("no chunk")
	}
	// The sample has short gaps between phrases; any cut must be inside a gap
	// or the long pause, i.e. at a quiet spot.
	for _, ch := range chunks {
		end := ch[len(ch)-speechWindow:]
		if l := rms(end); l > 0.01 {
			t.Errorf("chunk of %.1fs ends in speech (rms %.4f)", float64(len(ch))/rate, l)
		}
	}
}

func TestFileName(t *testing.T) {
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	start := at("2026-09-26 14:03:12")
	if got := InProgressName(start); got != "2026-09-26_14-03-12_recording.txt" {
		t.Errorf("InProgressName = %s", got)
	}
	if got := FileName(start, at("2026-09-26 14:41:55")); got != "2026-09-26_14-03-12_to_14-41-55.txt" {
		t.Errorf("same day: %s", got)
	}
	if got := FileName(at("2026-09-26 23:50:00"), at("2026-09-27 00:20:05")); got != "2026-09-26_23-50-00_to_2026-09-27_00-20-05.txt" {
		t.Errorf("past midnight: %s", got)
	}
}

func TestTimestamp(t *testing.T) {
	start := time.Date(2026, 9, 26, 14, 3, 12, 0, time.Local)
	offset := 9*time.Minute + 25*time.Second + 400*time.Millisecond
	if got := Timestamp("clock", start, offset); got != "[14:12:37]" {
		t.Errorf("clock: %s", got)
	}
	if got := Timestamp("elapsed", start, offset); got != "[00:09:25]" {
		t.Errorf("elapsed: %s", got)
	}
	if got := Timestamp("elapsed", start, 2*time.Hour+time.Second); got != "[02:00:01]" {
		t.Errorf("elapsed over an hour: %s", got)
	}
	if got := Timestamp("clock", time.Date(2026, 9, 26, 23, 59, 50, 0, time.Local), 20*time.Second); got != "[00:00:10]" {
		t.Errorf("clock past midnight: %s", got)
	}
}

func TestTrimLeadingSilence(t *testing.T) {
	s := append(noise(3*rate, 0.003), tone(2*rate, 0.2)...)
	rest, cut := TrimLeadingSilence(s)
	if cut < 2*rate || cut > 3*rate || len(rest)+cut != len(s) {
		t.Errorf("cut %.2fs, want just under 3s", float64(cut)/rate)
	}
	if _, cut := TrimLeadingSilence(tone(2*rate, 0.2)); cut != 0 {
		t.Errorf("speech from the start was trimmed by %d samples", cut)
	}
}
