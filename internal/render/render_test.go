package render

import (
	"image"
	"image/color"
	"testing"

	"github.com/gogpu/gg/text"
	"golang.org/x/image/font/gofont/goregular"
)

// TestPlaceShipAnnotationsAvoidsMapAndLabelCollisions protects dense-team readability.
func TestPlaceShipAnnotationsAvoidsMapAndLabelCollisions(t *testing.T) {
	var (
		fontSource *text.FontSource
		err        error
	)

	if fontSource, err = text.NewFontSource(goregular.TTF); err != nil {
		t.Fatalf("NewFontSource() error = %v", err)
	}

	defer fontSource.Close()

	var (
		ships []Ship = []Ship{
			{X: 0, Z: 0, Team: 1, TeamKnown: true, ShipName: "Ship A", Health: 50000, MaxHealth: 60000},
			{X: 0, Z: 0, Team: 1, TeamKnown: true, ShipName: "Ship B", Health: 40000, MaxHealth: 60000},
			{X: 0, Z: 0, Team: 2, Enemy: true, Spotted: true, HasBeenSpotted: true, ShipName: "Ship C", Health: 30000, MaxHealth: 60000},
		}
		annotations []shipAnnotation = placeShipAnnotations(ships, Bounds{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100}, image.Rect(0, 0, 800, 800), fontSource.Face(12))
	)

	if len(annotations) != len(ships) {
		t.Fatalf("placeShipAnnotations() returned %d positions, want %d", len(annotations), len(ships))
	}

	for left := range annotations {
		if !annotations[left].Visible {
			t.Fatalf("placeShipAnnotations()[%d] is hidden in a resolvable cluster", left)
		}

		for right := left + 1; right < len(annotations); right++ {
			if annotations[left].Bounds.Overlaps(annotations[right].Bounds) {
				t.Fatalf("ship annotation boxes overlap: %v and %v", annotations[left].Bounds, annotations[right].Bounds)
			}
		}
	}
}

