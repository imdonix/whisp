package stt

import "testing"

func TestPickGPU(t *testing.T) {
	cases := []struct {
		name    string
		devices []Device
		want    int
	}{
		{"integrated listed before discrete", []Device{{"AMD Radeon Graphics (RADV RAPHAEL_MENDOCINO)", false}, {"AMD Radeon RX 7800 XT (RADV NAVI32)", true}}, 1},
		{"only integrated", []Device{{"Intel(R) Arc(tm) Graphics (MTL)", false}}, -1},
		{"discrete first", []Device{{"NVIDIA GeForce RTX 2050", true}, {"Intel(R) Arc(tm) Graphics (MTL)", false}}, 0},
		{"none", nil, -1},
	}
	for _, tc := range cases {
		if got := pickGPU(tc.devices); got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}
