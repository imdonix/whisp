// Package config loads whisp settings from ~/.wisp/config.yaml.
package config

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	// Model is a model name (e.g. "large-v3-turbo-q5_0") resolved in ModelsDir, or an absolute path.
	Model    string `yaml:"model"`
	Language string `yaml:"language"`
	// Key is an evdev key name, e.g. "KEY_F9".
	Key string `yaml:"key"`
	// MeetingKey toggles meeting recording (microphone + system audio,
	// transcribed continuously into a new file in MeetingsDir).
	MeetingKey string `yaml:"meeting_key"`
	// MeetingsDir holds one transcript file per meeting; "~/" expands to the
	// home directory.
	MeetingsDir string `yaml:"meetings_dir"`
	// MeetingTimestamps prefixes each sentence of a meeting transcript with
	// its time: "clock" (14:12:37), "elapsed" (since the start, 00:09:25) or
	// "none". Dictation is never timestamped.
	MeetingTimestamps string `yaml:"meeting_timestamps"`
	// Paste is the key combo sent after the text is put on the clipboard.
	// "shift+insert" works in Chrome and GNOME Terminal; "ctrl+v" or "ctrl+shift+v" are alternatives.
	Paste string `yaml:"paste"`
	// RestoreClipboard puts the previous clipboard contents back after pasting.
	RestoreClipboard bool `yaml:"restore_clipboard"`
	// MinDuration: recordings shorter than this are discarded (accidental taps).
	MinDuration Duration `yaml:"min_duration"`
	// Threads used by whisper for the CPU parts (0 = auto).
	Threads uint `yaml:"threads"`
	// Prompt is an optional initial prompt to bias vocabulary/punctuation.
	Prompt string `yaml:"prompt"`
	// TrailingSpace appends a space so consecutive dictations don't run together.
	TrailingSpace bool `yaml:"trailing_space"`
	// Mic is an optional capture device name substring; empty = system default.
	Mic string `yaml:"mic"`
}

type Duration struct{ time.Duration }

func (d *Duration) UnmarshalText(b []byte) error {
	v, err := time.ParseDuration(string(b))
	d.Duration = v
	return err
}

func (d Duration) MarshalText() ([]byte, error) { return []byte(d.String()), nil }

func Default() Config {
	return Config{
		Model:             "large-v3-turbo-q5_0",
		Language:          "auto",
		Key:               "KEY_F9",
		MeetingKey:        "KEY_F10",
		MeetingsDir:       "~/.wisp",
		MeetingTimestamps: "clock",
		Paste:             "shift+insert",
		RestoreClipboard:  true,
		MinDuration:       Duration{300 * time.Millisecond},
		Threads:           0,
		TrailingSpace:     true,
	}
}

// Dir is whisp's home folder (~/.wisp, or $WISP_HOME), holding the config
// file and, by default, the meeting transcripts.
func Dir() string {
	if d := os.Getenv("WISP_HOME"); d != "" {
		return d
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".wisp")
}

func DataDir() string {
	if d := os.Getenv("XDG_DATA_HOME"); d != "" {
		return filepath.Join(d, "whisp")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "whisp")
}

// LogPath is where the background instance logs (~/.wisp/whisp.log).
func LogPath() string { return filepath.Join(Dir(), "whisp.log") }

// ExpandHome replaces a leading "~/" with the home directory.
func ExpandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, _ := os.UserHomeDir()
		return filepath.Join(home, path[2:])
	}
	return path
}

func ModelsDir() string { return filepath.Join(DataDir(), "models") }

// ModelPath resolves a model name to a file path.
func ModelPath(model string) string {
	if filepath.IsAbs(model) {
		return model
	}
	return filepath.Join(ModelsDir(), "ggml-"+model+".bin")
}

// Load reads the config file, writing a default one if none exists.
func Load() (Config, string, error) {
	cfg := Default()
	path := filepath.Join(Dir(), "config.yaml")
	err := decodeFile(path, &cfg)
	if errors.Is(err, fs.ErrNotExist) {
		err = writeDefault(path, cfg)
	}
	if cfg.Threads == 0 {
		cfg.Threads = uint(min(8, runtime.NumCPU()))
	}
	return cfg, path, err
}

func decodeFile(path string, cfg *Config) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec := yaml.NewDecoder(f)
	dec.KnownFields(true) // report typos instead of silently ignoring them
	if err := dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	return nil
}

// comments documents each option in the generated default config file.
var comments = map[string]string{
	"model":              "Model name in ~/.local/share/whisp/models (e.g. large-v3-turbo-q5_0, small.en), or an absolute path.",
	"language":           "Spoken language (en, hu, ...), or \"auto\" to detect it per clip.",
	"key":                "Hold this key to dictate (any evdev key name).",
	"meeting_key":        "Press to start/stop recording a meeting (microphone + system audio).",
	"meetings_dir":       "Each meeting is saved here as <start>_to_<end>.txt.",
	"meeting_timestamps": "Time in front of each sentence of a meeting: clock (14:12:37), elapsed (00:09:25) or none.",
	"paste":              "Key combo that pastes the text: shift+insert, ctrl+v or ctrl+shift+v.",
	"restore_clipboard":  "Put the previous clipboard content back after pasting.",
	"min_duration":       "Dictation clips shorter than this are ignored (accidental taps).",
	"threads":            "CPU threads for whisper (0 = auto).",
	"prompt":             "Optional vocabulary hint, e.g. \"Kubernetes, kubectl, Go\".",
	"trailing_space":     "Add a space after dictated text so consecutive dictations don't run together.",
	"mic":                "Capture device name substring; empty = system default.",
}

func writeDefault(path string, cfg Config) error {
	var doc yaml.Node
	if err := doc.Encode(cfg); err != nil {
		return err
	}
	doc.HeadComment = "whisp configuration"
	for i := 0; i+1 < len(doc.Content); i += 2 {
		key := doc.Content[i]
		key.HeadComment = comments[key.Value]
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { // transcripts live here too
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := yaml.NewEncoder(f)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return err
	}
	return enc.Close()
}
