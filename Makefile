# whisp build
#   make                  build whisper.cpp + bin/whisp. The GPU backend is chosen here, at
#                         build time: CUDA if an NVIDIA GPU and nvcc are present, otherwise
#                         Vulkan (AMD/Intel GPUs) if its build tools are installed, otherwise CPU
#   make BACKEND=vulkan   force a backend: cuda | vulkan | cpu
#   make deps             install the build dependencies for this machine's GPU (sudo)
#   make model            download the default model
#   make install          install to ~/.local/bin

comma := ,
WHISPER_DIR := $(abspath third_party/whisper.cpp)
CMAKE       ?= cmake
CMAKE_EXTRA ?=
GLSLC       ?= glslc
PREFIX      ?= $(HOME)/.local
MODEL       ?= large-v3-turbo-q5_0
CUDA_ARCH   ?= 86
CUDA_PATH   ?= $(shell dirname $$(dirname $$(command -v nvcc 2>/dev/null || echo /usr/local/cuda/bin/nvcc)))

HAVE_NVIDIA_GPU := $(shell lspci 2>/dev/null | grep -qi nvidia && echo 1)
HAVE_NVCC       := $(shell command -v nvcc >/dev/null 2>&1 && echo 1)
HAVE_VULKAN     := $(shell pkg-config --exists vulkan 2>/dev/null && command -v $(GLSLC) >/dev/null 2>&1 && echo 1)

ifeq ($(origin BACKEND),undefined)
BACKEND := $(if $(and $(HAVE_NVIDIA_GPU),$(HAVE_NVCC)),cuda,$(if $(HAVE_VULKAN),vulkan,cpu))
endif
ifeq ($(filter $(BACKEND),cuda vulkan cpu),)
$(error BACKEND must be cuda, vulkan or cpu, not "$(BACKEND)")
endif

BUILD_DIR := $(WHISPER_DIR)/build_whisp_$(BACKEND)
LIBS      := -L$(BUILD_DIR)/src -L$(BUILD_DIR)/ggml/src
GGML_LIBS := -lwhisper -lggml -lggml-base -lggml-cpu

ifeq ($(BACKEND),cuda)
CMAKE_FLAGS := -DGGML_CUDA=ON -DCMAKE_CUDA_ARCHITECTURES=$(CUDA_ARCH)
# Ubuntu's CUDA 12.0 nvcc does not support GCC 13; use g++-12 as host compiler when present.
ifneq ($(shell command -v g++-12 2>/dev/null),)
CMAKE_FLAGS += -DCMAKE_CUDA_HOST_COMPILER=g++-12
endif
CUDA_LIBDIRS := $(wildcard $(CUDA_PATH)/lib64 $(CUDA_PATH)/targets/x86_64-linux/lib)
LIBS += -L$(BUILD_DIR)/ggml/src/ggml-cuda $(addprefix -L,$(CUDA_LIBDIRS)) $(addprefix -Wl$(comma)-rpath$(comma),$(CUDA_LIBDIRS))
GGML_LIBS += -lggml-cuda
EXTLIBS := -lcudart -lcublas -lcublasLt -lcuda
else ifeq ($(BACKEND),vulkan)
CMAKE_FLAGS := -DGGML_VULKAN=ON -DVulkan_GLSLC_EXECUTABLE=$(shell command -v $(GLSLC))
LIBS += -L$(BUILD_DIR)/ggml/src/ggml-vulkan
GGML_LIBS += -lggml-vulkan
EXTLIBS := -lvulkan
endif

export C_INCLUDE_PATH := $(WHISPER_DIR)/include:$(WHISPER_DIR)/ggml/include
export CGO_LDFLAGS := $(LIBS) -Wl,--start-group $(GGML_LIBS) -Wl,--end-group $(EXTLIBS) -fopenmp -lstdc++ -lm
# Go code specific to a backend (e.g. Vulkan GPU selection) is behind a build tag.
export GOFLAGS := -tags=$(BACKEND)

.PHONY: all whisper build test model install uninstall clean deps setup-permissions

all: build

$(BUILD_DIR)/.built:
	@echo "whisp: building whisper.cpp with the $(BACKEND) backend"
	$(CMAKE) -S $(WHISPER_DIR) -B $(BUILD_DIR) -DCMAKE_BUILD_TYPE=Release \
		-DBUILD_SHARED_LIBS=OFF -DWHISPER_BUILD_TESTS=OFF -DWHISPER_BUILD_EXAMPLES=OFF \
		-DGGML_NATIVE=ON $(CMAKE_FLAGS) $(CMAKE_EXTRA)
	$(CMAKE) --build $(BUILD_DIR) --target whisper -j$$(nproc)
	touch $@

whisper: $(BUILD_DIR)/.built

build: whisper
	go build -trimpath -o bin/whisp ./cmd/whisp
	@echo "built bin/whisp (backend: $(BACKEND))"

test: whisper
	go test $(TESTFLAGS) ./...

model: build
	./bin/whisp download-model $(MODEL)

install: build
	install -Dm755 bin/whisp $(PREFIX)/bin/whisp
	install -Dm644 packaging/whisp.desktop $(PREFIX)/share/applications/whisp.desktop
	@echo "installed $(PREFIX)/bin/whisp"
	@echo "autostart on login: cp packaging/whisp.desktop ~/.config/autostart/"

uninstall:
	rm -f $(PREFIX)/bin/whisp $(PREFIX)/share/applications/whisp.desktop $(HOME)/.config/autostart/whisp.desktop

# One-time system setup (needs sudo): build tools, clipboard tools, and the GPU
# toolchain for this machine: CUDA for an NVIDIA GPU, otherwise Vulkan.
COMMON_DEPS := build-essential cmake git pkg-config wl-clipboard xclip
deps:
ifeq ($(HAVE_NVIDIA_GPU),1)
	sudo apt install -y $(COMMON_DEPS) g++-12 nvidia-cuda-toolkit
else
	sudo apt install -y $(COMMON_DEPS) libvulkan-dev glslc spirv-headers mesa-vulkan-drivers vulkan-tools
endif

# One-time system setup (needs sudo): uinput permissions and input group.
setup-permissions:
	sudo install -m644 packaging/99-whisp-uinput.rules /etc/udev/rules.d/99-whisp-uinput.rules
	echo uinput | sudo tee /etc/modules-load.d/uinput.conf >/dev/null
	sudo modprobe uinput
	sudo udevadm control --reload-rules && sudo udevadm trigger /dev/uinput
	sudo usermod -aG input $(USER)
	@echo "log out and back in for the input group to take effect"

clean:
	rm -rf bin $(WHISPER_DIR)/build_whisp_*
