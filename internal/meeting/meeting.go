// Package meeting turns a continuous recording (microphone + system audio)
// into chunks for transcription and appends the transcript to a text file.
package meeting

import (
	"fmt"
	"math"
	"os"
	"slices"
	"time"

	"github.com/tamasmagyar/whisp/internal/audio"
)

const (
	rate = audio.SampleRate

	// Chunks are cut at a pause once they are at least minChunk long, and
	// forced at maxChunk (whisper processes 30 s windows).
	minChunk = 8 * rate
	maxChunk = 28 * rate

	// 400 ms close to the background level, and well below the typical
	// level, counts as a pause.
	pauseWindow = rate * 4 / 10
	pauseRatio  = 2

	// A 100 ms window counts as speech when it is clearly louder than the
	// chunk's background noise and above an absolute floor (digital silence,
	// faint hiss). Chunks with fewer than minSpeech such windows are skipped.
	speechWindow = rate / 10
	speechFloor  = 0.004
	speechRatio  = 3
	minSpeech    = 3

	// If one source falls this far behind (e.g. a suspended output device
	// delivers no monitor data), it is padded with silence.
	maxLag = rate
)

// Mixer sums two streams that arrive in independently sized pieces.
type Mixer struct{ a, b []float32 }

// Add appends new samples of both sources and returns the mixed samples that
// are available from both.
func (m *Mixer) Add(a, b []float32) []float32 {
	m.a = append(m.a, a...)
	m.b = append(m.b, b...)
	if len(m.a) > len(m.b)+maxLag {
		m.b = append(m.b, make([]float32, len(m.a)-len(m.b))...)
	} else if len(m.b) > len(m.a)+maxLag {
		m.a = append(m.a, make([]float32, len(m.b)-len(m.a))...)
	}
	n := min(len(m.a), len(m.b))
	out := make([]float32, n)
	for i := range out {
		out[i] = max(-1, min(1, m.a[i]+m.b[i]))
	}
	m.a = append(m.a[:0], m.a[n:]...)
	m.b = append(m.b[:0], m.b[n:]...)
	return out
}

// Flush returns everything left, padding the shorter source with silence.
func (m *Mixer) Flush() []float32 {
	out := m.Add(make([]float32, max(0, len(m.b)-len(m.a))), make([]float32, max(0, len(m.a)-len(m.b))))
	m.a, m.b = nil, nil
	return out
}

// Chunker splits a stream into chunks, preferably at pauses in speech.
type Chunker struct{ buf []float32 }

// Add appends samples and returns any complete chunks.
func (c *Chunker) Add(samples []float32) [][]float32 {
	c.buf = append(c.buf, samples...)
	var chunks [][]float32
	for len(c.buf) >= minChunk {
		cut := findPause(c.buf, minChunk, min(len(c.buf), maxChunk))
		if cut < 0 {
			if len(c.buf) < maxChunk {
				break
			}
			cut = maxChunk
		}
		chunks = append(chunks, append([]float32(nil), c.buf[:cut]...))
		c.buf = append(c.buf[:0], c.buf[cut:]...)
	}
	return chunks
}

// Flush returns the remaining samples as a final chunk (may be empty).
func (c *Chunker) Flush() []float32 {
	out := c.buf
	c.buf = nil
	return out
}

// findPause returns the middle of the first quiet window starting in
// [from, to), or -1.
func findPause(s []float32, from, to int) int {
	levels := windowLevels(s[:to])
	slices.Sort(levels)
	noise, median := levels[len(levels)/10], levels[len(levels)/2]
	threshold := math.Max(speechFloor, math.Min(noise*pauseRatio, median/2))
	for i := from; i+pauseWindow <= to; i += pauseWindow / 4 {
		if rms(s[i:i+pauseWindow]) < threshold {
			return i + pauseWindow/2
		}
	}
	return -1
}

// windowLevels returns the RMS of consecutive 100 ms windows.
func windowLevels(s []float32) []float64 {
	var levels []float64
	for i := 0; i+speechWindow <= len(s); i += speechWindow {
		levels = append(levels, rms(s[i:i+speechWindow]))
	}
	return levels
}

// noiseLevel estimates the background level as the 20th percentile of the
// window levels.
func noiseLevel(s []float32) float64 {
	levels := windowLevels(s)
	if len(levels) == 0 {
		return 0
	}
	slices.Sort(levels)
	return levels[len(levels)/5]
}

// TrimLeadingSilence drops the quiet start of a chunk (keeping 200 ms before
// the first speech) and returns the rest plus the number of samples dropped.
// Whisper tends to time the first words at the start of its input, so
// leading silence would make sentence timestamps too early.
func TrimLeadingSilence(s []float32) ([]float32, int) {
	threshold := math.Max(speechFloor, noiseLevel(s)*speechRatio)
	for i, l := range windowLevels(s) {
		if l >= threshold {
			cut := max(0, (i-2)*speechWindow)
			return s[cut:], cut
		}
	}
	return s, 0
}

// HasSpeech reports whether a chunk contains enough audio above its
// background noise to be worth transcribing; whisper tends to invent text
// ("Thank you.") for silence.
func HasSpeech(s []float32) bool {
	levels := windowLevels(s)
	if len(levels) < minSpeech {
		return false
	}
	threshold := math.Max(speechFloor, noiseLevel(s)*speechRatio)
	n := 0
	for _, l := range levels {
		if l >= threshold {
			n++
		}
	}
	return n >= minSpeech
}

func rms(s []float32) float64 {
	var sum float64
	for _, v := range s {
		sum += float64(v) * float64(v)
	}
	return math.Sqrt(sum / float64(max(1, len(s))))
}

const stamp = "2006-01-02_15-04-05"

// InProgressName is the transcript file name while a recording is running;
// the end time isn't known yet.
func InProgressName(start time.Time) string {
	return start.Format(stamp) + "_recording.txt"
}

// FileName is the final transcript file name, e.g.
// 2026-09-26_14-03-12_to_14-41-55.txt; the end date is included only when
// the recording ran past midnight.
func FileName(start, end time.Time) string {
	endFmt := "15-04-05"
	if end.Format("2006-01-02") != start.Format("2006-01-02") {
		endFmt = stamp
	}
	return start.Format(stamp) + "_to_" + end.Format(endFmt) + ".txt"
}

// Timestamp formats the time of a sentence spoken at offset into a recording
// that started at start, as "[14:12:37]" (mode "clock") or "[00:09:25]"
// (mode "elapsed").
func Timestamp(mode string, start time.Time, offset time.Duration) string {
	if mode == "elapsed" {
		s := int(offset.Round(time.Second) / time.Second)
		return fmt.Sprintf("[%02d:%02d:%02d]", s/3600, s/60%60, s%60)
	}
	return start.Add(offset).Format("[15:04:05]")
}

// ValidTimestampMode reports whether mode is a supported meeting_timestamps value.
func ValidTimestampMode(mode string) bool {
	return mode == "clock" || mode == "elapsed" || mode == "none"
}

// Append adds a line to the transcript file, creating it if needed.
func Append(path, line string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintln(f, line); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func StartedHeader(t time.Time) string {
	return fmt.Sprintf("===== Recording started %s =====", t.Format("2006-01-02 15:04:05"))
}

func StoppedFooter(t time.Time) string {
	return fmt.Sprintf("===== Recording stopped %s =====", t.Format("2006-01-02 15:04:05"))
}
