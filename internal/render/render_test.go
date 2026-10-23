package render

import (
	"image"
	"image/color"
	"testing"
)

// TestRenderFrameCreatesIndependentMapAndSidebarRegions checks the basic three-panel canvas.
func TestRenderFrameCreatesIndependentMapAndSidebarRegions(t *testing.T) {
	var mapImage *image.RGBA = image.NewRGBA(image.Rect(0, 0, 100, 100))
	for y := mapImage.Bounds().Min.Y; y < mapImage.Bounds().Max.Y; y++ {
		for x := mapImage.Bounds().Min.X; x < mapImage.Bounds().Max.X; x++ {
			mapImage.Set(x, y, color.RGBA{R: 20, G: 90, B: 110, A: 255})
		}
	}

	var frame Frame = Frame{
		Layout: Layout{
			Width:           400,
			Height:          300,
			LeftPanelWidth:  80,
			RightPanelWidth: 80,
			FooterHeight:    30,
		},
		Map: mapImage,
		WorldBounds: Bounds{
			MinX: -100,
			MaxX: 100,
			MinZ: -100,
			MaxZ: 100,
		},
		LeftTeam:  []Player{{Name: "Alpha", ShipName: "Cruiser"}},
		RightTeam: []Player{{Name: "Bravo", ShipName: "Battleship"}},
		Ships:     []Ship{{X: 0, Z: 0}},
	}

	var (
		result image.Image
		err    error
	)
	if result, err = RenderFrame(frame); err != nil {
		t.Fatalf("RenderFrame() error = %v", err)
	}
	if result.Bounds().Dx() != frame.Layout.Width || result.Bounds().Dy() != frame.Layout.Height {
		t.Fatalf("RenderFrame() bounds = %v, want %dx%d", result.Bounds(), frame.Layout.Width, frame.Layout.Height)
	}
	if result.At(10, 60) == result.At(200, 60) {
		t.Fatal("left information panel and map region should have different colors")
	}
}

// TestRenderFrameRejectsMissingMap checks invalid frame inputs fail cleanly.
func TestRenderFrameRejectsMissingMap(t *testing.T) {
	var frame Frame = Frame{WorldBounds: Bounds{MinX: -1, MaxX: 1, MinZ: -1, MaxZ: 1}}
	if _, err := RenderFrame(frame); err == nil {
		t.Fatal("RenderFrame() error = nil, want missing map error")
	}
}