// TestOutlineShipIconKeepsOnlyTheSilhouetteEdge checks tactical states use outlines, not filled markers.
func TestOutlineShipIconKeepsOnlyTheSilhouetteEdge(t *testing.T) {
	var source *image.RGBA = image.NewRGBA(image.Rect(0, 0, 3, 3))
	for y := range 3 {
		for x := range 3 {
			source.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}

	var (
		outline color.RGBA  = color.RGBA{R: 230, G: 240, B: 255, A: 255}
		result  image.Image = outlineShipIcon(source, outline)
	)

	if result.At(0, 0) != outline || result.At(1, 1) != (color.RGBA{}) {
		t.Fatalf("outline pixels = corner %v, center %v; want edge color and transparent center", result.At(0, 0), result.At(1, 1))
	}
}

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

// TestRenderFrameDistinguishesShipVisibilityAndDeath checks all three tactical marker states.
func TestRenderFrameDistinguishesShipVisibilityAndDeath(t *testing.T) {
	var (
		base Frame = Frame{
			Layout:      Layout{Width: 400, Height: 300, LeftPanelWidth: 80, RightPanelWidth: 80, FooterHeight: 30},
			Map:         image.NewRGBA(image.Rect(0, 0, 100, 100)),
			WorldBounds: Bounds{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100},
			Ships:       []Ship{{X: 0, Z: 0, Spotted: true}},
		}
		spotted image.Image
		err     error
	)

	if spotted, err = RenderFrame(base); err != nil {
		t.Fatalf("RenderFrame(spotted) error = %v", err)
	}

	base.Ships[0].Spotted = false

	var unspotted image.Image
	if unspotted, err = RenderFrame(base); err != nil {
		t.Fatalf("RenderFrame(unspotted) error = %v", err)
	}

	base.Ships[0].Dead = true

	var dead image.Image
	if dead, err = RenderFrame(base); err != nil {
		t.Fatalf("RenderFrame(dead) error = %v", err)
	}

	if imagesEqual(spotted, unspotted) || imagesEqual(spotted, dead) || imagesEqual(unspotted, dead) {
		t.Fatal("spotted, unspotted, and dead markers should render differently")
	}
}

// TestRenderFrameHidesNeverSpottedEnemies checks unseen ships do not leak their initial positions.
func TestRenderFrameHidesNeverSpottedEnemies(t *testing.T) {
	var (
		base Frame = Frame{
			Layout:      Layout{Width: 400, Height: 300, LeftPanelWidth: 80, RightPanelWidth: 80, FooterHeight: 30},
			Map:         image.NewRGBA(image.Rect(0, 0, 100, 100)),
			WorldBounds: Bounds{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100},
			Ships:       []Ship{{X: 0, Z: 0, Team: 2, Enemy: true}},
		}
		hidden image.Image
		err    error
	)

	if hidden, err = RenderFrame(base); err != nil {
		t.Fatalf("RenderFrame(hidden) error = %v", err)
	}

	base.Ships = nil

	var empty image.Image
	if empty, err = RenderFrame(base); err != nil {
		t.Fatalf("RenderFrame(empty) error = %v", err)
	}

	if !imagesEqual(hidden, empty) {
		t.Fatal("never-spotted enemy should not be visible on the map")
	}

	base.Ships = []Ship{{X: 0, Z: 0, Team: 2, Enemy: true, HasBeenSpotted: true}}

	var lastKnown image.Image
	if lastKnown, err = RenderFrame(base); err != nil {
		t.Fatalf("RenderFrame(last-known) error = %v", err)
	}

	if imagesEqual(hidden, lastKnown) {
		t.Fatal("previously spotted enemy should render its last-known position")
	}
}

// TestRenderFrameShowsVehicleHealthBar checks current HP changes are visible on the tactical map.
func TestRenderFrameShowsVehicleHealthBar(t *testing.T) {
	var (
		frame Frame = Frame{
			Layout:      Layout{Width: 400, Height: 300, LeftPanelWidth: 80, RightPanelWidth: 80, FooterHeight: 30},
			Map:         image.NewRGBA(image.Rect(0, 0, 100, 100)),
			WorldBounds: Bounds{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100},
			Ships:       []Ship{{X: 0, Z: 0, Team: 1, Spotted: true, Health: 100, MaxHealth: 100}},
		}
		fullHealth image.Image
		err        error
	)

	if fullHealth, err = RenderFrame(frame); err != nil {
		t.Fatalf("RenderFrame(full health) error = %v", err)
	}

	frame.Ships[0].Health = 50

	var halfHealth image.Image
	if halfHealth, err = RenderFrame(frame); err != nil {
		t.Fatalf("RenderFrame(half health) error = %v", err)
	}

	if imagesEqual(fullHealth, halfHealth) {
		t.Fatal("different ship health values should render different health bars")
	}
}

// TestRenderFrameShowsSquadronMarkers checks aircraft stay visible without optional image assets.
func TestRenderFrameShowsSquadronMarkers(t *testing.T) {
	var (
		frame Frame = Frame{
			Layout:      Layout{Width: 400, Height: 300, LeftPanelWidth: 80, RightPanelWidth: 80, FooterHeight: 30},
			Map:         image.NewRGBA(image.Rect(0, 0, 100, 100)),
			WorldBounds: Bounds{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100},
			Squadrons:   []Squadron{{X: -20, Z: 10}, {X: 20, Z: -10, Enemy: true}},
		}
		withAircraft image.Image
		err          error
	)

	if withAircraft, err = RenderFrame(frame); err != nil {
		t.Fatalf("RenderFrame(aircraft) error = %v", err)
	}

	frame.Squadrons = nil

	var withoutAircraft image.Image
	if withoutAircraft, err = RenderFrame(frame); err != nil {
		t.Fatalf("RenderFrame(without aircraft) error = %v", err)
	}

	if imagesEqual(withAircraft, withoutAircraft) {
		t.Fatal("squadron markers should add visible aircraft glyphs to the map")
	}
}

// TestRenderFrameShowsTorpedoMarkers checks projectile markers render independently of ships and planes.
func TestRenderFrameShowsTorpedoMarkers(t *testing.T) {
	var (
		frame Frame = Frame{
			Layout:      Layout{Width: 400, Height: 300, LeftPanelWidth: 80, RightPanelWidth: 80, FooterHeight: 30},
			Map:         image.NewRGBA(image.Rect(0, 0, 100, 100)),
			WorldBounds: Bounds{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100},
			Torpedoes:   []Torpedo{{X: -20, Z: 10, TeamKnown: true}, {X: 20, Z: -10, TeamKnown: true, Enemy: true}},
		}
		withTorpedoes image.Image
		err           error
	)

	if withTorpedoes, err = RenderFrame(frame); err != nil {
		t.Fatalf("RenderFrame(torpedoes) error = %v", err)
	}

	frame.Torpedoes = nil

	var withoutTorpedoes image.Image
	if withoutTorpedoes, err = RenderFrame(frame); err != nil {
		t.Fatalf("RenderFrame(without torpedoes) error = %v", err)
	}

	if imagesEqual(withTorpedoes, withoutTorpedoes) {
		t.Fatal("torpedo markers should add visible markers to the map")
	}
}

// TestRenderFrameScalesTheWholeHUDWithOutputResolution verifies text and markers keep design proportions.
func TestRenderFrameScalesTheWholeHUDWithOutputResolution(t *testing.T) {
	var (
		frame Frame = Frame{
			Layout:      Layout{Width: 1328, Height: 800, LeftPanelWidth: 280, RightPanelWidth: 280, FooterHeight: 48},
			Map:         image.NewRGBA(image.Rect(0, 0, 100, 100)),
			WorldBounds: Bounds{MinX: -100, MaxX: 100, MinZ: -100, MaxZ: 100},
			Torpedoes:   []Torpedo{{X: 0, Z: 0, TeamKnown: true}},
		}
		base image.Image
		err  error
	)

	if base, err = RenderFrame(frame); err != nil {
		t.Fatalf("RenderFrame(base) error = %v", err)
	}

	frame.Layout = Layout{Width: 2656, Height: 1600, LeftPanelWidth: 560, RightPanelWidth: 560, FooterHeight: 96}

	var doubled image.Image
	if doubled, err = RenderFrame(frame); err != nil {
		t.Fatalf("RenderFrame(doubled) error = %v", err)
	}

	if base.Bounds().Dx() != 1328 || base.Bounds().Dy() != 800 || doubled.Bounds().Dx() != 2656 || doubled.Bounds().Dy() != 1600 {
		t.Fatalf("rendered output bounds = %v and %v, want 1328x800 and 2656x1600", base.Bounds(), doubled.Bounds())
	}

	if base.At(664, 391) != doubled.At(1328, 782) {
		t.Fatalf("center torpedo marker changed color across scales: %v != %v", base.At(664, 391), doubled.At(1328, 782))
	}
}

// imagesEqual compares rendered pixels without relying on exact rasterizer output.
func imagesEqual(left image.Image, right image.Image) (equal bool) {
	if left.Bounds() != right.Bounds() {
		equal = false
		return
	}

	for y := left.Bounds().Min.Y; y < left.Bounds().Max.Y; y++ {
		for x := left.Bounds().Min.X; x < left.Bounds().Max.X; x++ {
			if left.At(x, y) != right.At(x, y) {
				equal = false
				return
			}
		}
	}

	equal = true
	return
}
