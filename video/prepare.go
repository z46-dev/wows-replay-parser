package video

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/svg"
	"github.com/gogpu/gg/text"
	replayparser "github.com/z46-dev/wows-replay-parser"
	"github.com/z46-dev/wows-replay-parser/internal/events"
	"github.com/z46-dev/wows-replay-parser/internal/render"
	"github.com/z46-dev/wows-replay-parser/internal/replaydata"
	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"
)

type (
	// PrepareOptions selects local game assets and the replay clock for a prepared frame.
	PrepareOptions struct {
		GameDataPath string
		SpacesPath   string
		FontPath     string
		Clock        float64
	}

	mapResources struct {
		name     string
		image    image.Image
		bounds   Bounds
		font     text.Face
		boldFont text.Face
	}
)

// PrepareFrame loads map assets and turns replay results/events into a complete renderable frame.
func PrepareFrame(replay *replayparser.Replay, timeline *Timeline, schema *ReplaySchema, options PrepareOptions) (frame Frame, mapName string, err error) {
	if replay == nil || timeline == nil {
		err = fmt.Errorf("replay and event timeline are required")
		return
	}
	if options.SpacesPath == "" || options.GameDataPath == "" {
		err = fmt.Errorf("spaces and game-data paths are required to prepare a frame")
		return
	}

	var resources mapResources
	if resources, err = loadMapResources(options, replay); err != nil {
		return
	}
	mapName = resources.name

	var summary *events.BattleSummary
	for _, event := range timeline.Events {
		if event.Type == events.BattleResultsEvent && event.Results != nil && event.Results.Summary != nil {
			summary = event.Results.Summary
		}
	}
	if summary == nil {
		err = fmt.Errorf("replay does not contain decoded battle results for team lineups")
		return
	}

	var teamIDs []uint64
	for _, player := range summary.Players {
		if !containsTeam(teamIDs, player.TeamID) {
			teamIDs = append(teamIDs, player.TeamID)
		}
	}
	sort.Slice(teamIDs, func(left int, right int) bool { return teamIDs[left] < teamIDs[right] })
	if len(teamIDs) < 1 || len(teamIDs) > 2 {
		err = fmt.Errorf("expected one or two teams in battle results, found %d", len(teamIDs))
		return
	}

	teamIDs, _ = orderTeamsByOwnTeam(teamIDs, summary.Players, summary.AccountDBID)
	var (
		ownTeamID uint64 = teamIDs[0]
		leftTeam  []Player
		rightTeam []Player
		playerIDs []uint64
	)
	for accountID := range summary.Players {
		playerIDs = append(playerIDs, accountID)
	}
	sort.Slice(playerIDs, func(left int, right int) bool { return playerIDs[left] < playerIDs[right] })

	for _, accountID := range playerIDs {
		var (
			player                                    events.BattlePlayerResult = summary.Players[accountID]
			classIcon, classIconDead, classIconHidden image.Image
			iconColor                                 color.RGBA = color.RGBA{R: 64, G: 224, B: 151, A: 255}
			shipClass                                 string     = shipSpecies(replaydata.Vehicle{ShipID: player.ShipID})
		)
		if player.TeamID != ownTeamID {
			iconColor = color.RGBA{R: 244, G: 67, B: 74, A: 255}
		}
		if player.AccountDBID == summary.AccountDBID {
			iconColor = color.RGBA{R: 235, G: 239, B: 244, A: 255}
		}

		if schema != nil {
			if vehicle, ok := schema.Vehicle(player.VehicleTypeID); ok {
				shipClass = shipSpecies(vehicle)
				if classIcon, err = loadShipClassIcon(options.GameDataPath, options.SpacesPath, shipClass, ""); err != nil {
					return
				}
				if classIconDead, err = loadShipClassIcon(options.GameDataPath, options.SpacesPath, shipClass, "_dead"); err != nil {
					return
				}
				if classIconHidden, err = loadShipClassIcon(options.GameDataPath, options.SpacesPath, shipClass, "_last_visible"); err != nil {
					return
				}
				if classIcon != nil {
					classIcon = tintShipClassIcon(classIcon, iconColor)
				}
			}
		}

		var renderedPlayer Player = Player{Name: player.Name, ClanTag: player.ClanTag, ShipName: player.ShipName, AccountDBID: player.AccountDBID, ClassOrder: shipClassOrder(shipClass), ClassIcon: classIcon, ClassIconDead: classIconDead, ClassIconUnspotted: classIconHidden, Self: player.AccountDBID == summary.AccountDBID, Detected: player.TeamID == ownTeamID, Health: player.MaxHealth, MaxHealth: player.MaxHealth, Alive: true, TeamID: player.TeamID}
		if player.TeamID == ownTeamID {
			leftTeam = append(leftTeam, renderedPlayer)
		} else if len(teamIDs) == 2 && player.TeamID == teamIDs[1] {
			rightTeam = append(rightTeam, renderedPlayer)
		}
	}
	render.SortPlayers(leftTeam)
	render.SortPlayers(rightTeam)
	var killIcon image.Image
	if killIcon, err = loadKillIcon(options.GameDataPath, options.SpacesPath); err != nil {
		return
	}

	var (
		state          *FrameState            = NewFrameState(timeline)
		planeIconNames map[uint32]string      = make(map[uint32]string)
		planeIcons     map[string]image.Image = make(map[string]image.Image)
		duration       float64                = replayDuration(summary.Common.Duration, timeline, replay.Metadata.Duration)
	)
	for _, event := range timeline.Events {
		if event.Type != events.MinimapSquadronEvent || event.Squadron == nil {
			continue
		}

		var iconName string = "consumables/fighter"
		if schema != nil {
			if resolvedIcon, ok := schema.PlaneIcon(event.Squadron.GameParamsID); ok {
				iconName = resolvedIcon
			}
		}
		planeIconNames[event.Squadron.GameParamsID] = iconName
		for _, side := range []string{"ally", "enemy"} {
			var key string = iconName + "_" + side
			if _, exists := planeIcons[key]; exists {
				continue
			}
			if planeIcons[key], err = loadPlaneIcon(options.GameDataPath, options.SpacesPath, iconName, side); err != nil {
				return
			}
		}
	}
	frame = Frame{
		Layout:         Layout{Width: 1328, Height: 800, LeftPanelWidth: 280, RightPanelWidth: 280, TopBarHeight: 40},
		Map:            resources.image,
		MapBuffer:      gg.ImageBufFromImage(resources.image),
		MapName:        resources.name,
		Clock:          options.Clock,
		Duration:       duration,
		BattleLimit:    float64(replay.Metadata.BattleDuration),
		WorldBounds:    resources.bounds,
		LeftTeamName:   "OUR TEAM",
		RightTeamName:  "ENEMY TEAM",
		LeftTeam:       leftTeam,
		RightTeam:      rightTeam,
		PlaneIconNames: planeIconNames,
		PlaneIcons:     planeIcons,
		KillIcon:       killIcon,
		Font:           resources.font,
		BoldFont:       resources.boldFont,
	}
	state.initializeScore(0)
	frame = state.FrameAt(frame, options.Clock)
	return
}

