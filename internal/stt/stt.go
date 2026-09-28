// Package stt wraps whisper.cpp for one-shot transcription of short clips.
package stt

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

type Engine struct {
	mu       sync.Mutex
	model    whisper.Model
	language string
	threads  uint
	prompt   string
}

func Load(path, language string, threads uint, prompt string) (*Engine, error) {
	selectGPU()
	m, err := whisper.New(path)
	if err != nil {
		return nil, fmt.Errorf("load model %s: %w", path, err)
	}
	return &Engine{model: m, language: language, threads: threads, prompt: prompt}, nil
}

func (e *Engine) Close() error { return e.model.Close() }

// Segment is a piece of transcribed text with its position in the clip.
type Segment struct {
	Start, End time.Duration
	Text       string
}

// Transcribe converts 16 kHz mono float32 samples to text.
func (e *Engine) Transcribe(samples []float32) (string, error) {
	segs, err := e.Segments(samples)
	if err != nil {
		return "", err
	}
	parts := make([]string, len(segs))
	for i, s := range segs {
		parts[i] = s.Text
	}
	return Clean(strings.Join(parts, " ")), nil
}

// Segments transcribes 16 kHz mono float32 samples into timed sentences.
// Whisper's own segments can span a whole clip, so they are split at sentence
// ends using per-token timestamps. Empty sentences (after Clean) are dropped.
func (e *Engine) Segments(samples []float32) ([]Segment, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	ctx, err := e.model.NewContext()
	if err != nil {
		return nil, err
	}
	if err := ctx.SetLanguage(e.language); err != nil {
		return nil, err
	}
	ctx.SetThreads(e.threads)
	ctx.SetTranslate(false)
	ctx.SetTokenTimestamps(true)
	if e.prompt != "" {
		ctx.SetInitialPrompt(e.prompt)
	}

	// Whisper expects at least 1s of audio; pad short clips with silence.
	if len(samples) < 16000+1600 {
		samples = append(samples, make([]float32, 16000+1600-len(samples))...)
	}
	if err := ctx.Process(samples, nil, nil, nil); err != nil {
		return nil, err
	}

	var segs []Segment
	add := func(start, end time.Duration, text string) {
		if text = Clean(text); text != "" {
			segs = append(segs, Segment{Start: start, End: end, Text: text})
		}
	}
	for {
		seg, err := ctx.NextSegment()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return nil, err
		}
		var words []whisper.Token
		for _, tok := range seg.Tokens {
			if ctx.IsText(tok) {
				words = append(words, tok)
			}
		}
		if len(words) == 0 {
			add(seg.Start, seg.End, seg.Text)
			continue
		}
		for _, s := range SplitSentences(words) {
			add(s.Start, s.End, s.Text)
		}
	}
	return segs, nil
}

// SplitSentences groups text tokens into sentences ending in ".", "?" or "!".
func SplitSentences(tokens []whisper.Token) []Segment {
	var out []Segment
	var text strings.Builder
	var start time.Duration
	for i, tok := range tokens {
		if text.Len() == 0 {
			start = tok.Start
		}
		text.WriteString(tok.Text)
		t := strings.TrimSpace(tok.Text)
		if strings.HasSuffix(t, ".") || strings.HasSuffix(t, "?") || strings.HasSuffix(t, "!") || i == len(tokens)-1 {
			out = append(out, Segment{Start: start, End: tok.End, Text: text.String()})
			text.Reset()
		}
	}
	return out
}

// Non-speech annotations whisper emits, e.g. [BLANK_AUDIO], (music), *coughs*.
var artefacts = regexp.MustCompile(`\[[^\]]*\]|\([^)]*\)|\*[^*]*\*`)
var spaces = regexp.MustCompile(`\s+`)

func Clean(s string) string {
	s = artefacts.ReplaceAllString(s, " ")
	s = spaces.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}
