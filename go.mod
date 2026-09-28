module github.com/tamasmagyar/whisp

go 1.26.3

replace github.com/ggerganov/whisper.cpp/bindings/go => ./third_party/whisper.cpp/bindings/go

require (
	fyne.io/systray v1.12.2
	github.com/bendahl/uinput v1.7.0
	github.com/gen2brain/malgo v0.11.26
	github.com/ggerganov/whisper.cpp/bindings/go v0.0.0
	github.com/go-audio/wav v1.1.0
	github.com/holoplot/go-evdev v0.0.0-20260909133534-d873ed210bda
	gopkg.in/yaml.v3 v3.0.1
)

require (
	github.com/go-audio/audio v1.0.0 // indirect
	github.com/go-audio/riff v1.0.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	golang.org/x/sys v0.15.0 // indirect
)
