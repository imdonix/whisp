// Package inject inserts text into the focused window: the text is placed on
// the session's clipboard (and primary selection), then a paste shortcut is sent
// through a uinput virtual keyboard. This is independent of the keyboard layout
// and handles any Unicode text.
package inject

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/bendahl/uinput"
)

type Injector struct {
	kb      uinput.Keyboard
	combo   []int
	restore bool
	clip    clipboard
}

func New(paste string, restore bool) (*Injector, error) {
	combo, err := parseCombo(paste)
	if err != nil {
		return nil, err
	}
	clip, err := newClipboard()
	if err != nil {
		return nil, err
	}
	kb, err := uinput.CreateKeyboard("/dev/uinput", []byte("whisp virtual keyboard"))
	if err != nil {
		return nil, fmt.Errorf("create virtual keyboard: %w (run 'make setup-permissions')", err)
	}
	// Give the compositor a moment to register the new device.
	time.Sleep(200 * time.Millisecond)
	return &Injector{kb: kb, combo: combo, restore: restore, clip: clip}, nil
}

func (in *Injector) Close() error { return in.kb.Close() }

// clipboard moves text in and out of the session's clipboard: wl-copy on
// Wayland, xclip on X11 (wl-copy has no Wayland display to talk to there).
type clipboard struct {
	bin         string
	clipArgs    []string
	primaryArgs []string
	readBin     string
	readArgs    []string
}

func newClipboard() (clipboard, error) {
	var clip clipboard
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		clip = clipboard{
			bin:         "wl-copy",
			primaryArgs: []string{"--primary"},
			readBin:     "wl-paste",
			readArgs:    []string{"--no-newline"},
		}
	} else {
		clip = clipboard{
			bin:         "xclip",
			clipArgs:    []string{"-selection", "clipboard"},
			primaryArgs: []string{"-selection", "primary"},
			readBin:     "xclip",
			readArgs:    []string{"-selection", "clipboard", "-o"},
		}
	}
	for _, bin := range []string{clip.bin, clip.readBin} {
		if _, err := exec.LookPath(bin); err != nil {
			pkg := "xclip"
			if clip.bin == "wl-copy" {
				pkg = "wl-clipboard"
			}
			return clipboard{}, fmt.Errorf("%s not found (sudo apt install %s)", bin, pkg)
		}
	}
	return clip, nil
}

func (in *Injector) readClipboard() ([]byte, error) {
	// Reading a selection whose owner is gone or busy blocks, so cap the wait.
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, in.clip.readBin, in.clip.readArgs...).Output()
}

func (in *Injector) Type(text string) error {
	var prev []byte
	var hadPrev bool
	if in.restore {
		if out, err := in.readClipboard(); err == nil {
			prev, hadPrev = out, true
		}
	}

	if err := in.copyTo(text, false); err != nil {
		return err
	}
	_ = in.copyTo(text, true) // primary selection; some terminals paste it on Shift+Insert

	time.Sleep(50 * time.Millisecond)
	if err := in.sendCombo(); err != nil {
		return err
	}

	if hadPrev {
		go func() {
			time.Sleep(400 * time.Millisecond)
			cmd := exec.Command(in.clip.bin, in.clip.clipArgs...)
			cmd.Stdin = bytes.NewReader(prev)
			_ = cmd.Run()
		}()
	}
	return nil
}

func (in *Injector) copyTo(text string, primary bool) error {
	args := in.clip.clipArgs
	if primary {
		args = in.clip.primaryArgs
	}
	// The copy tool forks a child that keeps serving the selection; it must not
	// inherit a pipe from us, or Run would wait for that child to exit.
	// Stdout/Stderr stay nil (/dev/null) and WaitDelay guards against any other
	// inherited handle.
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, in.clip.bin, args...)
	cmd.Stdin = strings.NewReader(text)
	cmd.WaitDelay = time.Second
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w", in.clip.bin, err)
	}
	return nil
}

func (in *Injector) sendCombo() error {
	for _, k := range in.combo {
		if err := in.kb.KeyDown(k); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	for i := len(in.combo) - 1; i >= 0; i-- {
		if err := in.kb.KeyUp(in.combo[i]); err != nil {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
	return nil
}

var keyNames = map[string]int{
	"shift":  uinput.KeyLeftshift,
	"ctrl":   uinput.KeyLeftctrl,
	"alt":    uinput.KeyLeftalt,
	"insert": uinput.KeyInsert,
	"v":      uinput.KeyV,
}

func parseCombo(s string) ([]int, error) {
	var combo []int
	for _, part := range strings.Split(strings.ToLower(s), "+") {
		k, ok := keyNames[strings.TrimSpace(part)]
		if !ok {
			return nil, fmt.Errorf("unsupported key %q in paste combo %q", part, s)
		}
		combo = append(combo, k)
	}
	return combo, nil
}
