//go:build !android && !ios && !freebsd && !js

package main

import (
	"bytes"
	"flag"
	"hash/fnv"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"sync"
)

// badgePalette holds the dot colours of the extra trays, chosen to differ from
// each other and from the orange of the NetBird logo.
var badgePalette = []color.NRGBA{
	{R: 0x25, G: 0x63, B: 0xEB, A: 0xFF}, // blue
	{R: 0x16, G: 0xA3, B: 0x4A, A: 0xFF}, // green
	{R: 0x93, G: 0x33, B: 0xEA, A: 0xFF}, // purple
	{R: 0xDC, G: 0x26, B: 0x26, A: 0xFF}, // red
	{R: 0x0E, G: 0x9F, B: 0xAE, A: 0xFF}, // teal
}

var badged sync.Map // key: icon hash and suffix, value: []byte

// badgeFlag picks the dot colour by number, so two trays never share one:
// "netbird instance add" passes the number of the instance's interface (wt1 is 1).
// Without it the colour follows from the instance name.
var badgeFlag = flag.Int("badge", 0, "colour number of the tray's instance dot (1 = blue, 2 = green, ...); default: derived from the daemon address")

// badgeForInstance marks the tray icon of an instance other than the default one
// with a coloured dot, so several trays can be told apart at a glance. The
// default instance's icon is returned untouched.
func badgeForInstance(icon, dark []byte) ([]byte, []byte) {
	suffix := flagInstanceSuffix()
	if suffix == "" {
		return icon, dark
	}
	return badgePNG(icon, suffix), badgePNG(dark, suffix)
}

// badgeColor picks the dot colour for an instance: by number when one was given,
// otherwise from its name, so the same instance always gets the same colour.
func badgeColor(suffix string) color.NRGBA {
	if *badgeFlag > 0 {
		return badgePalette[(*badgeFlag-1)%len(badgePalette)]
	}

	h := fnv.New32a()
	_, _ = h.Write([]byte(suffix))
	return badgePalette[int(h.Sum32())%len(badgePalette)]
}

// badgePNG returns the icon with the dot in its lower right corner. Anything that
// cannot be decoded is returned as it was: a plain icon is better than none.
func badgePNG(data []byte, suffix string) []byte {
	if len(data) == 0 {
		return data
	}

	h := fnv.New64a()
	_, _ = h.Write(data)
	_, _ = h.Write([]byte(suffix))
	key := h.Sum64()
	if cached, ok := badged.Load(key); ok {
		return cached.([]byte)
	}

	src, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return data
	}

	out := drawBadge(src, badgeColor(suffix))
	var buf bytes.Buffer
	if err := png.Encode(&buf, out); err != nil {
		return data
	}

	badged.Store(key, buf.Bytes())
	return buf.Bytes()
}

// drawBadge draws a filled circle with a white rim, smoothed at the edge.
func drawBadge(src image.Image, fill color.NRGBA) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(b)
	draw.Draw(dst, b, src, b.Min, draw.Src)

	size := float64(min(b.Dx(), b.Dy()))
	rim := math.Max(1, size/28)
	radius := size * 0.24
	cx := float64(b.Max.X) - radius - rim - 0.5
	cy := float64(b.Max.Y) - radius - rim - 0.5

	outer := coverage(b, cx, cy, radius+rim)
	inner := coverage(b, cx, cy, radius)

	draw.DrawMask(dst, b, &image.Uniform{C: color.White}, image.Point{}, outer, b.Min, draw.Over)
	draw.DrawMask(dst, b, &image.Uniform{C: fill}, image.Point{}, inner, b.Min, draw.Over)
	return dst
}

// coverage returns a mask that is opaque inside the circle and fades over one pixel at its edge.
func coverage(b image.Rectangle, cx, cy, r float64) *image.Alpha {
	mask := image.NewAlpha(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			a := math.Min(1, math.Max(0, r+0.5-d))
			if a > 0 {
				mask.SetAlpha(x, y, color.Alpha{A: uint8(a * 255)})
			}
		}
	}
	return mask
}
