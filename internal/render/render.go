// Package render creates display-independent replay frames.
package render

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"strings"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/text"
)

type (
	// Bounds describes the horizontal coordinates covered by a minimap image.
	Bounds struct {
		MinX float64
		MaxX float64
		MinZ float64
		MaxZ float64
	}

	// Layout reserves separate regions for the map, information panels, and supporting UI.
	Layout struct {
		Width           int
		Height          int
		LeftPanelWidth  int
		RightPanelWidth int
		FooterHeight    int
	}

	// Player contains the lineup details displayed in a team sidebar.
	Player struct {
		Name      string
		ClanTag   string
		ShipName  string
		Health    float64
		MaxHealth float64
		Damage    float64
		Alive     bool
	}

	// Ship is one map marker at the requested replay time.
	Ship struct {
		X       float64
		Z       float64
		Heading float64
		Team    int
	}

	// Frame describes all content needed to render one replay moment.
	Frame struct {
		Layout           Layout
		Map              image.Image
		MapName          string
		Clock            float64
		Duration         float64
		WorldBounds      Bounds
		LeftTeamName     string
		RightTeamName    string
		LeftTeam         []Player
		RightTeam        []Player
		Ships            []Ship
		FooterText       string
		Font             text.Face
		ShowResultsStats bool
	}
)

var (
	backgroundColor = color.RGBA{R: 8, G: 13, B: 22, A: 255}
	panelColor      = color.RGBA{R: 10, G: 16, B: 27, A: 255}
	gridColor       = color.RGBA{R: 28, G: 64, B: 72, A: 160}
	leftColor       = color.RGBA{R: 64, G: 224, B: 151, A: 255}
	rightColor      = color.RGBA{R: 244, G: 67, B: 74, A: 255}
	textColor       = color.RGBA{R: 235, G: 239, B: 244, A: 255}
	mutedColor      = color.RGBA{R: 146, G: 157, B: 171, A: 255}
)

// RenderFrame draws a map view with independently sized information panels.
func RenderFrame(frame Frame) (result image.Image, err error) {
	var layout Layout = frame.Layout
	if layout.Width <= 0 {
		layout.Width = 1328
	}

	if layout.Height <= 0 {
		layout.Height = 800
	}
	
	if layout.LeftPanelWidth < 0 || layout.RightPanelWidth < 0 || layout.FooterHeight < 0 || layout.LeftPanelWidth+layout.RightPanelWidth >= layout.Width || layout.FooterHeight >= layout.Height {
		err = fmt.Errorf("invalid frame layout dimensions")
		return
	}
	
	if frame.Map == nil {
		err = fmt.Errorf("frame map image is required")
		return
	}
	
	if frame.WorldBounds.MaxX <= frame.WorldBounds.MinX || frame.WorldBounds.MaxZ <= frame.WorldBounds.MinZ {
		err = fmt.Errorf("frame world bounds must have positive width and height")
		return
	}

	var canvas *gg.Context = gg.NewContext(layout.Width, layout.Height)
	defer canvas.Close()

	canvas.SetColor(backgroundColor)
	canvas.ClearWithColor(gg.FromColor(backgroundColor))

	var (
		footerTop int             = layout.Height - layout.FooterHeight
		mapArea   image.Rectangle = image.Rect(layout.LeftPanelWidth, 0, layout.Width-layout.RightPanelWidth, footerTop)
	)
	
	if err = drawMap(canvas, frame, mapArea); err != nil {
		return
	}
	
	if err = drawSidebar(canvas, frame.LeftTeamName, frame.LeftTeam, 0, layout.LeftPanelWidth, footerTop, leftColor, frame.Font, frame.ShowResultsStats); err != nil {
		return
	}
	
	if err = drawSidebar(canvas, frame.RightTeamName, frame.RightTeam, layout.Width-layout.RightPanelWidth, layout.RightPanelWidth, footerTop, rightColor, frame.Font, frame.ShowResultsStats); err != nil {
		return
	}
	
	if err = drawFooter(canvas, frame, footerTop); err != nil {
		return
	}

	result = canvas.Image()
	return
}