// loadPlaneIcon reads the exact per-aircraft minimap marker packaged from the selected game version.
func loadPlaneIcon(gameDataPath string, spacesPath string, name string, side string) (icon image.Image, err error) {
	if !filepath.IsLocal(filepath.FromSlash(name)) || (side != "ally" && side != "enemy") {
		return
	}
	var relativePath string = filepath.FromSlash(name) + "_" + side + ".png"
	for _, iconPath := range []string{
		filepath.Join(gameDataPath, "plane_icons", relativePath),
		filepath.Join(filepath.Dir(spacesPath), "gui", "battle_hud", "markers_minimap", "plane", relativePath),
	} {
		var file *os.File
		if file, err = os.Open(iconPath); err != nil { // #nosec G304 -- name is local and side is allowlisted; the path stays beneath game asset roots.
			if os.IsNotExist(err) {
				err = nil
				continue
			}
			return
		}
		if icon, _, err = image.Decode(file); err != nil {
			err = errors.Join(err, file.Close())
			return
		}
		if err = file.Close(); err != nil {
			return
		}
		return
	}
	return
}

// replayDuration preserves trailing replay events that occur after the battle-results duration.
func replayDuration(resultDuration uint64, timeline *Timeline, fallback int) (duration float64) {
	duration = float64(resultDuration)
	for _, event := range timeline.Events {
		if float64(event.Clock) > duration {
			duration = float64(event.Clock)
		}
	}
	if duration <= 0 {
		duration = float64(fallback)
	}
	return
}

