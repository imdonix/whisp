// Package tray shows whisp's status icon in the GNOME top bar (via the
// StatusNotifierItem / AppIndicator protocol).
package tray

import (
	"fyne.io/systray"
)

type State int

const (
	Loading State = iota
	Idle
	Recording
	Transcribing
	Error
	Meeting
)

var labels = map[State]string{
	Loading:      "Loading model…",
	Idle:         "Ready — hold the hotkey to dictate",
	Recording:    "Listening…",
	Transcribing: "Transcribing…",
	Error:        "Error",
	Meeting:      "Recording meeting…",
}

type Tray struct {
	status *systray.MenuItem
}

// Run blocks, running the tray event loop. onReady is called once the tray is up;
// onQuit is called when the user picks Quit.
func Run(title string, onReady func(*Tray), onQuit func()) {
	systray.Run(func() {
		systray.SetIcon(icons[Loading])
		systray.SetTitle("")
		systray.SetTooltip("whisp")
		t := &Tray{}
		t.status = systray.AddMenuItem(labels[Loading], "")
		t.status.Disable()
		info := systray.AddMenuItem(title, "")
		info.Disable()
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit whisp", "Stop voice typing")
		go func() {
			<-quit.ClickedCh
			systray.Quit()
		}()
		onReady(t)
	}, onQuit)
}

// Set updates icon and status line. detail, if given, replaces the default label.
func (t *Tray) Set(s State, detail string) {
	label := labels[s]
	if detail != "" {
		label = detail
	}
	systray.SetIcon(icons[s])
	systray.SetTooltip("whisp: " + label)
	t.status.SetTitle(label)
}

func Quit() { systray.Quit() }
