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

const (
	designWidth  int = 1328
	designHeight int = 800
	// WinningScore is the score threshold used by the domination HUD.
	WinningScore float64 = 1000
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
		TopBarHeight    int
	}

	// Player contains the lineup details displayed in a team sidebar.
	Player struct {
		EntityID           uint32
		Name               string
		ClanTag            string
		ShipName           string
		AccountDBID        uint64
		Icon               image.Image
		ClassIconDead      image.Image
		ClassIconUnspotted image.Image
		ClassOrder         int
		Self               bool
		Detected           bool
		Health             float64
		MaxHealth          float64
		Damage             float64
		Kills              float64
		Alive              bool
		TeamID             uint64
		ClassIcon          image.Image
	}

	// Ship is one map marker at the requested replay time.
	Ship struct {
		EntityID        uint32
		X               float64
		Z               float64
		Heading         float64
		Team            int
		TeamID          int64
		TeamKnown       bool
		Health          float64
		MaxHealth       float64
		Self            bool
		Enemy           bool
		Spotted         bool
		DetectedByEnemy bool
		VisibilityKnown bool
		HasBeenSpotted  bool
		Dead            bool
		ShipName        string
		Icon            image.Image
		DeadIcon        image.Image
		UnspottedIcon   image.Image
	}

	// CapturePoint marks a map objective at its replay-supplied position.
	CapturePoint struct {
		X         float64
		Z         float64
		Radius    float64
		TeamID    int64
		TeamKnown bool
		Enemy     bool
		Progress  float64
		Label     string
	}

	// Squadron is an aircraft minimap marker at the requested replay time.
	Squadron struct {
		X            float64
		Z            float64
		TeamID       int64
		TeamKnown    bool
		Enemy        bool
		GameParamsID uint32
		IconName     string
	}

	// Torpedo marks a moving projectile at its world-space position.
	Torpedo struct {
		X         float64
		Z         float64
		OwnerID   uint32
		TeamID    int64
		TeamKnown bool
		Enemy     bool
	}

	// ShellTracer is the visible segment of a shell's flight at one replay time.
	ShellTracer struct {
		TailX     float64
		TailZ     float64
		HeadX     float64
		HeadZ     float64
		TeamKnown bool
		Enemy     bool
		Self      bool
	}

	// Frame describes all content needed to render one replay moment.
	Frame struct {
		Layout              Layout
		Map                 image.Image
		MapBuffer           *gg.ImageBuf
		MapName             string
		Clock               float64
		Duration            float64
		BattleLimit         float64
		LeftStartingPoints  float64
		RightStartingPoints float64
		LeftTeamPoints      float64
		RightTeamPoints     float64
		LeftPointsPerSec    float64
		RightPointsPerSec   float64
		WorldBounds         Bounds
		LeftTeamName        string
		RightTeamName       string
		LeftTeam            []Player
		RightTeam           []Player
		Ships               []Ship
		Squadrons           []Squadron
		PlaneIconNames      map[uint32]string
		Torpedoes           []Torpedo
		Shells              []ShellTracer
		PlaneIcons          map[string]image.Image
		KillIcon            image.Image
		CapturePoints       []CapturePoint
		FooterText          string
		Font                text.Face
		BoldFont            text.Face
	}

	shipAnnotation struct {
		Name    string
		CenterX float64
		CenterY float64
		Bounds  image.Rectangle
		Visible bool
	}
)

var (
	backgroundColor  = color.RGBA{R: 8, G: 13, B: 22, A: 255}
	panelColor       = color.RGBA{R: 10, G: 16, B: 27, A: 255}
	gridColor        = color.RGBA{R: 28, G: 64, B: 72, A: 160}
	captureColor     = color.RGBA{R: 248, G: 204, B: 92, A: 230}
	leftColor        = color.RGBA{R: 64, G: 224, B: 151, A: 255}
	rightColor       = color.RGBA{R: 244, G: 67, B: 74, A: 255}
	textColor        = color.RGBA{R: 235, G: 239, B: 244, A: 255}
	mutedColor       = color.RGBA{R: 146, G: 157, B: 171, A: 255}
	topBarMutedColor = color.RGBA{R: 215, G: 221, B: 230, A: 255}
)

