# whisp

Hold **F9**, speak, release — the text lands in whatever window you were typing
in. Local speech recognition, GPU-accelerated, no cloud, no API key.

```sh
whisp            # tray app
whisp listen 4   # record 4 s, print the transcript and how long it took
```

## Why it works where dictation usually doesn't

- **Keyboard layout independent.** Most voice typing injects text as keycodes,
  which breaks on a `hu` layout. whisp puts the text on the clipboard and sends
  Shift+Insert through a virtual keyboard, so any Unicode text arrives intact —
  in Chrome, terminals, editors.
- **Local and fast.** [whisper.cpp](https://github.com/ggml-org/whisper.cpp)
  `large-v3-turbo` on your GPU: a clip transcribes in **~300 ms**, versus
  ~10–20 s on the CPU backend. Nothing leaves the machine.
- **Language detected per clip**, so English and Hungarian both work.

## Tray

| Icon | Meaning |
|---|---|
| grey speech bubble | loading the model |
| white speech bubble | ready |
| red speech bubble | listening (key held) |
| white bubble + orange dot | transcribing |
| grey bubble + yellow dot | error (see the menu / log) |
| two red speech bubbles | recording a meeting (F10) |

## Usage

```sh
whisp                     # start in the background (tray icon appears)
whisp --foreground        # run attached, log to the terminal
whisp --foreground --dry-run   # print transcripts instead of typing them
whisp listen 4            # record 4 s and print the transcript (mic/GPU test)
whisp meeting             # start/stop meeting recording (same as F10)
whisp status
whisp stop                # or "Quit whisp" in the tray menu
whisp download-model small.en   # other models: base.en, small.en, medium.en, large-v3-turbo, ...
```

## Configuration

`~/.wisp/config.yaml` is created on first run, with every option explained in a
comment. Restart whisp (`whisp stop && whisp`) after editing it. Options you
remove fall back to their defaults; unknown options (typos) are reported as an
error.

```yaml
model: large-v3-turbo-q5_0   # name in ~/.local/share/whisp/models, or absolute path
language: auto               # detect per clip, or fix it: en, hu, ...
key: KEY_F9                  # hold to dictate; any evdev key name
meeting_key: KEY_F10         # toggles meeting recording
meetings_dir: ~/.wisp        # one <start>_to_<end>.txt file per meeting
meeting_timestamps: clock    # time before each sentence: clock, elapsed or none
paste: shift+insert          # or ctrl+v, ctrl+shift+v
restore_clipboard: true
min_duration: 300ms          # shorter taps are ignored
threads: 0                   # 0 = auto
prompt: ""                   # optional vocabulary hint, e.g. "Kubernetes, kubectl, Go"
trailing_space: true
mic: ""                      # capture device name substring; empty = default
```

Log: `~/.wisp/whisp.log` (readable only by you: it contains dictated text).
`~/.wisp` is mode `0700`, transcripts `0600`.

Recording other people may require their consent — check what applies to you.

## Install

See **[INSTALL.md](INSTALL.md)** for the requirements, the GPU backend
selection (CUDA / Vulkan / CPU) and a step-by-step guide.

Built for and on the machine it runs on: it links the GPU libraries dynamically
and is compiled with `-march=native`.

```sh
make deps    # build tools + the GPU toolchain for this machine
make         # whisper.cpp + bin/whisp
make model   # download the default model
make install # -> ~/.local/bin/whisp
```

## Development

```sh
make test            # includes a transcription of whisper.cpp's jfk.wav sample
```
