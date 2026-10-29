package video

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"testing/fstest"
)

// TestArchivedMapAssets checks that the renderer reads map images and bounds through fs.FS.
func TestArchivedMapAssets(t *testing.T) {
	var imageData bytes.Buffer
	if err := png.Encode(&imageData, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}

	var archive fstest.MapFS = fstest.MapFS{
		"spaces/test/minimap.png":       &fstest.MapFile{Data: imageData.Bytes()},
		"spaces/test/minimap_water.png": &fstest.MapFile{Data: imageData.Bytes()},
		"spaces/test/space.settings":    &fstest.MapFile{Data: []byte(`<space><bounds minX="0" maxX="10" minY="0" maxY="10"/><chunkSize>100</chunkSize></space>`)},
	}
	var (
		mapImage image.Image
		bounds   Bounds
		err      error
	)
	if mapImage, err = loadMap(archive, "spaces/test"); err != nil {
		t.Fatal(err)
	}
	if bounds, err = LoadWorldBounds(archive, "spaces/test/space.settings"); err != nil {
		t.Fatal(err)
	}
	if mapImage.Bounds().Dx() != 2 || bounds.MaxX <= bounds.MinX {
		t.Fatalf("map size %v, bounds %+v", mapImage.Bounds(), bounds)
	}
}
