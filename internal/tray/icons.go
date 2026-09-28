package tray

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

const iconSize = 64

// shape is a signed distance function in icon pixels: negative inside.
type shape func(x, y float64) float64

// render draws a shape in the given color; the optional badge is drawn as a
// small dot in the bottom-right corner.
func render(sh shape, fg color.RGBA, badge *color.RGBA) []byte {
	img := image.NewRGBA(image.Rect(0, 0, iconSize, iconSize))
	set := func(x, y int, c color.RGBA, a float64) {
		if a <= 0 {
			return
		}
		if a > 1 {
			a = 1
		}
		old := img.RGBAAt(x, y)
		blend := func(o, c uint8) uint8 { return uint8(float64(o)*(1-a) + float64(c)*a) }
		img.SetRGBA(x, y, color.RGBA{blend(old.R, c.R), blend(old.G, c.G), blend(old.B, c.B), uint8(math.Max(float64(old.A), a*255))})
	}
	for y := 0; y < iconSize; y++ {
		for x := 0; x < iconSize; x++ {
			px, py := float64(x)+0.5, float64(y)+0.5
			set(x, y, fg, 0.5-sh(px, py))
			if badge != nil {
				bd := math.Hypot(px-52, py-52) - 10
				if bd < 2 { // dark outline so the dot is visible on any panel
					set(x, y, color.RGBA{0, 0, 0, 255}, 0.5-(bd-2))
				}
				set(x, y, *badge, 0.5-bd)
			}
		}
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

// bubble is a speech bubble with three cut-out dots ("text is coming").
// A microphone is deliberately avoided: GNOME already shows its own
// microphone indicator while any app records.
func bubble(x, y float64) float64 {
	body := roundedRect(x, y, 32, 27, 24, 17, 11)
	tail := triangle(x, y, 14, 38, 30, 38, 12, 57)
	return math.Max(math.Min(body, tail), -dots(x, y, 32, 27, 12, 4.5))
}

// conversation is two overlapping speech bubbles, the front one with dots,
// shown while a meeting is being recorded.
func conversation(x, y float64) float64 {
	back := math.Min(roundedRect(x, y, 39, 20, 21, 14, 9), triangle(x, y, 44, 30, 55, 30, 59, 42))
	front := math.Min(roundedRect(x, y, 25, 38, 21, 14, 9), triangle(x, y, 9, 48, 21, 48, 6, 60))
	back = math.Max(back, -(front - 3.5)) // gap around the front bubble
	return math.Min(math.Max(front, -dots(x, y, 25, 38, 10, 3.5)), back)
}

// dots is the distance to three dots centred at (cx,cy), spaced dx apart.
func dots(x, y, cx, cy, dx, r float64) float64 {
	return math.Min(math.Hypot(x-cx+dx, y-cy), math.Min(math.Hypot(x-cx, y-cy), math.Hypot(x-cx-dx, y-cy))) - r
}

// roundedRect is centred at (cx,cy) with half sizes hw,hh and corner radius r.
func roundedRect(x, y, cx, cy, hw, hh, r float64) float64 {
	qx, qy := math.Abs(x-cx)-(hw-r), math.Abs(y-cy)-(hh-r)
	return math.Hypot(math.Max(qx, 0), math.Max(qy, 0)) + math.Min(math.Max(qx, qy), 0) - r
}

// triangle returns an approximate signed distance from p to the triangle abc
// (negative inside), good enough for anti-aliased edges.
func triangle(px, py, ax, ay, bx, by, cx, cy float64) float64 {
	// Edge distances are negative on the inner side for a clockwise winding in
	// image coordinates; flip for the other winding.
	sign := 1.0
	if (bx-ax)*(cy-ay)-(by-ay)*(cx-ax) < 0 {
		sign = -1
	}
	edge := func(x0, y0, x1, y1 float64) float64 {
		ex, ey := x1-x0, y1-y0
		return sign * ((px-x0)*ey - (py-y0)*ex) / math.Hypot(ex, ey)
	}
	return math.Max(edge(ax, ay, bx, by), math.Max(edge(bx, by, cx, cy), edge(cx, cy, ax, ay)))
}

var (
	white  = color.RGBA{0xf2, 0xf2, 0xf2, 0xff}
	grey   = color.RGBA{0x8a, 0x8a, 0x8a, 0xff}
	red    = color.RGBA{0xe0, 0x1b, 0x24, 0xff}
	orange = color.RGBA{0xff, 0xa3, 0x48, 0xff}
	yellow = color.RGBA{0xf6, 0xd3, 0x2d, 0xff}
)

var icons = map[State][]byte{
	Loading:      render(bubble, grey, nil),
	Idle:         render(bubble, white, nil),
	Recording:    render(bubble, red, nil),
	Transcribing: render(bubble, white, &orange),
	Error:        render(bubble, grey, &yellow),
	Meeting:      render(conversation, red, nil),
}
