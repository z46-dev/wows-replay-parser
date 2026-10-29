package video

import (
	"encoding/xml"
	"fmt"
	"io/fs"
	"math"
)

type spaceSettings struct {
	Bounds struct {
		MinX      float64 `xml:"minX,attr"`
		MaxX      float64 `xml:"maxX,attr"`
		MinY      float64 `xml:"minY,attr"`
		MaxY      float64 `xml:"maxY,attr"`
		MinXValue float64 `xml:"minX"`
		MaxXValue float64 `xml:"maxX"`
		MinYValue float64 `xml:"minY"`
		MaxYValue float64 `xml:"maxY"`
	} `xml:"bounds"`
	ChunkSize float64 `xml:"chunkSize"`
	Terrain   struct {
		ChunkSize float64 `xml:"chunkSize"`
	} `xml:"terrain"`
}

// LoadWorldBounds converts archived space.settings bounds into world coordinates.
func LoadWorldBounds(archive fs.FS, name string) (bounds Bounds, err error) {
	var data []byte
	if data, err = fs.ReadFile(archive, name); err != nil {
		return
	}

	var settings spaceSettings
	if err = xml.Unmarshal(data, &settings); err != nil {
		return
	}
	if settings.Bounds.MinX == 0 {
		settings.Bounds.MinX = settings.Bounds.MinXValue
	}
	if settings.Bounds.MaxX == 0 {
		settings.Bounds.MaxX = settings.Bounds.MaxXValue
	}
	if settings.Bounds.MinY == 0 {
		settings.Bounds.MinY = settings.Bounds.MinYValue
	}
	if settings.Bounds.MaxY == 0 {
		settings.Bounds.MaxY = settings.Bounds.MaxYValue
	}

	var chunkSize float64 = settings.ChunkSize
	if chunkSize <= 0 {
		chunkSize = settings.Terrain.ChunkSize
	}
	if chunkSize <= 0 {
		chunkSize = 100
	}

	var (
		spaceWidth  float64 = math.Max(0, settings.Bounds.MaxX-settings.Bounds.MinX+1-4) * chunkSize
		spaceHeight float64 = math.Max(0, settings.Bounds.MaxY-settings.Bounds.MinY+1-4) * chunkSize
		spaceSize   float64 = math.Max(spaceWidth, spaceHeight)
	)
	bounds = Bounds{MinX: -spaceSize / 2, MaxX: spaceSize / 2, MinZ: -spaceSize / 2, MaxZ: spaceSize / 2}
	if bounds.MaxX <= bounds.MinX || bounds.MaxZ <= bounds.MinZ {
		err = fmt.Errorf("space settings contain invalid minimap bounds")
	}
	return
}
