package stt

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/go-audio/wav"
	"github.com/tamasmagyar/whisp/internal/config"
)

func TestClean(t *testing.T) {
	cases := map[string]string{
		" Hello world.":             "Hello world.",
		"[BLANK_AUDIO]":             "",
		" (music) Hi there [noise]": "Hi there",
	}
	for in, want := range cases {
		if got := Clean(in); got != want {
			t.Errorf("Clean(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitSentences(t *testing.T) {
	tok := func(text string, start int) whisper.Token {
		return whisper.Token{Text: text, Start: time.Duration(start) * time.Second, End: time.Duration(start+1) * time.Second}
	}
	got := SplitSentences([]whisper.Token{
		tok(" Hello", 0), tok(" there", 1), tok(".", 2),
		tok(" How", 3), tok(" are", 4), tok(" you", 5), tok("?", 6),
		tok(" Fine", 8),
	})
	want := []Segment{
		{0, 3 * time.Second, " Hello there."},
		{3 * time.Second, 7 * time.Second, " How are you?"},
		{8 * time.Second, 9 * time.Second, " Fine"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %+v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("sentence %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestJFK(t *testing.T) {
	model := os.Getenv("WHISP_TEST_MODEL")
	if model == "" {
		model = config.ModelPath(config.Default().Model)
	}
	if _, err := os.Stat(model); err != nil {
		t.Skip("model not found:", model)
	}
	f, err := os.Open("../../third_party/whisper.cpp/samples/jfk.wav")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	buf, err := wav.NewDecoder(f).FullPCMBuffer()
	if err != nil {
		t.Fatal(err)
	}
	e, err := Load(model, "en", 4, "")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	samples := make([]float32, len(buf.Data))
	for i, v := range buf.Data {
		samples[i] = float32(v) / 32768
	}
	text, err := e.Transcribe(samples)
	if err != nil {
		t.Fatal(err)
	}
	t.Log(text)
	if !strings.Contains(strings.ToLower(text), "ask not what your country can do for you") {
		t.Errorf("unexpected transcript: %q", text)
	}
}

func TestJFKSegments(t *testing.T) {
	model := os.Getenv("WHISP_TEST_MODEL")
	if model == "" {
		model = config.ModelPath(config.Default().Model)
	}
	if _, err := os.Stat(model); err != nil {
		t.Skip("model not found:", model)
	}
	f, err := os.Open("../../third_party/whisper.cpp/samples/jfk.wav")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	buf, err := wav.NewDecoder(f).FullPCMBuffer()
	if err != nil {
		t.Fatal(err)
	}
	samples := make([]float32, len(buf.Data))
	for i, v := range buf.Data {
		samples[i] = float32(v) / 32768
	}
	e, err := Load(model, "en", 4, "")
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	segs, err := e.Segments(samples)
	if err != nil {
		t.Fatal(err)
	}
	if len(segs) == 0 {
		t.Fatal("no segments")
	}
	clip := time.Duration(len(samples)) * time.Second / 16000
	var prev time.Duration = -1
	for _, s := range segs {
		t.Logf("%v-%v %q", s.Start, s.End, s.Text)
		if s.Start < prev || s.End < s.Start || s.Start > clip {
			t.Errorf("bad timing %v-%v (clip %v)", s.Start, s.End, clip)
		}
		prev = s.Start
	}
}