// drawMap paints the minimap, coordinate grid, current ship markers, and match clock.
func drawMap(canvas *gg.Context, frame Frame, area image.Rectangle) (err error) {
	var mapBounds image.Rectangle = frame.Map.Bounds()
	if mapBounds.Empty() {
		err = fmt.Errorf("frame map image is empty")
		return
	}

	var (
		mapImage *gg.ImageBuf = gg.ImageBufFromImage(frame.Map)
		scale    float64      = math.Min(float64(area.Dx())/float64(mapBounds.Dx()), float64(area.Dy())/float64(mapBounds.Dy()))
		width    float64      = float64(mapBounds.Dx()) * scale
		height   float64      = float64(mapBounds.Dy()) * scale
		left     float64      = float64(area.Min.X) + (float64(area.Dx())-width)/2
		top      float64      = float64(area.Min.Y) + (float64(area.Dy())-height)/2
	)

	canvas.Push()
	canvas.ClipRect(float64(area.Min.X), float64(area.Min.Y), float64(area.Dx()), float64(area.Dy()))
	canvas.Translate(left, top)
	canvas.Scale(scale, scale)
	canvas.DrawImage(mapImage, 0, 0)
	canvas.Pop()

	var (
		mapRect image.Rectangle = image.Rect(int(math.Round(left)), int(math.Round(top)), int(math.Round(left+width)), int(math.Round(top+height)))
		columns int             = 10
		rows    int             = 10
	)
	
	canvas.SetColor(gridColor)
	canvas.SetLineWidth(1)
	for index := 0; index <= columns; index++ {
		var x float64 = float64(mapRect.Min.X) + float64(mapRect.Dx())*float64(index)/float64(columns)
		canvas.MoveTo(x, float64(mapRect.Min.Y))
		canvas.LineTo(x, float64(mapRect.Max.Y))
		if err = canvas.Stroke(); err != nil {
			return
		}
	}
	
	for index := 0; index <= rows; index++ {
		var y float64 = float64(mapRect.Min.Y) + float64(mapRect.Dy())*float64(index)/float64(rows)
		canvas.MoveTo(float64(mapRect.Min.X), y)
		canvas.LineTo(float64(mapRect.Max.X), y)
		if err = canvas.Stroke(); err != nil {
			return
		}
	}
	
	if frame.Font != nil {
		canvas.SetFont(frame.Font)
		canvas.SetColor(textColor)
		for index := 0; index < columns; index++ {
			canvas.DrawString(fmt.Sprintf("%d", index+1), float64(mapRect.Min.X)+float64(mapRect.Dx())*(float64(index)+0.5)/float64(columns), float64(mapRect.Min.Y)+14)
		}
	
		for index := 0; index < rows; index++ {
			canvas.DrawString(string(rune('A'+index)), float64(mapRect.Min.X)+4, float64(mapRect.Min.Y)+float64(mapRect.Dy())*(float64(index)+0.5)/float64(rows))
		}
	}

	for _, ship := range frame.Ships {
		if ship.X < frame.WorldBounds.MinX || ship.X > frame.WorldBounds.MaxX || ship.Z < frame.WorldBounds.MinZ || ship.Z > frame.WorldBounds.MaxZ {
			continue
		}

		var x, y float64 = mapPoint(frame.WorldBounds, mapRect, ship.X, ship.Z)
		var markerColor color.RGBA = textColor
		if ship.Team == 1 {
			markerColor = leftColor
		} else if ship.Team == 2 {
			markerColor = rightColor
		}
	
		canvas.Push()
		canvas.Translate(x, y)
		canvas.Rotate(ship.Heading)
		canvas.SetColor(markerColor)
		canvas.MoveTo(0, -7)
		canvas.LineTo(5, 5)
		canvas.LineTo(-5, 5)
		canvas.ClosePath()
		if err = canvas.Fill(); err != nil {
			canvas.Pop()
			return
		}
	
		canvas.Pop()
	}

	if frame.Font != nil {
		canvas.SetFont(frame.Font)
		canvas.SetColor(textColor)
		canvas.DrawString(frame.MapName, float64(area.Min.X)+12, 24)
		canvas.DrawString(fmt.Sprintf("%s  /  %s", formatClock(frame.Clock), formatClock(frame.Duration)), float64(area.Min.X)+float64(area.Dx())-120, 24)
	}

	return
}