// RenderFrame draws the same design-sized HUD at the requested output scale without stretching its map.
func RenderFrame(frame Frame) (result image.Image, err error) {
	var layout Layout = frame.Layout
	if layout.Width <= 0 {
		layout.Width = designWidth
	}

	if layout.Height <= 0 {
		layout.Height = designHeight
	}

	if layout.LeftPanelWidth < 0 || layout.RightPanelWidth < 0 || layout.FooterHeight < 0 || layout.TopBarHeight < 0 || layout.LeftPanelWidth+layout.RightPanelWidth >= layout.Width || layout.FooterHeight+layout.TopBarHeight >= layout.Height {
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
		scale         float64 = math.Min(float64(layout.Width)/float64(designWidth), float64(layout.Height)/float64(designHeight))
		contentWidth  float64 = float64(designWidth) * scale
		contentHeight float64 = float64(designHeight) * scale
		left          float64 = (float64(layout.Width) - contentWidth) / 2
		top           float64 = (float64(layout.Height) - contentHeight) / 2
		designLayout  Layout  = Layout{
			Width:           designWidth,
			Height:          designHeight,
			LeftPanelWidth:  int(math.Round(float64(layout.LeftPanelWidth) * float64(designWidth) / float64(layout.Width))),
			RightPanelWidth: int(math.Round(float64(layout.RightPanelWidth) * float64(designWidth) / float64(layout.Width))),
			FooterHeight:    int(math.Round(float64(layout.FooterHeight) * float64(designHeight) / float64(layout.Height))),
			TopBarHeight:    int(math.Round(float64(layout.TopBarHeight) * float64(designHeight) / float64(layout.Height))),
		}
		contentBottom int             = designLayout.Height - designLayout.FooterHeight
		mapArea       image.Rectangle = image.Rect(designLayout.LeftPanelWidth, designLayout.TopBarHeight, designLayout.Width-designLayout.RightPanelWidth, contentBottom)
		renderFrame   Frame           = frame
	)

	renderFrame.Layout = designLayout
	canvas.Push()
	canvas.Translate(left, top)
	canvas.Scale(scale, scale)

	if err = drawMap(canvas, renderFrame, mapArea); err != nil {
		canvas.Pop()
		return
	}

	if err = drawSidebar(canvas, frame.LeftTeamName, frame.LeftTeam, 0, designLayout.LeftPanelWidth, designLayout.TopBarHeight, contentBottom, leftColor, frame.Font, frame.BoldFont, frame.KillIcon); err != nil {
		canvas.Pop()
		return
	}

	if err = drawSidebar(canvas, frame.RightTeamName, frame.RightTeam, designLayout.Width-designLayout.RightPanelWidth, designLayout.RightPanelWidth, designLayout.TopBarHeight, contentBottom, rightColor, frame.Font, frame.BoldFont, frame.KillIcon); err != nil {
		canvas.Pop()
		return
	}

	if err = drawTopBar(canvas, renderFrame, designLayout.TopBarHeight); err != nil {
		canvas.Pop()
		return
	}

	canvas.Pop()
	result = canvas.Image()
	return
}

