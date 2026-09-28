// Package hotkey watches all keyboards via evdev for press/release of a set of keys.
// Wayland offers no global key-release hooks, so we read /dev/input directly
// (requires membership in the "input" group).
package hotkey

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	evdev "github.com/holoplot/go-evdev"
)

type Event struct {
	Key  evdev.EvCode
	Down bool
}

func ParseKey(name string) (evdev.EvCode, error) {
	name = strings.ToUpper(name)
	if !strings.HasPrefix(name, "KEY_") {
		name = "KEY_" + name
	}
	code, ok := evdev.KEYFromString[name]
	if !ok {
		return 0, fmt.Errorf("unknown key %q", name)
	}
	return code, nil
}

type Watcher struct {
	keys   []evdev.EvCode
	events chan Event

	mu   sync.Mutex
	open map[string]bool
	down map[evdev.EvCode]int // number of devices currently holding each key
}

// Watch starts watching; devices plugged in later are picked up by a periodic rescan.
func Watch(keys ...evdev.EvCode) (*Watcher, error) {
	w := &Watcher{keys: keys, events: make(chan Event, 8), open: map[string]bool{}, down: map[evdev.EvCode]int{}}
	if n := w.scan(); n == 0 {
		return nil, fmt.Errorf("no readable keyboard in /dev/input (is the user in the 'input' group?)")
	}
	go func() {
		for range time.Tick(3 * time.Second) {
			w.scan()
		}
	}()
	return w, nil
}

func (w *Watcher) Events() <-chan Event { return w.events }

func (w *Watcher) scan() int {
	paths, err := evdev.ListDevicePaths()
	if err != nil {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, p := range paths {
		if w.open[p.Path] {
			continue
		}
		dev, err := evdev.Open(p.Path)
		if err != nil {
			continue
		}
		if !hasKey(dev, w.keys[0]) || strings.HasPrefix(p.Name, "whisp") {
			dev.Close()
			continue
		}
		w.open[p.Path] = true
		log.Printf("hotkey: watching %s (%s)", p.Path, p.Name)
		go w.read(p.Path, dev)
	}
	return len(w.open)
}

func hasKey(dev *evdev.InputDevice, key evdev.EvCode) bool {
	for _, c := range dev.CapableEvents(evdev.EV_KEY) {
		if c == key {
			return true
		}
	}
	return false
}

func (w *Watcher) watched(code evdev.EvCode) bool {
	for _, k := range w.keys {
		if k == code {
			return true
		}
	}
	return false
}

// read forwards key events of one device. A key counts as held while any
// device holds it, so two keyboards don't produce duplicate events.
func (w *Watcher) read(path string, dev *evdev.InputDevice) {
	defer dev.Close()
	held := map[evdev.EvCode]bool{}
	for {
		ev, err := dev.ReadOne()
		if err != nil {
			w.mu.Lock()
			delete(w.open, path)
			for key := range held {
				w.release(key)
			}
			w.mu.Unlock()
			return
		}
		if ev.Type != evdev.EV_KEY || !w.watched(ev.Code) || ev.Value == 2 { // 2 = autorepeat
			continue
		}
		w.mu.Lock()
		switch {
		case ev.Value == 1 && !held[ev.Code]:
			held[ev.Code] = true
			w.down[ev.Code]++
			if w.down[ev.Code] == 1 {
				w.events <- Event{Key: ev.Code, Down: true}
			}
		case ev.Value == 0 && held[ev.Code]:
			delete(held, ev.Code)
			w.release(ev.Code)
		}
		w.mu.Unlock()
	}
}

// release must be called with w.mu held.
func (w *Watcher) release(key evdev.EvCode) {
	w.down[key]--
	if w.down[key] == 0 {
		w.events <- Event{Key: key, Down: false}
	}
}
