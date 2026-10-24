package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/z46-dev/wows-replay-parser/internal/replaydata"
)

// TestCopyShipSilhouettesPackagesOnlyCatalogAssets checks extraction stays scoped to known ships.
func TestCopyShipSilhouettesPackagesOnlyCatalogAssets(t *testing.T) {
	var (
		sourceDirectory  string            = filepath.Join(t.TempDir(), "source")
		packageDirectory string            = filepath.Join(t.TempDir(), "package")
		iconPath         string            = filepath.Join(sourceDirectory, "PJSB508.png")
		files            map[string]string = make(map[string]string)
	)

	if err := os.MkdirAll(sourceDirectory, 0o755); err != nil {
		t.Fatalf("create source directory: %v", err)
	}

	var (
		icon *os.File
		err  error
	)

	if icon, err = os.Create(iconPath); err != nil {
		t.Fatalf("create test icon: %v", err)
	}

	var pixel *image.RGBA = image.NewRGBA(image.Rect(0, 0, 1, 1))
	pixel.Set(0, 0, color.White)
	if err = png.Encode(icon, pixel); err != nil {
		icon.Close()
		t.Fatalf("write test icon: %v", err)
	}

	if err = icon.Close(); err != nil {
		t.Fatalf("close test icon: %v", err)
	}

	var vehicles []replaydata.Vehicle = []replaydata.Vehicle{{ShipID: "PJSB508"}, {ShipID: "PJSB508"}, {ShipID: "missing"}, {ShipID: "../outside"}}
	if err = copyShipSilhouettes(sourceDirectory, packageDirectory, vehicles, files); err != nil {
		t.Fatalf("copyShipSilhouettes() error = %v", err)
	}

	if _, err = os.Stat(filepath.Join(packageDirectory, "ships_silhouettes", "PJSB508.png")); err != nil {
		t.Fatalf("copied ship silhouette missing: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("manifest contains %d files, want only the known ship icon", len(files))
	}
}

// TestCopySquadronIconsPackagesTheAircraftSilhouette checks aircraft extraction remains minimal.
func TestCopySquadronIconsPackagesTheAircraftSilhouette(t *testing.T) {
	var (
		sourceDirectory  string            = t.TempDir()
		packageDirectory string            = t.TempDir()
		files            map[string]string = make(map[string]string)
		err              error
		icon             *os.File
	)

	if icon, err = os.Create(filepath.Join(sourceDirectory, "aircraft.png")); err != nil {
		t.Fatalf("create aircraft icon: %v", err)
	}

	if err = png.Encode(icon, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		icon.Close()
		t.Fatalf("encode aircraft icon: %v", err)
	}

	if err = icon.Close(); err != nil {
		t.Fatalf("close aircraft icon: %v", err)
	}

	if err = copySquadronIcons(sourceDirectory, packageDirectory, files); err != nil {
		t.Fatalf("copySquadronIcons() error = %v", err)
	}

	if _, err = os.Stat(filepath.Join(packageDirectory, "squadrons", "aircraft.png")); err != nil {
		t.Fatalf("copied squadron icon missing: %v", err)
	}

	if len(files) != 1 {
		t.Fatalf("manifest contains %d files, want exactly one squadron icon", len(files))
	}
}

// TestCopyShipClassIconsPackagesTheUnifiedMinimapMarkers checks the five supported classes are included.
func TestCopyShipClassIconsPackagesTheUnifiedMinimapMarkers(t *testing.T) {
	var (
		sourceDirectory  string            = t.TempDir()
		packageDirectory string            = t.TempDir()
		files            map[string]string = make(map[string]string)
		species                            = []string{"aircarrier", "battleship", "cruiser", "destroyer", "submarine"}
		err              error
	)

	for _, class := range species {
		for _, state := range []string{"", "_dead", "_last_visible"} {
			if err = os.WriteFile(filepath.Join(sourceDirectory, "minimap_"+class+state+".svg"), []byte("<svg/>"), 0o600); err != nil {
				t.Fatalf("write %s%s class marker: %v", class, state, err)
			}
		}
	}

	if err = copyShipClassIcons(sourceDirectory, packageDirectory, files); err != nil {
		t.Fatalf("copyShipClassIcons() error = %v", err)
	}

	if len(files) != len(species)*3 {
		t.Fatalf("manifest contains %d files, want %d class markers and state variants", len(files), len(species)*4)
	}

	for _, class := range species {
		for _, state := range []string{"", "_dead", "_last_visible"} {
			if _, err = os.Stat(filepath.Join(packageDirectory, "ships_classes", class+state+".svg")); err != nil {
				t.Fatalf("copied %s%s class marker missing: %v", class, state, err)
			}
		}
	}
}
