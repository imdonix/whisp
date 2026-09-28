package meeting

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tamasmagyar/whisp/internal/config"
	"github.com/tamasmagyar/whisp/internal/stt"
)

// TestPipeline runs a simulated quiet ~40 s recording (speech on the mic,
// nothing on the system audio) through mixer, chunker, speech gate and whisper.
func TestPipeline(t *testing.T) {
	model := config.ModelPath(config.Default().Model)
	if _, err := os.Stat(model); err != nil {
		t.Skip("model not found:", model)
	}
	speech := jfk(t, 0.1, 0.003)
	var mic []float32
	for i := 0; i < 3; i++ {
		mic = append(mic, speech...)
		mic = append(mic, noise(2*rate, 0.003)...)
	}
	mic = append(mic, noise(10*rate, 0.003)...) // trailing silence: must be skipped

	var m Mixer
	var c Chunker
	var chunks [][]float32
	for i := 0; i < len(mic); i += rate / 2 { // 500 ms ticks, like the app
		end := min(i+rate/2, len(mic))
		chunks = append(chunks, c.Add(m.Add(mic[i:end], make([]float32, end-i)))...)
	}
	chunks = append(chunks, c.Add(m.Flush())...)
	chunks = append(chunks, c.Flush())

	e, err := stt.Load(model, "en", 4, "")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	var text []string
	for _, ch := range chunks {
		if !HasSpeech(ch) {
			t.Logf("skipped %.1fs chunk (no speech)", float64(len(ch))/rate)
			continue
		}
		out, err := e.Transcribe(ch)
		if err != nil {
			t.Fatal(err)
		}
		t.Logf("%.1fs chunk: %q", float64(len(ch))/rate, out)
		text = append(text, out)
	}
	all := strings.ToLower(strings.Join(text, " "))
	if n := strings.Count(all, "ask not what your country can do for you"); n != 3 {
		t.Errorf("phrase found %d times, want 3:\n%s", n, all)
	}

	// Sentence timestamps: chunk offset (all chunks so far, including
	// skipped ones, as in the app) + the sentence's start within the chunk.
	// The speech starts at 0 s, 13 s and 26 s.
	var starts []time.Duration
	emitted := 0
	for _, ch := range chunks {
		offset := time.Duration(emitted) * time.Second / rate
		emitted += len(ch)
		if !HasSpeech(ch) {
			continue
		}
		ch, cut := TrimLeadingSilence(ch)
		offset += time.Duration(cut) * time.Second / rate
		segs, err := e.Segments(ch)
		if err != nil {
			t.Fatal(err)
		}
		for _, seg := range segs {
			at := offset + seg.Start
			t.Logf("%s %s", Timestamp("elapsed", time.Time{}, at), seg.Text)
			if strings.HasPrefix(strings.ToLower(seg.Text), "and so") {
				starts = append(starts, at)
			}
		}
	}
	want := []time.Duration{0, 13 * time.Second, 26 * time.Second}
	if len(starts) != len(want) {
		t.Fatalf("found %d sentences starting with \"And so\", want 3", len(starts))
	}
	for i := range want {
		if d := starts[i] - want[i]; d < -time.Second || d > 1500*time.Millisecond {
			t.Errorf("sentence %d at %v, want about %v", i, starts[i], want[i])
		}
	}
}