// LoadReplaySchema loads version-matched data for decoding and preparing replay frames.
func LoadReplaySchema(directory string, clientVersion string, allowVersionMixing bool) (schema *ReplaySchema, err error) {
	return replaydata.Load(directory, clientVersion, allowVersionMixing)
}

// EncodePNG renders and encodes a frame as PNG.
func EncodePNG(writer io.Writer, frame Frame) (err error) {
	var rendered image.Image
	if rendered, err = RenderFrame(frame); err != nil {
		return
	}
	return png.Encode(writer, rendered)
}

// loadMapResources loads the replay's map image, bounds, and selected font.
func loadMapResources(options PrepareOptions, replay *replayparser.Replay) (resources mapResources, err error) {
	resources.name = filepath.Base(strings.TrimRight(replay.Metadata.MapName, "/\\"))
	if !filepath.IsLocal(resources.name) || resources.name == "." || resources.name == ".." {
		err = fmt.Errorf("invalid replay map name %q", replay.Metadata.MapName)
		return
	}

	var mapPath string = filepath.Join(options.SpacesPath, resources.name)
	if resources.image, err = loadMap(mapPath); err != nil {
		return
	}
	if resources.bounds, err = LoadWorldBounds(filepath.Join(mapPath, "space.settings")); err != nil {
		return
	}

	var fontData []byte = goregular.TTF
	if options.FontPath != "" {
		if fontData, err = os.ReadFile(options.FontPath); err != nil {
			return
		}
	}
	var fontSource *text.FontSource
	if fontSource, err = text.NewFontSource(fontData); err != nil {
		return
	}
	resources.font = fontSource.Face(13)
	var boldFontSource *text.FontSource
	if boldFontSource, err = text.NewFontSource(gobold.TTF); err != nil {
		return
	}
	resources.boldFont = boldFontSource.Face(18)
	return
}

// shipSpecies reads the class from GameParams or from the class code in older ship IDs.
func shipSpecies(vehicle replaydata.Vehicle) (species string) {
	if vehicle.Species != "" {
		return vehicle.Species
	}
	if len(vehicle.ShipID) <= 3 {
		return
	}
	switch vehicle.ShipID[3] {
	case 'A':
		species = "AirCarrier"
	case 'B':
		species = "Battleship"
	case 'C':
		species = "Cruiser"
	case 'D':
		species = "Destroyer"
	case 'S':
		species = "Submarine"
	}
	return
}

// shipClassOrder places carriers first and submarines last in the lineup.
func shipClassOrder(species string) (order int) {
	switch strings.ToLower(species) {
	case "aircarrier", "carrier", "aircraftcarrier":
		order = 0
	case "battleship":
		order = 1
	case "cruiser":
		order = 2
	case "destroyer":
		order = 3
	case "submarine":
		order = 4
	default:
		order = 5
	}
	return
}

// orderTeamsByOwnTeam puts the replay player's side first while retaining a missing-identity fallback.
func orderTeamsByOwnTeam(teamIDs []uint64, players map[uint64]events.BattlePlayerResult, ownAccountID uint64) (ordered []uint64, ownTeamID uint64) {
	ordered = append(ordered, teamIDs...)
	if len(ordered) == 0 {
		return
	}
	ownTeamID = ordered[0]
	for _, player := range players {
		if player.AccountDBID == ownAccountID {
			ownTeamID = player.TeamID
			break
		}
	}
	if len(ordered) == 2 && ordered[0] != ownTeamID {
		ordered[0], ordered[1] = ordered[1], ordered[0]
	}
	return
}