// drawSidebar renders one team and its player health and damage values.
func drawSidebar(canvas *gg.Context, title string, players []Player, left int, width int, height int, teamColor color.RGBA, face text.Face, showResultsStats bool) (err error) {
	canvas.SetColor(panelColor)
	canvas.DrawRectangle(float64(left), 0, float64(width), float64(height))
	if err = canvas.Fill(); err != nil {
		return
	}
	
	canvas.SetColor(teamColor)
	canvas.DrawRectangle(float64(left), 0, float64(width), 34)
	if err = canvas.Fill(); err != nil {
		return
	}
	
	if face == nil {
		return
	}

	canvas.SetFont(face)
	canvas.SetColor(backgroundColor)
	canvas.DrawString(strings.ToUpper(title), float64(left)+10, 23)

	var rowHeight float64 = float64(height-34) / math.Max(1, float64(len(players)))
	if rowHeight > 64 {
		rowHeight = 64
	}
	
	for index, player := range players {
		var y float64 = 34 + float64(index)*rowHeight
		canvas.SetColor(color.RGBA{R: 28, G: 36, B: 48, A: 255})
		canvas.DrawRectangle(float64(left)+4, y+rowHeight-1, float64(width)-8, 1)
		if err = canvas.Fill(); err != nil {
			return
		}

		canvas.SetColor(textColor)
		canvas.DrawString(playerLabel(player), float64(left)+10, y+17)
		canvas.SetColor(mutedColor)
		canvas.DrawString(player.ShipName, float64(left)+10, y+33)
		if !showResultsStats {
			continue
		}

		canvas.SetColor(textColor)
		canvas.DrawString(fmt.Sprintf("%.0f / %.0f", player.Health, player.MaxHealth), float64(left)+10, y+45)
		canvas.DrawStringAnchored(fmt.Sprintf("%s  %s", formatDamage(player.Damage), aliveLabel(player.Alive)), float64(left)+float64(width)-10, y+45, 1, 0.5)

		var healthWidth float64
		if player.MaxHealth > 0 {
			healthWidth = math.Max(0, math.Min(1, player.Health/player.MaxHealth)) * (float64(width) - 20)
		}
	
		canvas.SetColor(color.RGBA{R: 44, G: 55, B: 67, A: 255})
		canvas.DrawRectangle(float64(left)+10, y+rowHeight-7, float64(width)-20, 4)
		if err = canvas.Fill(); err != nil {
			return
		}

		if healthWidth > 0 {
			canvas.SetColor(teamColor)
			canvas.DrawRectangle(float64(left)+10, y+rowHeight-7, healthWidth, 4)
			if err = canvas.Fill(); err != nil {
				return
			}
		}
	}

	return
}

// drawFooter paints a compact area reserved for replay annotations and extra statistics.
func drawFooter(canvas *gg.Context, frame Frame, top int) (err error) {
	if frame.Layout.FooterHeight == 0 {
		return
	}

	canvas.SetColor(backgroundColor)
	canvas.DrawRectangle(0, float64(top), float64(frame.Layout.Width), float64(frame.Layout.FooterHeight))
	if err = canvas.Fill(); err != nil || frame.Font == nil {
		return
	}

	canvas.SetFont(frame.Font)
	canvas.SetColor(mutedColor)
	canvas.DrawString(frame.FooterText, 14, float64(top)+float64(frame.Layout.FooterHeight)/2+5)
	return
}

// mapPoint converts world coordinates to their pixel location inside the minimap.
func mapPoint(bounds Bounds, area image.Rectangle, x float64, z float64) (pixelX float64, pixelY float64) {
	pixelX = float64(area.Min.X) + (x-bounds.MinX)/(bounds.MaxX-bounds.MinX)*float64(area.Dx())
	pixelY = float64(area.Min.Y) + (bounds.MaxZ-z)/(bounds.MaxZ-bounds.MinZ)*float64(area.Dy())
	return
}

// playerLabel combines a clan tag and player name for a compact lineup row.
func playerLabel(player Player) (label string) {
	if player.ClanTag != "" {
		label = "[" + player.ClanTag + "] "
	}

	label += player.Name
	return
}

// formatClock formats seconds as a short minutes-and-seconds timestamp.
func formatClock(seconds float64) (formatted string) {
	if seconds < 0 {
		seconds = 0
	}

	var wholeSeconds int = int(seconds)
	formatted = fmt.Sprintf("%02d:%02d", wholeSeconds/60, wholeSeconds%60)
	return
}

// formatDamage makes large damage totals easier to scan in a narrow sidebar.
func formatDamage(damage float64) (formatted string) {
	formatted = fmt.Sprintf("%.0f", damage)
	if damage >= 1000 {
		formatted = fmt.Sprintf("%.1fk", damage/1000)
	}

	return
}

// aliveLabel identifies a player's post-battle survival state.
func aliveLabel(alive bool) (label string) {
	if alive {
		label = "ALIVE"
	} else {
		label = "SUNK"
	}
	
	return
}

// SortPlayers sorts a lineup by player name for deterministic frame generation.
func SortPlayers(players []Player) {
	sort.Slice(players, func(left int, right int) bool {
		return players[left].Name < players[right].Name
	})
}
