# whisp

Go tray app: hold F9 → record → whisper.cpp (CGo, static libs from third_party/whisper.cpp) → paste via the clipboard (wl-copy on Wayland, xclip on X11) + uinput Shift+Insert.
F10 toggles meeting recording: mic + `@DEFAULT_MONITOR@` mixed, chunked at pauses (internal/meeting), one file per meeting in ~/.wisp/ (also holds config.yaml and whisp.log).

- Always build/test through `make` (it sets C_INCLUDE_PATH / CGO_LDFLAGS for whisper.cpp); plain `go build` fails with `whisper.h not found`.
- The GPU backend is chosen at build time: CUDA (NVIDIA GPU + nvcc) → Vulkan (headers + glslc) → CPU; force with `make BACKEND=cuda|vulkan|cpu`. Build dirs: `third_party/whisper.cpp/build_whisp_<backend>`. Backend-specific Go code uses build tags (`GOFLAGS=-tags=<backend>`, set by the Makefile), e.g. `internal/stt/gpu_vulkan.go` picks the discrete Vulkan GPU.
- In `make ... TESTFLAGS=...`, `$` is expanded by make and `|` by the shell: quote the regex, e.g. `TESTFLAGS='-v -run "A|B"'`.
- Keyboard layout on the target machine is `hu`, so never inject text as keycodes — keep the clipboard+paste approach.
- `make test TESTFLAGS='-v -run TestPipeline'` runs the meeting pipeline end to end on the GPU using whisper.cpp's jfk.wav.