// placeShipAnnotations moves labels away from nearby ship markers and each other.
func placeShipAnnotations(ships []Ship, worldBounds Bounds, mapRect image.Rectangle, face text.Face) (annotations []shipAnnotation) {
	annotations = make([]shipAnnotation, len(ships))
	if face == nil {
		return
	}

	var (
		occupied []image.Rectangle
		order    []int
	)

	for shipIndex, ship := range ships {
		if ship.Dead || ship.Enemy && (!ship.HasBeenSpotted || !ship.Spotted) || ship.X < worldBounds.MinX || ship.X > worldBounds.MaxX || ship.Z < worldBounds.MinZ || ship.Z > worldBounds.MaxZ {
			continue
		}

		var x, y float64 = mapPoint(worldBounds, mapRect, ship.X, ship.Z)
		occupied = append(occupied, image.Rect(int(math.Round(x-20)), int(math.Round(y-20)), int(math.Round(x+20)), int(math.Round(y+23))))
		if ship.ShipName != "" || !ship.Dead && ship.MaxHealth > 0 {
			order = append(order, shipIndex)
		}
	}

	sort.SliceStable(order, func(left int, right int) bool {
		var (
			leftShip  Ship = ships[order[left]]
			rightShip Ship = ships[order[right]]
		)

		if leftShip.Self != rightShip.Self {
			return leftShip.Self
		}

		if leftShip.Team != rightShip.Team {
			return leftShip.Team < rightShip.Team
		}

		return order[left] < order[right]
	})

	for _, shipIndex := range order {
		var (
			ship      Ship    = ships[shipIndex]
			x, y      float64 = mapPoint(worldBounds, mapRect, ship.X, ship.Z)
			name      string  = ship.ShipName
			width     float64
			height    float64
			textWidth float64 = face.Advance(name)
		)

		if width = math.Min(float64(mapRect.Dx()), textWidth+2); width <= 2 {
			continue
		}

		height = face.Metrics().LineHeight()

		var (
			verticalOffset   float64 = height/2 + 17
			horizontalOffset float64 = width/2 + 20
			candidates               = [16][2]float64{{0, -verticalOffset}, {0, verticalOffset}, {horizontalOffset, 0}, {-horizontalOffset, 0}, {horizontalOffset, -verticalOffset}, {-horizontalOffset, -verticalOffset}, {horizontalOffset, verticalOffset}, {-horizontalOffset, verticalOffset}, {0, -2 * verticalOffset}, {0, 2 * verticalOffset}, {2 * horizontalOffset, 0}, {-2 * horizontalOffset, 0}, {2 * horizontalOffset, -verticalOffset}, {-2 * horizontalOffset, -verticalOffset}, {2 * horizontalOffset, verticalOffset}, {-2 * horizontalOffset, verticalOffset}}
		)

		for _, candidate := range candidates {
			var (
				centerX          float64         = x + candidate[0]
				centerY          float64         = y + candidate[1]
				annotationBounds image.Rectangle = image.Rect(int(math.Round(centerX-width/2)), int(math.Round(centerY-height/2)), int(math.Round(centerX+width/2)), int(math.Round(centerY+height/2)))
			)

			if !annotationBounds.In(mapRect) {
				continue
			}

			var overlaps bool
			for _, other := range occupied {
				if annotationBounds.Overlaps(other) {
					overlaps = true
					break
				}
			}

			if overlaps {
				continue
			}

			annotations[shipIndex] = shipAnnotation{Name: name, CenterX: centerX, CenterY: centerY, Bounds: annotationBounds, Visible: true}
			occupied = append(occupied, annotationBounds)
			break
		}
	}

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
		mapImage *gg.ImageBuf = frame.MapBuffer
		scale    float64      = math.Min(float64(area.Dx())/float64(mapBounds.Dx()), float64(area.Dy())/float64(mapBounds.Dy()))
		width    float64      = float64(mapBounds.Dx()) * scale
		height   float64      = float64(mapBounds.Dy()) * scale
		left     float64      = float64(area.Min.X) + (float64(area.Dx())-width)/2
		top      float64      = float64(area.Min.Y) + (float64(area.Dy())-height)/2
	)
	if mapImage == nil {
		mapImage = gg.ImageBufFromImage(frame.Map)
	}

	canvas.Push()
	canvas.ClipRect(float64(area.Min.X), float64(area.Min.Y), float64(area.Dx()), float64(area.Dy()))
	canvas.Translate(left, top)
	canvas.Scale(scale, scale)
	canvas.DrawImage(mapImage, 0, 0)
	canvas.Pop()

	var (
		mapRect     image.Rectangle  = image.Rect(int(math.Round(left)), int(math.Round(top)), int(math.Round(left+width)), int(math.Round(top+height)))
		columns     int              = 10
		rows        int              = 10
		annotations []shipAnnotation = placeShipAnnotations(frame.Ships, frame.WorldBounds, mapRect, frame.Font)
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

	for _, capturePoint := range frame.CapturePoints {
		if capturePoint.X < frame.WorldBounds.MinX || capturePoint.X > frame.WorldBounds.MaxX || capturePoint.Z < frame.WorldBounds.MinZ || capturePoint.Z > frame.WorldBounds.MaxZ {
			continue
		}

		var (
			x, y   float64 = mapPoint(frame.WorldBounds, mapRect, capturePoint.X, capturePoint.Z)
			radius float64 = capturePoint.Radius / (frame.WorldBounds.MaxX - frame.WorldBounds.MinX) * float64(mapRect.Dx())
		)

		if radius <= 0 {
			radius = 18
		}

		var pointColor color.RGBA = captureColor
		if capturePoint.TeamKnown {
			pointColor = leftColor
			if capturePoint.Enemy {
				pointColor = rightColor
			}
		}

		pointColor.A = 45
		canvas.SetColor(pointColor)
		canvas.DrawCircle(x, y, radius)
		if err = canvas.Fill(); err != nil {
			return
		}

		pointColor.A = 230
		canvas.SetColor(pointColor)
		canvas.SetLineWidth(2)
		canvas.DrawCircle(x, y, radius)

		if err = canvas.Stroke(); err != nil {
			return
		}

		if capturePoint.Progress > 0 {
			canvas.SetColor(textColor)
			canvas.SetLineWidth(4)
			canvas.DrawArc(x, y, radius+2, -math.Pi/2, -math.Pi/2+math.Min(1, capturePoint.Progress)*2*math.Pi)

			if err = canvas.Stroke(); err != nil {
				return
			}
		}

		if frame.Font != nil && capturePoint.Label != "" {
			canvas.SetFont(frame.Font)
			canvas.SetColor(textColor)
			canvas.DrawStringAnchored(capturePoint.Label, x, y, 0.5, 0.5)
		}
	}

	canvas.Push()
	canvas.ClipRect(float64(mapRect.Min.X), float64(mapRect.Min.Y), float64(mapRect.Dx()), float64(mapRect.Dy()))
	for _, shell := range frame.Shells {
		var (
			tailX, tailY float64    = mapPoint(frame.WorldBounds, mapRect, shell.TailX, shell.TailZ)
			headX, headY float64    = mapPoint(frame.WorldBounds, mapRect, shell.HeadX, shell.HeadZ)
			shotColor    color.RGBA = mutedColor
		)
		if shell.TeamKnown {
			shotColor = leftColor
			if shell.Enemy {
				shotColor = rightColor
			} else if shell.Self {
				shotColor = textColor
			}
		}

		canvas.SetColor(shotColor)
		canvas.SetLineWidth(1.2)
		canvas.MoveTo(tailX, tailY)
		canvas.LineTo(headX, headY)
		if err = canvas.Stroke(); err != nil {
			canvas.Pop()
			return
		}
	}
	canvas.Pop()

	for shipIndex, ship := range frame.Ships {
		if ship.Enemy && !ship.HasBeenSpotted {
			continue
		}

		if ship.X < frame.WorldBounds.MinX || ship.X > frame.WorldBounds.MaxX || ship.Z < frame.WorldBounds.MinZ || ship.Z > frame.WorldBounds.MaxZ {
			continue
		}

		var (
			x, y        float64    = mapPoint(frame.WorldBounds, mapRect, ship.X, ship.Z)
			markerColor color.RGBA = textColor
		)

		if ship.Team == 1 {
			markerColor = leftColor
		} else if ship.Team == 2 {
			markerColor = rightColor
		}

		if ship.Dead {
			markerColor = color.RGBA{R: 92, G: 98, B: 108, A: 255}
		} else if ship.Enemy && !ship.Spotted {
			markerColor = color.RGBA{R: 238, G: 242, B: 248, A: 255}
		}

		canvas.Push()
		canvas.Translate(x, y)
		canvas.Rotate(ship.Heading)
		if ship.Icon == nil {
			if ship.Spotted {
				canvas.SetColor(markerColor)
				canvas.MoveTo(0, -7)
				canvas.LineTo(5, 5)
				canvas.LineTo(-5, 5)
				canvas.ClosePath()
				if err = canvas.Fill(); err != nil {
					canvas.Pop()
					return
				}
			} else {
				markerColor.A = 150
				canvas.SetColor(markerColor)
				canvas.SetLineWidth(2)
				canvas.MoveTo(0, -7)
				canvas.LineTo(5, 5)
				canvas.LineTo(-5, 5)
				canvas.ClosePath()
				if err = canvas.Stroke(); err != nil {
					canvas.Pop()
					return
				}
			}
		}

		var shipIcon image.Image = ship.Icon
		if ship.Dead && ship.DeadIcon != nil {
			shipIcon = ship.DeadIcon
		} else if ship.Enemy && !ship.Spotted && ship.UnspottedIcon != nil {
			shipIcon = ship.UnspottedIcon
		}

		if shipIcon != nil && !shipIcon.Bounds().Empty() {
			var (
				iconBounds image.Rectangle = shipIcon.Bounds()
				iconSize   float64         = 22
				iconScale  float64         = math.Min(iconSize/float64(iconBounds.Dx()), iconSize/float64(iconBounds.Dy()))
			)

			canvas.Scale(iconScale, iconScale)
			if (ship.Dead && ship.DeadIcon == nil) || (ship.Enemy && !ship.Spotted && ship.UnspottedIcon == nil) {
				shipIcon = outlineShipIcon(shipIcon, markerColor)
			}

			canvas.DrawImage(gg.ImageBufFromImage(shipIcon), -iconSize/(2*iconScale), -iconSize/(2*iconScale))
		}

		canvas.Pop()

		var annotation shipAnnotation = annotations[shipIndex]
		if annotation.Visible {
			canvas.SetFont(frame.Font)
			canvas.SetColor(color.RGBA{A: 220})
			canvas.DrawStringAnchored(annotation.Name, annotation.CenterX+1, annotation.CenterY+1, 0.5, 0.5)
			canvas.SetColor(textColor)
			if annotation.Name != "" {
				canvas.DrawStringAnchored(annotation.Name, annotation.CenterX, annotation.CenterY, 0.5, 0.5)
			}
		}

		if !ship.Dead && (!ship.Enemy || ship.Spotted) && ship.MaxHealth > 0 {
			var healthWidth float64 = math.Max(0, math.Min(1, ship.Health/ship.MaxHealth)) * 30
			canvas.SetColor(color.RGBA{R: 8, G: 13, B: 22, A: 220})
			canvas.DrawRectangle(x-16, y+18, 32, 4)
			if err = canvas.Fill(); err != nil {
				return
			}

			if healthWidth > 0 {
				if !ship.Spotted {
					markerColor.A = 150
				}

				canvas.SetColor(healthBarColor(ship.Health / ship.MaxHealth))
				canvas.DrawRectangle(x-15, y+19, healthWidth, 2)
				if err = canvas.Fill(); err != nil {
					return
				}
			}
		}
	}

	for _, squadron := range frame.Squadrons {
		if squadron.X < frame.WorldBounds.MinX || squadron.X > frame.WorldBounds.MaxX || squadron.Z < frame.WorldBounds.MinZ || squadron.Z > frame.WorldBounds.MaxZ {
			continue
		}

		var suffix string = "_ally"
		if squadron.Enemy {
			suffix = "_enemy"
		}
		if squadron.IconName == "" {
			squadron.IconName = "consumables/fighter"
		}

		var (
			x, y float64     = mapPoint(frame.WorldBounds, mapRect, squadron.X, squadron.Z)
			icon image.Image = frame.PlaneIcons[squadron.IconName+suffix]
		)

		if icon == nil || icon.Bounds().Empty() {
			continue
		}

		var (
			iconBounds image.Rectangle = icon.Bounds()
			iconScale  float64         = math.Min(22/float64(iconBounds.Dx()), 22/float64(iconBounds.Dy()))
		)

		canvas.Push()
		canvas.Translate(x-11, y-11)
		canvas.Scale(iconScale, iconScale)
		canvas.DrawImage(gg.ImageBufFromImage(icon), 0, 0)
		canvas.Pop()
	}

	for _, torpedo := range frame.Torpedoes {
		if torpedo.X < frame.WorldBounds.MinX || torpedo.X > frame.WorldBounds.MaxX || torpedo.Z < frame.WorldBounds.MinZ || torpedo.Z > frame.WorldBounds.MaxZ {
			continue
		}

		var (
			x, y        float64    = mapPoint(frame.WorldBounds, mapRect, torpedo.X, torpedo.Z)
			markerColor color.RGBA = mutedColor
		)

		if torpedo.TeamKnown {
			markerColor = leftColor
			if torpedo.Enemy {
				markerColor = rightColor
			}
		}

		canvas.Push()
		canvas.Translate(x, y)
		canvas.SetColor(backgroundColor)
		canvas.DrawCircle(0, 0, 5)

		if err = canvas.Fill(); err != nil {
			canvas.Pop()
			return
		}

		canvas.SetColor(markerColor)
		canvas.DrawCircle(0, 0, 3.5)

		if err = canvas.Fill(); err != nil {
			canvas.Pop()
			return
		}
		canvas.Pop()
	}

	return
}

