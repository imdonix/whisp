//go:build !vulkan

package stt

// selectGPU is only needed for Vulkan; CUDA and CPU builds use whisper's default.
func selectGPU() {}
