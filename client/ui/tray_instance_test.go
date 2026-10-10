//go:build !android && !ios && !freebsd && !js

package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInstanceTooltipLabel(t *testing.T) {
	assert.Equal(t, "", instanceTooltipLabel("", "", 0), "no network known")
	assert.Equal(t, "", instanceTooltipLabel("", "Roffonet", 1), "a single default instance keeps its tooltip")
	assert.Equal(t, "Roffonet", instanceTooltipLabel("", "Roffonet", 2), "two instances: the profile tells them apart")
	assert.Equal(t, "", instanceTooltipLabel("", "", 2), "default instance without a known profile stays plain")
	assert.Equal(t, "Smartsys", instanceTooltipLabel("netbird-private", "Smartsys", 1), "another daemon always shows its network")
	assert.Equal(t, "netbird-private", instanceTooltipLabel("netbird-private", "", 2), "falls back to the daemon name")
}

func testIcon(t *testing.T) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.SetNRGBA(x, y, color.NRGBA{R: 0xF6, G: 0x83, B: 0x30, A: 0xFF}) // the orange of the logo
		}
	}
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func decode(t *testing.T, data []byte) image.Image {
	img, err := png.Decode(bytes.NewReader(data))
	require.NoError(t, err)
	return img
}

func TestBadgeDrawsADotInTheLowerRightCorner(t *testing.T) {
	icon := testIcon(t)
	got := decode(t, badgePNG(icon, "netbird-private"))

	assert.Equal(t, image.Rect(0, 0, 64, 64), got.Bounds(), "same size")

	want := badgeColor("netbird-private")
	r, g, b, _ := got.At(64-9, 64-9).RGBA()
	assert.Equal(t, [3]uint32{uint32(want.R) * 0x101, uint32(want.G) * 0x101, uint32(want.B) * 0x101}, [3]uint32{r, g, b}, "centre of the dot has the badge colour")

	tr, tg, tb, _ := got.At(5, 5).RGBA()
	assert.Equal(t, [3]uint32{0xF6 * 0x101, 0x83 * 0x101, 0x30 * 0x101}, [3]uint32{tr, tg, tb}, "the rest of the icon is untouched")
}

func TestBadgeColourIsStableAndDiffersBetweenInstances(t *testing.T) {
	assert.Equal(t, badgeColor("netbird-private"), badgeColor("netbird-private"))

	seen := map[color.NRGBA]bool{}
	for _, name := range []string{"netbird-private", "netbird-work", "netbird-lab", "netbird-home", "netbird-x"} {
		seen[badgeColor(name)] = true
	}
	assert.Greater(t, len(seen), 1, "different instances do not all get one colour")
}

// A number given on the command line decides the colour, and different numbers
// never share one (up to the size of the palette).
func TestBadgeNumberChoosesTheColour(t *testing.T) {
	old := *badgeFlag
	t.Cleanup(func() { *badgeFlag = old })

	seen := map[color.NRGBA]bool{}
	for n := 1; n <= len(badgePalette); n++ {
		*badgeFlag = n
		c := badgeColor("same-name")
		assert.False(t, seen[c], "number %d reuses a colour", n)
		seen[c] = true
	}

	*badgeFlag = 1
	first := badgeColor("a")
	assert.Equal(t, first, badgeColor("b"), "with a number the name no longer matters")

	*badgeFlag = len(badgePalette) + 1
	assert.Equal(t, badgePalette[0], badgeColor("x"), "numbers beyond the palette wrap around")
}

func TestBadgeIsDeterministicAndCached(t *testing.T) {
	icon := testIcon(t)
	assert.Equal(t, badgePNG(icon, "a"), badgePNG(icon, "a"))
	assert.NotEqual(t, badgePNG(icon, "netbird-private"), icon, "the icon changed")
}

func TestBadgeLeavesBrokenIconsAlone(t *testing.T) {
	assert.Equal(t, []byte("not a png"), badgePNG([]byte("not a png"), "x"))
	assert.Empty(t, badgePNG(nil, "x"))
}

// The default instance's icons must come back byte for byte as they went in.
func TestDefaultInstanceIconIsUntouched(t *testing.T) {
	icon, dark := testIcon(t), testIcon(t)

	gotIcon, gotDark := badgeForInstance(icon, dark)

	assert.Equal(t, icon, gotIcon)
	assert.Equal(t, dark, gotDark)
}