// outlineShipIcon turns a unified ship marker into a single-color silhouette outline.
func outlineShipIcon(source image.Image, outline color.RGBA) (result image.Image) {
	var (
		bounds    image.Rectangle = source.Bounds()
		imageData *image.RGBA     = image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var _, _, _, alpha uint32 = source.At(x, y).RGBA()
			if alpha == 0 {
				continue
			}

			var edge bool
			for offsetY := -1; offsetY <= 1 && !edge; offsetY++ {
				for offsetX := -1; offsetX <= 1; offsetX++ {
					if offsetX == 0 && offsetY == 0 {
						continue
					}

					var neighborX, neighborY int = x + offsetX, y + offsetY
					if !image.Pt(neighborX, neighborY).In(bounds) {
						edge = true
						break
					}

					var _, _, _, neighborAlpha uint32 = source.At(neighborX, neighborY).RGBA()
					if neighborAlpha == 0 {
						edge = true
						break
					}
				}
			}

			if edge {
				outline.A = uint8(alpha >> 8)
				imageData.SetRGBA(x-bounds.Min.X, y-bounds.Min.Y, outline)
			}
		}
	}

	result = imageData
	return
}

// drawSidebar renders a fixed twelve-slot roster with live health and combat stats.
func drawSidebar(canvas *gg.Context, title string, players []Player, left int, width int, top int, bottom int, teamColor color.RGBA, face text.Face, boldFace text.Face, killIcon image.Image) (err error) {
	var rightAligned bool = left > 0
	var sidebarLeft float64 = float64(left)
	var sidebarRight float64 = float64(left + width)
	var textX float64 = sidebarLeft + 8
	var iconX float64 = sidebarLeft + 8
	var healthX float64 = sidebarLeft + 10
	if rightAligned {
		textX = sidebarRight - 8
		iconX = sidebarRight - 36
		healthX = sidebarRight - 10 - (float64(width)-20)/2
	}

	canvas.SetColor(panelColor)
	canvas.DrawRectangle(float64(left), float64(top), float64(width), float64(bottom-top))
	if err = canvas.Fill(); err != nil {
		return
	}

	canvas.SetColor(teamColor)
	canvas.DrawRectangle(float64(left), float64(top), float64(width), 34)
	if err = canvas.Fill(); err != nil {
		return
	}

	if face == nil {
		return
	}

	canvas.SetFont(face)
	canvas.SetColor(backgroundColor)
	canvas.DrawString(strings.ToUpper(title), float64(left)+10, float64(top)+23)

	var rowHeight float64 = float64(bottom-top-34) / 12
	for index, player := range players {
		if index >= 12 {
			break
		}

		var y float64 = float64(top+34) + float64(index)*rowHeight
		if !player.Alive {
			canvas.SetColor(color.RGBA{R: 24, G: 32, B: 43, A: 255})
			canvas.DrawRectangle(float64(left)+4, y, float64(width)-8, rowHeight-1)
			if err = canvas.Fill(); err != nil {
				return
			}
		}

		canvas.SetColor(color.RGBA{R: 28, G: 36, B: 48, A: 255})
		canvas.DrawRectangle(float64(left)+4, y+rowHeight-1, float64(width)-8, 1)
		if err = canvas.Fill(); err != nil {
			return
		}

		var playerName string = playerLabel(player)
		if player.Self {
			playerName += "  YOU"
		}

		var nameColor color.RGBA = textColor
		if player.Detected {
			nameColor = color.RGBA{R: 255, G: 220, B: 80, A: 255}
		}
		if player.Detected && player.Self {
			nameColor = textColor
		}
		if player.Alive {
			canvas.SetColor(nameColor)
		} else {
			canvas.SetColor(color.RGBA{R: 190, G: 200, B: 211, A: 255})
		}
		if boldFace != nil {
			canvas.SetFont(boldFace)
		} else {
			canvas.SetFont(face)
		}
		canvas.Push()
		var textClipX float64 = float64(left) + 8
		if rightAligned {
			textClipX = float64(left) + 96
		}
		canvas.ClipRect(textClipX, y+2, float64(width)-104, rowHeight-3)
		canvas.Translate(textX, y+16)
		playerName = fitSidebarText(canvas, playerName, float64(width)-104)
		if rightAligned {
			canvas.DrawStringAnchored(playerName, 0, 0, 1, 0.5)
		} else {
			canvas.DrawString(playerName, 0, 0)
		}
		canvas.Pop()

		canvas.SetFont(face)
		var statColor color.RGBA = textColor
		if !player.Alive {
			statColor = mutedColor
		}
		canvas.SetColor(statColor)
		var damageAnchor float64 = float64(left + width - 10)
		var statRight float64 = float64(left+width) - 62
		if rightAligned {
			damageAnchor = float64(left + 10)
			statRight = float64(left + 62)
			canvas.DrawStringAnchored(formatDamage(player.Damage), damageAnchor, y+16, 0, 0.5)
		} else {
			canvas.DrawStringAnchored(formatDamage(player.Damage), damageAnchor, y+16, 1, 0.5)
		}
		if killIcon != nil {
			var iconBounds image.Rectangle = killIcon.Bounds()
			if !iconBounds.Empty() {
				var iconScale float64 = math.Min(13/float64(iconBounds.Dx()), 13/float64(iconBounds.Dy()))
				canvas.Push()
				if rightAligned {
					canvas.Translate(statRight+13, y+9)
					canvas.Scale(-iconScale, iconScale)
				} else {
					canvas.Translate(statRight, y+9)
					canvas.Scale(iconScale, iconScale)
				}
				canvas.DrawImage(gg.ImageBufFromImage(killIcon), 0, 0)
				canvas.Pop()
				statRight += 16
			}
		}
		if rightAligned {
			canvas.DrawStringAnchored(formatDamage(player.Kills), statRight, y+16, 1, 0.5)
		} else {
			canvas.DrawString(formatDamage(player.Kills), statRight, y+16)
		}

		if player.ClassIcon != nil {
			var iconBounds image.Rectangle = player.ClassIcon.Bounds()
			if !iconBounds.Empty() {
				var iconScale float64 = math.Min(28/float64(iconBounds.Dx()), 13/float64(iconBounds.Dy()))
				canvas.Push()
				if rightAligned {
					canvas.Translate(iconX+14, y+26)
					canvas.Rotate(-math.Pi / 2)
				} else {
					canvas.Translate(iconX+14, y+26)
					canvas.Rotate(math.Pi / 2)
				}
				canvas.Scale(iconScale, iconScale)
				canvas.DrawImage(gg.ImageBufFromImage(player.ClassIcon), -float64(iconBounds.Dx())/2, -float64(iconBounds.Dy())/2)
				canvas.Pop()
			}
		}
		canvas.SetColor(mutedColor)
		canvas.SetFont(face)
		if rightAligned {
			canvas.DrawStringAnchored(fitSidebarText(canvas, player.ShipName, float64(width)-48), iconX-8, y+31, 1, 0.5)
		} else {
			canvas.DrawString(fitSidebarText(canvas, player.ShipName, float64(width)-48), float64(left)+40, y+31)
		}

		var (
			healthBarWidth float64 = (float64(width) - 20) / 2
			healthWidth    float64
			healthY        float64 = y + rowHeight - 17
		)
		if player.MaxHealth > 0 {
			healthWidth = math.Max(0, math.Min(1, player.Health/player.MaxHealth)) * healthBarWidth
		}

		canvas.SetColor(color.RGBA{R: 44, G: 55, B: 67, A: 255})
		canvas.DrawRectangle(healthX, healthY, healthBarWidth, 13)
		if err = canvas.Fill(); err != nil {
			return
		}

		if healthWidth > 0 {
			canvas.SetColor(healthBarColor(player.Health / player.MaxHealth))
			if rightAligned {
				canvas.DrawRectangle(healthX+healthBarWidth-healthWidth, healthY, healthWidth, 13)
			} else {
				canvas.DrawRectangle(healthX, healthY, healthWidth, 13)
			}
			if err = canvas.Fill(); err != nil {
				return
			}
		}
		canvas.SetFont(face)
		var healthText string = fmt.Sprintf("%.0f / %.0f", player.Health, player.MaxHealth)
		canvas.SetColor(backgroundColor)
		canvas.DrawStringAnchored(healthText, healthX+healthBarWidth/2+1, healthY+11, 0.5, 0.5)
		canvas.SetColor(textColor)
		canvas.DrawStringAnchored(healthText, healthX+healthBarWidth/2, healthY+10, 0.5, 0.5)
	}

	return
}

