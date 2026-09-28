//go:build vulkan

package stt

/*
#cgo LDFLAGS: -lvulkan
#include <stdlib.h>
#include <string.h>
#include <vulkan/vulkan.h>

// list_devices fills names/discrete for up to max physical devices, in the
// same enumeration order ggml's Vulkan backend uses. Returns the device
// count, or -1 if Vulkan is unavailable.
static int list_devices(char names[][VK_MAX_PHYSICAL_DEVICE_NAME_SIZE], int *discrete, int max) {
	VkApplicationInfo app = {.sType = VK_STRUCTURE_TYPE_APPLICATION_INFO, .pApplicationName = "whisp", .apiVersion = VK_API_VERSION_1_2};
	VkInstanceCreateInfo info = {.sType = VK_STRUCTURE_TYPE_INSTANCE_CREATE_INFO, .pApplicationInfo = &app};
	VkInstance instance;
	if (vkCreateInstance(&info, NULL, &instance) != VK_SUCCESS) {
		return -1;
	}
	uint32_t n = 0;
	vkEnumeratePhysicalDevices(instance, &n, NULL);
	VkPhysicalDevice *devs = calloc(n, sizeof(VkPhysicalDevice));
	vkEnumeratePhysicalDevices(instance, &n, devs);
	int count = (int)n < max ? (int)n : max;
	for (int i = 0; i < count; i++) {
		VkPhysicalDeviceProperties props;
		vkGetPhysicalDeviceProperties(devs[i], &props);
		strncpy(names[i], props.deviceName, VK_MAX_PHYSICAL_DEVICE_NAME_SIZE - 1);
		names[i][VK_MAX_PHYSICAL_DEVICE_NAME_SIZE - 1] = 0;
		discrete[i] = props.deviceType == VK_PHYSICAL_DEVICE_TYPE_DISCRETE_GPU;
	}
	free(devs);
	vkDestroyInstance(instance, NULL);
	return count;
}
*/
import "C"

import (
	"log"
	"os"
	"strconv"
	"sync"
)

const maxDevices = 16

var selectOnce sync.Once

// selectGPU points ggml's Vulkan backend at the discrete GPU via
// GGML_VK_VISIBLE_DEVICES; it must run before the model is loaded. An
// existing GGML_VK_VISIBLE_DEVICES setting is respected.
func selectGPU() { selectOnce.Do(selectVulkanDevice) }

func selectVulkanDevice() {
	if v := os.Getenv("GGML_VK_VISIBLE_DEVICES"); v != "" {
		log.Printf("vulkan: GGML_VK_VISIBLE_DEVICES=%s set by the user", v)
		return
	}
	var names [maxDevices][C.VK_MAX_PHYSICAL_DEVICE_NAME_SIZE]C.char
	var discrete [maxDevices]C.int
	n := int(C.list_devices(&names[0], &discrete[0], maxDevices))
	if n < 0 {
		log.Print("vulkan: cannot list devices")
		return
	}
	devices := make([]Device, n)
	for i := range devices {
		devices[i] = Device{Name: C.GoString(&names[i][0]), Discrete: discrete[i] != 0}
		log.Printf("vulkan: device %d: %s (discrete: %v)", i, devices[i].Name, devices[i].Discrete)
	}
	if i := pickGPU(devices); i >= 0 {
		os.Setenv("GGML_VK_VISIBLE_DEVICES", strconv.Itoa(i))
		log.Printf("vulkan: using device %d: %s", i, devices[i].Name)
	}
}
