package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withConfigDir(t *testing.T) string {
	dir := t.TempDir()
	t.Setenv("WISP_HOME", dir)
	return filepath.Join(dir, "config.yaml")
}

func TestDefaultFileRoundTrips(t *testing.T) {
	path := withConfigDir(t)
	first, gotPath, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != path {
		t.Fatalf("path = %s, want %s", gotPath, path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"# whisp configuration", "meeting_key: KEY_F10", "min_duration: 300ms", "# Hold this key to dictate"} {
		if !strings.Contains(string(data), want) {
			t.Errorf("default file lacks %q:\n%s", want, data)
		}
	}
	second, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("reloaded config differs:\n%+v\n%+v", first, second)
	}
}

func TestOverridesAndDefaults(t *testing.T) {
	path := withConfigDir(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("key: KEY_F8\nmin_duration: 1.5s\nrestore_clipboard: false\n"), 0o644)
	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Key != "KEY_F8" || cfg.MinDuration.Duration != 1500*time.Millisecond || cfg.RestoreClipboard {
		t.Errorf("overrides not applied: %+v", cfg)
	}
	if cfg.MeetingKey != "KEY_F10" || cfg.Model != Default().Model {
		t.Errorf("unset options lost their defaults: %+v", cfg)
	}
}

func TestEmptyFileUsesDefaults(t *testing.T) {
	path := withConfigDir(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("# nothing set\n"), 0o644)
	cfg, _, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Key != "KEY_F9" {
		t.Errorf("got %+v", cfg)
	}
}

func TestUnknownKeyIsAnError(t *testing.T) {
	path := withConfigDir(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("meeting_kye: KEY_F11\n"), 0o644)
	if _, _, err := Load(); err == nil || !strings.Contains(err.Error(), "meeting_kye") {
		t.Errorf("err = %v, want an error naming the unknown key", err)
	}
}

func TestBadDurationIsAnError(t *testing.T) {
	path := withConfigDir(t)
	os.MkdirAll(filepath.Dir(path), 0o755)
	os.WriteFile(path, []byte("min_duration: soon\n"), 0o644)
	if _, _, err := Load(); err == nil {
		t.Error("expected an error for an invalid duration")
	}
}