// fitSidebarText keeps roster labels inside their column without cutting through a glyph.
func fitSidebarText(canvas *gg.Context, value string, maxWidth float64) (fitted string) {
	if width, _ := canvas.MeasureString(value); width <= maxWidth {
		fitted = value
		return
	}

	var characters []rune = []rune(value)
	for len(characters) > 0 {
		characters = characters[:len(characters)-1]
		fitted = string(characters) + "…"
		if width, _ := canvas.MeasureString(fitted); width <= maxWidth {
			return
		}
	}

	fitted = "…"
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

// drawTopBar paints the battle countdown and score progress without adding other HUD content.
func drawTopBar(canvas *gg.Context, frame Frame, height int) (err error) {
	if height <= 0 {
		return
	}

	canvas.SetColor(backgroundColor)
	canvas.DrawRectangle(0, 0, float64(frame.Layout.Width), float64(height))
	if err = canvas.Fill(); err != nil {
		return
	}

	var (
		center        float64   = float64(frame.Layout.Width) / 2
		barWidth      float64   = center
		leftProgress  float64   = math.Max(0, math.Min(1, frame.LeftTeamPoints/WinningScore))
		rightProgress float64   = math.Max(0, math.Min(1, frame.RightTeamPoints/WinningScore))
		font          text.Face = frame.BoldFont
	)
	if font == nil {
		font = frame.Font
	}

	canvas.SetColor(leftColor)
	canvas.DrawRectangle(0, 0, barWidth*leftProgress, float64(height))
	if err = canvas.Fill(); err != nil {
		return
	}
	canvas.SetColor(rightColor)
	canvas.DrawRectangle(float64(frame.Layout.Width)-barWidth*rightProgress, 0, barWidth*rightProgress, float64(height))
	if err = canvas.Fill(); err != nil || font == nil {
		return
	}

	canvas.SetFont(font)
	canvas.SetColor(textColor)
	canvas.DrawStringAnchored(fmt.Sprintf("%.0f", frame.LeftTeamPoints), 12, float64(height)/2+6, 0, 0.5)
	canvas.SetColor(topBarMutedColor)
	canvas.Push()
	canvas.Translate(65, float64(height)/2+6)
	canvas.Scale(0.8, 0.8)
	canvas.DrawStringAnchored(formatTimeToWin(frame.LeftTeamPoints, frame.LeftPointsPerSec), 0, 0, 0, 0.5)
	canvas.Pop()
	canvas.SetColor(textColor)
	canvas.DrawStringAnchored(fmt.Sprintf("%.0f", frame.RightTeamPoints), float64(frame.Layout.Width)-12, float64(height)/2+6, 1, 0.5)
	canvas.SetColor(topBarMutedColor)
	canvas.Push()
	canvas.Translate(float64(frame.Layout.Width)-65, float64(height)/2+6)
	canvas.Scale(0.8, 0.8)
	canvas.DrawStringAnchored(formatTimeToWin(frame.RightTeamPoints, frame.RightPointsPerSec), 0, 0, 1, 0.5)
	canvas.Pop()

	if frame.BattleLimit > 0 {
		canvas.SetColor(textColor)
		canvas.DrawStringAnchored(formatClock(frame.BattleLimit-frame.Clock), center, float64(height)/2+6, 0.5, 0.5)
	}
	return
}

// formatTimeToWin estimates remaining score time from the recent authoritative score rate.
func formatTimeToWin(points float64, pointsPerSecond float64) (formatted string) {
	if pointsPerSecond <= 0 || points >= WinningScore {
		formatted = "--:--"
		return
	}

	formatted = formatClock((WinningScore - points) / pointsPerSecond)
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

// SortPlayers orders classes, ship names, and survival state deterministically.
func SortPlayers(players []Player) {
	sort.SliceStable(players, func(left int, right int) bool {
		if players[left].Alive != players[right].Alive {
			return players[left].Alive
		}
		if players[left].ClassOrder != players[right].ClassOrder {
			return players[left].ClassOrder < players[right].ClassOrder
		}
		if players[left].ShipName != players[right].ShipName {
			return strings.ToLower(players[left].ShipName) < strings.ToLower(players[right].ShipName)
		}
		return strings.ToLower(players[left].Name) < strings.ToLower(players[right].Name)
	})
}

// healthBarColor matches the game's three-band green/yellow/red HP cue.
func healthBarColor(fraction float64) (healthColor color.RGBA) {
	if fraction > 0.66 {
		healthColor = color.RGBA{R: 0, G: 255, B: 0, A: 255}
	} else if fraction > 0.33 {
		healthColor = color.RGBA{R: 255, G: 255, B: 0, A: 255}
	} else {
		healthColor = color.RGBA{R: 255, A: 255}
	}
	return
}
