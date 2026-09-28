# Installing whisp

whisp is built for, and on, the machine it runs on. The binary is **not
portable**: it is compiled for the build machine's exact CPU (`-march=native`)
and GPU backend, and links the CUDA or Vulkan libraries dynamically. Build it on
each machine instead of copying it. Machines similar to the reference one work:

| | Required | Reference machine |
|---|---|---|
| OS | Ubuntu 24.04 (glibc ≥ 2.38), GNOME on **Wayland** (X11 works too) | Ubuntu 24.04, GNOME 46 |
| Tray | `ubuntu-appindicators` extension (enabled by default on Ubuntu) | ✓ |
| GPU | NVIDIA RTX 30 series or newer (CUDA), **or** any Vulkan GPU, e.g. AMD Radeon RX 6000/7000 (Vulkan); ≥ 2 GB VRAM | RTX 2050 4 GB (laptop, Intel iGPU drives the display) |
| Driver | NVIDIA: driver loaded, `nvidia-smi` works. AMD/Intel: nothing to install (Ubuntu's kernel and Mesa drivers) | 580.173.02 |
| CPU | x86-64; the build uses whatever the build CPU supports | Intel Core Ultra (Meteor Lake) |
| Disk | ~600 MB for the model, ~3 GB for the CUDA toolkit (NVIDIA only) | |

## GPU backend (chosen at build time)

`make` picks the backend for the machine it runs on:

1. **CUDA** – if there is an NVIDIA GPU (`lspci`) and the CUDA toolkit (`nvcc`) is installed;
2. otherwise **Vulkan** – if the Vulkan headers and `glslc` are installed (AMD, Intel, also NVIDIA);
3. otherwise **CPU** – works, but a clip takes ~10–20 s instead of < 1 s.

Force one with `make BACKEND=cuda|vulkan|cpu` (each backend has its own build
directory, so switching doesn't rebuild the others). For an RTX 20 / GTX 16 series
GPU with CUDA, add `CUDA_ARCH=75`.

With Vulkan, whisp uses the first **discrete** GPU, so a desktop's integrated GPU
(e.g. a Ryzen iGPU next to a Radeon card) isn't picked by accident; the log lists
the devices it saw. To choose a device yourself, start whisp with
`GGML_VK_VISIBLE_DEVICES=<index>` (index as listed in the log).

## Setup guide

Run these once per machine, in order.

### 1. Get the code

```sh
git clone --recurse-submodules <repo-url> whisp
cd whisp
# already cloned without submodules? -> git submodule update --init
```

### 2. Get the GPU driver working

**AMD or Intel GPU:** nothing to do — Ubuntu's kernel (`amdgpu`, `i915`/`xe`)
and Mesa's Vulkan drivers already support it. After step 3 you can check with
`vulkaninfo --summary` (your GPU should be listed). Skip to step 3.

**NVIDIA GPU:** check first — if this lists your GPU, skip to step 3:

```sh
nvidia-smi
```

How to fix it depends on how Ubuntu was installed. Check the kernel command line:

```sh
grep -o snapd_recovery_mode /proc/cmdline
```

**No output → normal install (kernel from apt).** Use Ubuntu's prebuilt,
Canonical-signed driver so it works with Secure Boot:

```sh
sudo ubuntu-drivers install
sudo reboot
nvidia-smi
```

If you use a DKMS driver instead (`nvidia-dkms-*`), the kernel headers must be
installed (`sudo apt install linux-headers-$(uname -r)`), and with Secure Boot
enabled the module must be signed with an enrolled MOK key, or the kernel refuses
to load it.

**`snapd_recovery_mode` → TPM-backed full-disk encryption (kernel from the
`pc-kernel` snap).** apt/DKMS drivers can't be used here: apt can't install
kernel modules for the snap kernel, and unsigned DKMS modules are rejected by
Secure Boot. Do **not** enroll MOK keys or disable Secure Boot — that changes what
the TPM measures and the disk will ask for the recovery key at boot. Instead use
the signed NVIDIA kernel module that ships as a component of the kernel snap:

```sh
# 1. remove any DKMS driver (it can never load here and would clash)
dkms status                                   # note any nvidia/<version> entries
sudo dkms remove nvidia/<version> --all       # for each of them
sudo apt purge 'nvidia-dkms-*' 'nvidia-driver-*'

# 2. install the signed kernel module component
snap components pc-kernel | grep nvidia       # lists available versions
sudo snap install pc-kernel+nvidia-580-erd-ko
sudo reboot

# 3. install the user-space libraries matching the module version exactly
cat /proc/driver/nvidia/version               # e.g. "Kernel Module 580.173.02"
apt-cache policy nvidia-utils-580-server      # pick the package whose version matches
sudo apt install --no-install-recommends nvidia-utils-580-server libnvidia-compute-580-server
nvidia-smi
```

Notes for this path:

- The user-space version must match the kernel module **exactly**, otherwise
  `nvidia-smi` reports "Driver/library version mismatch". On the reference
  machine the `-server` packages (580.173.02) matched the component; the plain
  `nvidia-utils-580` package (580.178.04) did not.
- Only install `nvidia-utils-*` and `libnvidia-compute-*`, not `nvidia-driver-*`:
  the latter pulls in DKMS again. The Intel iGPU keeps driving the display.
- `snap install` may report an error for the `nvidia-580-erd-user` component (or
  for the whole change) even though the kernel module ends up installed. Trust
  `nvidia-smi` and `lsmod | grep nvidia`, not `snap changes`.
- After a `pc-kernel` snap update, check `nvidia-smi` again; if the module is
  gone, re-run `sudo snap install pc-kernel+nvidia-580-erd-ko`.

### 3. Install build dependencies

```sh
make deps
```

This installs the common tools (build-essential, cmake, git, pkg-config,
wl-clipboard, xclip) plus the GPU toolchain for this machine:

- NVIDIA GPU found: `nvidia-cuda-toolkit` and `g++-12` (Ubuntu's CUDA 12.0 doesn't
  support the default GCC 13, so the Makefile uses `g++-12` as the CUDA host compiler);
- otherwise: `libvulkan-dev glslc spirv-headers mesa-vulkan-drivers vulkan-tools`.

### 4. Grant input permissions

whisp needs to read the keyboard (`/dev/input`, for the F9 hold/release) and to
create a virtual keyboard (`/dev/uinput`, to press Shift+Insert):

```sh
make setup-permissions    # udev rule for /dev/uinput, loads uinput, adds you to the input group
```

**Log out and back in** (or reboot) so the new group applies; check with `groups`
(should list `input`) and `ls -l /dev/uinput` (should be group `input`, mode `crw-rw----`).

Note: members of the `input` group can read all keyboard input; that is inherent
to push-to-talk on Wayland.

### 5. Build, download the model, install

```sh
make              # builds whisper.cpp for this machine's GPU (first build takes a few minutes) and bin/whisp
make model        # ~574 MB -> ~/.local/share/whisp/models/ggml-large-v3-turbo-q5_0.bin
make install      # -> ~/.local/bin/whisp
```

The output of `make` ends with `built bin/whisp (backend: cuda)` (NVIDIA) or
`(backend: vulkan)` (AMD/Intel). If it says `cpu`, the GPU toolchain wasn't found —
check step 3.

### 6. Test

```sh
whisp listen 4    # speak for 4 seconds; prints the transcript and how long it took
```

On the GPU the transcription takes well under a second. Then start it for real:

```sh
whisp
grep -E "using .* backend|no GPU" ~/.wisp/whisp.log | tail -1   # expect "using CUDA0 backend" or "using Vulkan0 backend"
```

Hold F9 in a text field or terminal, speak, release.

### 7. Start on login (optional)

```sh
cp packaging/whisp.desktop ~/.config/autostart/
```

## Troubleshooting

| Symptom | Cause / fix |
|---|---|
| Tray shows an error: `create virtual keyboard ... could not open device file` | No access to `/dev/uinput`: run step 4 and log out/in. |
| Error `no readable keyboard in /dev/input` | Not in the `input` group yet (log out/in after step 4). |
| Each clip takes 10–20 s | Running on CPU. The log shows `no GPU found` / `no CUDA-capable device` → the driver isn't loaded (step 2), or `make` built the `cpu` backend (step 3). |
| Vulkan build uses the wrong GPU | The log lists the Vulkan devices (`vulkan: device N: …`); start whisp with `GGML_VK_VISIBLE_DEVICES=N` to choose one. |
| `make` fails with `SPIRV-Headers` or `glslc` not found (Vulkan) | `sudo apt install spirv-headers glslc libvulkan-dev` (or `make deps`). |
| `whisp: error while loading shared libraries: libcuda.so.1` | NVIDIA user-space libraries missing (step 2). |
| `nvidia-smi`: "couldn't communicate with the NVIDIA driver" | Kernel module not loaded: missing headers for DKMS, unsigned module under Secure Boot, or (TPM FDE) the snap component isn't installed. See step 2. |
| `nvidia-smi`: "Driver/library version mismatch" | User-space packages don't match the kernel module version; compare `cat /proc/driver/nvidia/version` with `dpkg -l \| grep nvidia-utils`. |
| Text is transcribed (see log) but nothing appears | The app doesn't paste on Shift+Insert; set `paste = "ctrl+v"` (or `"ctrl+shift+v"` for terminals) in the config. |
| Error `wl-copy: exit status 1` or `xclip: ...` | Wrong clipboard tool for the session: whisp uses `wl-copy` on Wayland and `xclip` on X11. If you run it from a terminal without `WAYLAND_DISPLAY`/`DISPLAY`, or one of them isn't installed, run `make deps`. |
| Meeting transcript only has the start/stop lines | No speech was detected: check the mic isn't muted and that the call audio plays on the **default** output device (Settings → Sound) when pressing F10. |
| Tray error "cannot record system audio" | PipeWire's PulseAudio layer isn't running: `systemctl --user status pipewire-pulse`. |
| Very short taps produce "Thank you." or similar | Whisper hallucinates on near-silent clips; hold the key a bit longer, or raise `min_duration`. |