// loadShipClassIcon rasterizes a unified SVG minimap marker for the requested class/state.
func loadShipClassIcon(gameDataPath string, spacesPath string, species string, state string) (icon image.Image, err error) {
	var class string
	switch strings.ToLower(species) {
	case "aircarrier", "aircraftcarrier", "carrier":
		class = "aircarrier"
	case "battleship":
		class = "battleship"
	case "cruiser":
		class = "cruiser"
	case "destroyer":
		class = "destroyer"
	case "submarine":
		class = "submarine"
	default:
		return
	}
	for _, iconPath := range []string{
		filepath.Join(gameDataPath, "ships_classes", class+state+".svg"),
		filepath.Join(filepath.Dir(spacesPath), "gui", "fla", "minimap", "ship_icons", "minimap_"+class+state+".svg"),
	} {
		var file *os.File
		if file, err = os.Open(iconPath); err != nil { // #nosec G304 -- class/state are fixed allowlisted path components.
			if os.IsNotExist(err) {
				err = nil
				continue
			}
			return
		}
		var svgData []byte
		svgData, err = io.ReadAll(file)
		err = errors.Join(err, file.Close())
		if err != nil {
			return
		}
		var document *svg.Document
		if document, err = svg.Parse(svgData); err != nil {
			return
		}
		var (
			targetScale  float64     = math.Min(26/document.ViewBox.Width, 32/document.ViewBox.Height)
			targetWidth  float64     = document.ViewBox.Width * targetScale
			targetHeight float64     = document.ViewBox.Height * targetScale
			canvas       *gg.Context = gg.NewContext(36, 36)
		)
		document.RenderTo(canvas, (36-targetWidth)/2, (36-targetHeight)/2, targetWidth, targetHeight)
		icon = canvas.Image()
		return
	}
	return
}

// loadKillIcon loads the game's fragment counter marker from the package or unpacked install.
func loadKillIcon(gameDataPath string, spacesPath string) (icon image.Image, err error) {
	for _, iconPath := range []string{
		filepath.Join(gameDataPath, "gui", "battle_hud", "icon_frag", "frags.png"),
		filepath.Join(filepath.Dir(spacesPath), "gui", "battle_hud", "icon_frag", "frags.png"),
	} {
		var file *os.File
		if file, err = os.Open(iconPath); err != nil { // #nosec G304 -- the path uses fixed icon names beneath selected game asset roots.
			if os.IsNotExist(err) {
				err = nil
				continue
			}
			return
		}
		icon, _, err = image.Decode(file)
		err = errors.Join(err, file.Close())
		return
	}
	return
}

// tintShipClassIcon team-tints a minimap marker while preserving its shading.
func tintShipClassIcon(icon image.Image, tint color.RGBA) (tinted image.Image) {
	var (
		bounds    image.Rectangle = icon.Bounds()
		imageData *image.RGBA     = image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var red, green, blue, alpha uint32 = icon.At(x, y).RGBA()
			if alpha > 0 {
				var luminance uint32 = (red + green + blue) / 3
				var shade uint32 = 64 + luminance*191/65535
				imageData.SetRGBA(x-bounds.Min.X, y-bounds.Min.Y, color.RGBA{
					R: uint8(uint32(tint.R) * shade / 255), G: uint8(uint32(tint.G) * shade / 255),
					B: uint8(uint32(tint.B) * shade / 255), A: uint8(alpha >> 8),
				})
			}
		}
	}
	return imageData
}

// loadMap composites the game's water and transparent terrain minimaps.
func loadMap(directory string) (mapImage image.Image, err error) {
	var water image.Image
	if water, err = loadImage(filepath.Join(directory, "minimap_water.png")); err != nil {
		return
	}
	var land image.Image
	if land, err = loadImage(filepath.Join(directory, "minimap.png")); err != nil {
		return
	}
	if water.Bounds().Size() != land.Bounds().Size() {
		err = fmt.Errorf("water and terrain minimaps have different dimensions")
		return
	}
	var composite *image.RGBA = image.NewRGBA(water.Bounds())
	draw.Draw(composite, composite.Bounds(), water, water.Bounds().Min, draw.Src)
	draw.Draw(composite, composite.Bounds(), land, land.Bounds().Min, draw.Over)
	return composite, nil
}

// loadImage decodes one game minimap image.
func loadImage(path string) (loaded image.Image, err error) {
	var file *os.File
	if file, err = os.Open(path); err != nil { // #nosec G304 -- caller constructs paths below validated game-data roots.
		return
	}
	defer file.Close()
	loaded, _, err = image.Decode(file)
	return
}

// containsTeam reports whether a team identifier is already present.
func containsTeam(teamIDs []uint64, teamID uint64) (contains bool) {
	for _, existing := range teamIDs {
		if existing == teamID {
			contains = true
			return
		}
	}
	return
}
