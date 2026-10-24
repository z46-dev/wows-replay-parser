package main

import (
	"encoding/json"
	"encoding/xml"
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

	"github.com/alexflint/go-arg"
	"github.com/gogpu/gg"
	"github.com/gogpu/gg/svg"
	"github.com/gogpu/gg/text"
	replayparser "github.com/z46-dev/wows-replay-parser"
	"github.com/z46-dev/wows-replay-parser/internal/events"
	"github.com/z46-dev/wows-replay-parser/internal/render"
	"github.com/z46-dev/wows-replay-parser/internal/replaydata"
	"golang.org/x/image/font/gofont/goregular"
)

const maxReplayFileSize int64 = 256 << 20

type arguments struct {
	ReplayPath         string  `arg:"positional,required" placeholder:"REPLAY" help:"Path to a .wowsreplay file"`
	EventsJSON         bool    `arg:"--events-json" help:"Write decoded events as JSON instead of the replay summary"`
	EventType          string  `arg:"--event-type" placeholder:"TYPE" help:"Filter JSON events by type"`
	EntityID           uint32  `arg:"--entity-id" placeholder:"ID" help:"Filter JSON events by entity ID"`
	MyShip             bool    `arg:"--my-ship" help:"Filter JSON events to player-ship orientation updates by default"`
	GameData           string  `arg:"--gamedata" placeholder:"PATH" help:"Extracted replay-data package matching this replay's client version"`
	ExportFrame        string  `arg:"--export-frame" placeholder:"PATH" help:"Export one rendered PNG frame instead of printing the replay summary"`
	ExportVideo        string  `arg:"--export-video" placeholder:"PATH" help:"Export a headless MP4 replay video using FFmpeg"`
	SpacesPath         string  `arg:"--spaces" placeholder:"PATH" help:"Unpacked game spaces directory (required for frame and video exports)"`
	FontPath           string  `arg:"--font" placeholder:"PATH" help:"Optional TTF/OTF font file for frame export"`
	At                 float64 `arg:"--at" placeholder:"SECONDS" default:"0" help:"Replay clock time to render with --export-frame"`
	VideoFPS           int     `arg:"--video-fps" default:"30" help:"Video frame rate (1-60)"`
	VideoSpeed         float64 `arg:"--video-speed" default:"20" help:"Replay speed multiplier for video export"`
	VideoCRF           int     `arg:"--video-crf" default:"20" help:"H.264 quality (16-35; lower is larger and sharper)"`
	VideoWidth         int     `arg:"--video-width" default:"1328" help:"Video width in pixels (960-3840, even)"`
	VideoHeight        int     `arg:"--video-height" default:"800" help:"Video height in pixels (540-2160, even)"`
	FFmpegPath         string  `arg:"--ffmpeg" default:"ffmpeg" placeholder:"PATH" help:"FFmpeg executable used for video export"`
	AllowVersionMixing bool    `arg:"--allow-version-mixing" help:"Use replay data from another client version; resolved names may be incorrect"`
}

// Description explains the current replay inspection command.
func (arguments) Description() string {
	return "Parse a World of Warships replay and print the information currently supported."
}

// main parses arguments and reports command errors with a nonzero exit status.
func main() {
	var (
		args arguments
		err  error
	)

	arg.MustParse(&args)

	if err = inspectReplay(args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// inspectReplay parses a replay and prints its metadata and raw packet-type counts.
func inspectReplay(args arguments) (err error) {
	var outputModes int
	if args.EventsJSON {
		outputModes++
	}

	if args.ExportFrame != "" {
		outputModes++
	}

	if args.ExportVideo != "" {
		outputModes++
	}

	if outputModes > 1 {
		err = fmt.Errorf("--events-json, --export-frame, and --export-video are mutually exclusive")
		return
	}

	if args.AllowVersionMixing && ((!args.EventsJSON && args.ExportFrame == "" && args.ExportVideo == "") || args.GameData == "") {
		err = fmt.Errorf("--allow-version-mixing requires an export mode and --gamedata")
		return
	}

	if (args.ExportFrame != "" || args.ExportVideo != "") && (args.SpacesPath == "" || args.GameData == "") {
		err = fmt.Errorf("--export-frame and --export-video require --spaces and --gamedata")
		return
	}

	if args.ExportVideo != "" && (args.VideoFPS < 1 || args.VideoFPS > 60 || args.VideoSpeed <= 0 || math.IsNaN(args.VideoSpeed) || math.IsInf(args.VideoSpeed, 0) || args.VideoCRF < 16 || args.VideoCRF > 35 || args.VideoWidth < 960 || args.VideoWidth > 3840 || args.VideoWidth%2 != 0 || args.VideoHeight < 540 || args.VideoHeight > 2160 || args.VideoHeight%2 != 0) {
		err = fmt.Errorf("video settings are out of range (fps 1-60, speed > 0, CRF 16-35, width 960-3840 even, height 540-2160 even)")
		return
	}

	if args.At < 0 {
		err = fmt.Errorf("--at must not be negative")
		return
	}

	var file *os.File
	if file, err = os.Open(args.ReplayPath); err != nil {
		return
	}

	defer file.Close()

	var data []byte
	if data, err = io.ReadAll(io.LimitReader(file, maxReplayFileSize+1)); err != nil {
		return
	}

	var replay *replayparser.Replay
	if replay, err = replayparser.Parse(data); err != nil {
		return
	}

	if args.EventsJSON || args.ExportFrame != "" || args.ExportVideo != "" {
		var (
			timeline *events.Timeline
			schema   *replaydata.Schema
		)

		if args.GameData != "" {
			if schema, err = replaydata.Load(args.GameData, replay.ClientVersion, args.AllowVersionMixing); err != nil {
				return
			}

			if schema.ClientVersion != replay.ClientVersion {
				fmt.Fprintf(os.Stderr, "warning: using %s replay data for %s replay; resolved member names may be incorrect\n", schema.ClientVersion, replay.ClientVersion)
			}
		}

		if timeline, err = events.DecodeWithSchema(replay, schema); err != nil {
			return
		}

		if args.ExportFrame != "" {
			if err = exportReplayFrame(args, replay, timeline, schema); err != nil {
				return
			}

			return
		}

		if args.ExportVideo != "" {
			if err = exportReplayVideo(args, replay, timeline, schema); err != nil {
				return
			}

			return
		}

		var (
			entityID  uint32      = args.EntityID
			eventType events.Type = events.Type(args.EventType)
		)

		if args.MyShip {
			entityID = timeline.OwnShipID
			if entityID == 0 {
				err = fmt.Errorf("replay does not identify the player ship")
				return
			}

			if eventType == "" {
				eventType = events.PlayerOrientationEvent
			}
		}

		timeline.Events = events.Filter(timeline.Events, eventType, entityID)
		err = json.NewEncoder(os.Stdout).Encode(timeline)
		return
	}

	fmt.Printf("Replay: %s\nClient version: %s\nRecorded at: %s\nPlayer: %s\nShip: %s\nMap: %s (%s, ID %d)\nBattle: %s / %s / %s (mode %d)\nTeams: %d x %d players\nDuration: %d seconds (battle limit %d seconds)\nPackets: %d\n", args.ReplayPath, replay.ClientVersion, replay.Metadata.DateTime, replay.Metadata.PlayerName, replay.Metadata.PlayerVehicle, replay.Metadata.MapDisplayName, replay.Metadata.MapName, replay.Metadata.MapID, replay.Metadata.GameType, replay.Metadata.MatchGroup, replay.Metadata.Scenario, replay.Metadata.GameMode, replay.Metadata.TeamsCount, replay.Metadata.PlayersPerTeam, replay.Metadata.Duration, replay.Metadata.BattleDuration, len(replay.Packets))
	return
}

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

// exportReplayFrame renders the requested replay clock into a PNG file.
func exportReplayFrame(args arguments, replay *replayparser.Replay, timeline *events.Timeline, schema *replaydata.Schema) (err error) {
	var (
		mapName  string
		mapImage image.Image
		bounds   render.Bounds
		font     text.Face
	)

	if mapName, mapImage, bounds, font, err = loadRenderResources(args, replay); err != nil {
		return
	}

	var frame render.Frame
	if frame, err = buildFrame(args, replay, timeline, mapImage, mapName, bounds, font, schema); err != nil {
		return
	}

	if err = os.MkdirAll(filepath.Dir(args.ExportFrame), 0o755); err != nil {
		return
	}

	var output *os.File
	if output, err = os.OpenFile(args.ExportFrame, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644); err != nil {
		return
	}

	defer output.Close()
	if err = encodePNG(output, frame); err != nil {
		os.Remove(args.ExportFrame) // #nosec G104 -- cleanup must not replace the original encoding error.
		return
	}

	fmt.Printf("Rendered %.1f-second frame for %s to %s\n", args.At, mapName, args.ExportFrame)
	return
}

// loadRenderResources loads the replay map, its world bounds, and the selected font.
func loadRenderResources(args arguments, replay *replayparser.Replay) (mapName string, mapImage image.Image, bounds render.Bounds, font text.Face, err error) {
	mapName = filepath.Base(strings.TrimRight(replay.Metadata.MapName, "/\\"))
	if !filepath.IsLocal(mapName) || mapName == "." || mapName == ".." {
		err = fmt.Errorf("invalid replay map name %q", replay.Metadata.MapName)
		return
	}

	var mapPath string = filepath.Join(args.SpacesPath, mapName)
	if mapImage, err = loadMap(mapPath); err != nil {
		return
	}

	if bounds, err = loadWorldBounds(filepath.Join(mapPath, "space.settings")); err != nil {
		return
	}

	var fontData []byte = goregular.TTF
	if args.FontPath != "" {
		if fontData, err = os.ReadFile(args.FontPath); err != nil {
			return
		}
	}

	var fontSource *text.FontSource
	if fontSource, err = text.NewFontSource(fontData); err != nil {
		return
	}

	font = fontSource.Face(13)
	return
}

// buildFrame combines decoded results and vehicle positions into render content.
func buildFrame(args arguments, replay *replayparser.Replay, timeline *events.Timeline, mapImage image.Image, mapName string, bounds render.Bounds, font text.Face, schema *replaydata.Schema) (frame render.Frame, err error) {
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

	sort.Slice(teamIDs, func(left int, right int) bool {
		return teamIDs[left] < teamIDs[right]
	})

	if len(teamIDs) < 1 || len(teamIDs) > 2 {
		err = fmt.Errorf("expected one or two teams in battle results, found %d", len(teamIDs))
		return
	}

	teamIDs, _ = orderTeamsByOwnTeam(teamIDs, summary.Players, summary.AccountDBID)

	var (
		ownTeamID uint64 = teamIDs[0]
		leftTeam  []render.Player
		rightTeam []render.Player
		playerIDs []uint64
	)

	for accountID := range summary.Players {
		playerIDs = append(playerIDs, accountID)
	}

	sort.Slice(playerIDs, func(left int, right int) bool {
		return playerIDs[left] < playerIDs[right]
	})

	for _, accountID := range playerIDs {
		var (
			player                                          events.BattlePlayerResult = summary.Players[accountID]
			icon, classIcon, classIconDead, classIconHidden image.Image
			iconColor                                       color.RGBA = color.RGBA{R: 64, G: 224, B: 151, A: 255}
		)

		if player.TeamID != ownTeamID {
			iconColor = color.RGBA{R: 244, G: 67, B: 74, A: 255}
		}

		if player.AccountDBID == summary.AccountDBID {
			iconColor = color.RGBA{R: 235, G: 239, B: 244, A: 255}
		}

		if icon, err = loadShipIcon(args.GameData, args.SpacesPath, player.ShipID); err != nil {
			return
		}

		if icon != nil {
			icon = tintShipIcon(icon, iconColor)
		}

		if schema != nil {
			if vehicle, ok := schema.Vehicle(player.VehicleTypeID); ok {
				if classIcon, err = loadShipClassIcon(args.GameData, args.SpacesPath, shipSpecies(vehicle), ""); err != nil {
					return
				}

				if classIconDead, err = loadShipClassIcon(args.GameData, args.SpacesPath, shipSpecies(vehicle), "_dead"); err != nil {
					return
				}

				if classIconHidden, err = loadShipClassIcon(args.GameData, args.SpacesPath, shipSpecies(vehicle), "_last_visible"); err != nil {
					return
				}

				if classIcon != nil {
					classIcon = tintShipClassIcon(classIcon, iconColor)
				}
			}
		}

		var renderedPlayer render.Player = render.Player{Name: player.Name, ClanTag: player.ClanTag, ShipName: player.ShipName, Icon: icon, ClassIcon: classIcon, ClassIconDead: classIconDead, ClassIconUnspotted: classIconHidden, Self: player.AccountDBID == summary.AccountDBID, Health: player.RemainingHealth, MaxHealth: player.MaxHealth, Damage: player.Damage, Kills: player.ShipsKilled, Alive: player.IsAlive, TeamID: player.TeamID}
		if player.TeamID == ownTeamID {
			leftTeam = append(leftTeam, renderedPlayer)
		} else if len(teamIDs) == 2 && player.TeamID == teamIDs[1] {
			rightTeam = append(rightTeam, renderedPlayer)
		}
	}

	render.SortPlayers(leftTeam)
	render.SortPlayers(rightTeam)

	var (
		replayState    *replayFrameState     = newReplayFrameState(timeline)
		shipPositions  []render.Ship         = replayState.ShipsAt(args.At)
		squadrons      []render.Squadron     = replayState.SquadronsAt()
		torpedoes      []render.Torpedo      = replayState.TorpedoesAt(args.At)
		capturePoints  []render.CapturePoint = replayState.CapturePoints()
		allyPlaneIcon  image.Image
		enemyPlaneIcon image.Image
	)

	bindShipLineup(shipPositions, leftTeam)
	bindShipLineup(shipPositions, rightTeam)
	if allyPlaneIcon, err = loadSquadronIcon(args.GameData, args.SpacesPath, "ally"); err != nil {
		return
	}

	if enemyPlaneIcon, err = loadSquadronIcon(args.GameData, args.SpacesPath, "enemy"); err != nil {
		return
	}

	var duration float64 = float64(replay.Metadata.Duration)
	if summary.Common.Duration > 0 {
		duration = float64(summary.Common.Duration)
	}

	var (
		leftTeamName  string = "OUR TEAM"
		rightTeamName string = "ENEMY TEAM"
	)

	frame = render.Frame{Layout: render.Layout{Width: 1328, Height: 800, LeftPanelWidth: 280, RightPanelWidth: 280, FooterHeight: 48}, Map: mapImage, MapName: mapName, Clock: args.At, Duration: duration, WorldBounds: bounds, LeftTeamName: leftTeamName, RightTeamName: rightTeamName, LeftTeam: leftTeam, RightTeam: rightTeam, Ships: shipPositions, Squadrons: squadrons, Torpedoes: torpedoes, CapturePoints: capturePoints, AllySquadronIcon: allyPlaneIcon, EnemySquadronIcon: enemyPlaneIcon, FooterText: "Self: white • allies: green • enemies: red • outlines: last known • capture rings: ownership and progress", Font: font}
	return
}

// shipSpecies reads the class from GameParams, or from the class code in older ship IDs.
func shipSpecies(vehicle replaydata.Vehicle) (species string) {
	if vehicle.Species != "" {
		return vehicle.Species
	}

	if len(vehicle.ShipID) <= 3 {
		return
	}

	// shortcut: use the fourth ship-ID character for older packages; replace if WG changes that encoding.
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

// orderTeamsByOwnTeam puts the replay player's side first while keeping a fallback for missing player identity.
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

// loadSquadronIcon loads and team-tints the game's aircraft silhouette from the package or install.
func loadSquadronIcon(gameDataPath string, spacesPath string, side string) (icon image.Image, err error) {
	var tint color.RGBA
	switch side {
	case "ally":
		tint = color.RGBA{R: 64, G: 224, B: 151, A: 255}
	case "enemy":
		tint = color.RGBA{R: 244, G: 67, B: 74, A: 255}
	default:
		err = fmt.Errorf("invalid squadron icon side %q", side)
		return
	}

	for _, iconPath := range []string{
		filepath.Join(gameDataPath, "squadrons", "aircraft.png"),
		filepath.Join(filepath.Dir(spacesPath), "gui", "grand_strategy_pass", "pawns", "aircraft.png"),
	} {
		var file *os.File
		if file, err = os.Open(iconPath); err != nil { // #nosec G304 -- path is under the user-selected game-data roots.
			if os.IsNotExist(err) {
				err = nil
				continue
			}

			return
		}

		defer file.Close()

		if icon, _, err = image.Decode(file); err == nil {
			icon = tintShipIcon(icon, tint)
		}

		return
	}

	return
}

// loadShipIcon loads the game's ship silhouette when it is present beside the unpacked map assets.
func loadShipIcon(gameDataPath string, spacesPath string, shipID string) (icon image.Image, err error) {
	if shipID == "" || filepath.Base(shipID) != shipID {
		return
	}

	for _, iconPath := range []string{
		filepath.Join(gameDataPath, "ships_silhouettes", shipID+".png"),
		filepath.Join(filepath.Dir(spacesPath), "gui", "ships_silhouettes", shipID+".png"),
	} {
		var file *os.File
		if file, err = os.Open(iconPath); err != nil { // #nosec G304 -- shipID is a basename under selected game-data roots.
			if os.IsNotExist(err) {
				err = nil
				continue
			}

			return
		}

		defer file.Close()
		icon, _, err = image.Decode(file)
		return
	}

	return
}

// loadShipClassIcon rasterizes the game's unified SVG minimap marker for a ship class.
func loadShipClassIcon(gameDataPath string, spacesPath string, species string, state string) (icon image.Image, err error) {
	var class string
	switch strings.ToLower(species) {
	case "aircarrier":
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
		if file, err = os.Open(iconPath); err != nil { // #nosec G304 -- class/state are fixed allowlisted values under selected roots.
			if os.IsNotExist(err) {
				err = nil
				continue
			}

			return
		}

		defer file.Close()

		var svgData []byte
		if svgData, err = io.ReadAll(file); err != nil {
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

// tintShipIcon applies the team color while retaining the ship silhouette alpha channel.
func tintShipIcon(icon image.Image, tint color.RGBA) (tinted image.Image) {
	var (
		bounds    image.Rectangle = icon.Bounds()
		imageData *image.RGBA     = image.NewRGBA(image.Rect(0, 0, bounds.Dx(), bounds.Dy()))
	)

	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			var alpha uint32
			if _, _, _, alpha = icon.At(x, y).RGBA(); alpha > 0 {
				imageData.SetRGBA(x-bounds.Min.X, y-bounds.Min.Y, color.RGBA{R: tint.R, G: tint.G, B: tint.B, A: uint8(alpha >> 8)})
			}
		}
	}

	tinted = imageData
	return
}

// tintShipClassIcon team-tints a minimap marker while preserving its dark-and-light outline details.
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
					R: uint8(uint32(tint.R) * shade / 255),
					G: uint8(uint32(tint.G) * shade / 255),
					B: uint8(uint32(tint.B) * shade / 255),
					A: uint8(alpha >> 8),
				})
			}
		}
	}

	tinted = imageData
	return
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
	mapImage = composite
	return
}

// loadImage decodes one game minimap image.
func loadImage(path string) (loaded image.Image, err error) {
	var file *os.File
	if file, err = os.Open(path); err != nil { // #nosec G304 -- caller builds this below the validated game map directory.
		return
	}

	defer file.Close()
	loaded, _, err = image.Decode(file)
	return
}

// loadWorldBounds converts space settings bounds to game coordinates.
func loadWorldBounds(path string) (bounds render.Bounds, err error) {
	var data []byte
	if data, err = os.ReadFile(path); err != nil { // #nosec G304 -- caller builds this below the validated game map directory.
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

	bounds = render.Bounds{MinX: -spaceSize / 2, MaxX: spaceSize / 2, MinZ: -spaceSize / 2, MaxZ: spaceSize / 2}
	if bounds.MaxX <= bounds.MinX || bounds.MaxZ <= bounds.MinZ {
		err = fmt.Errorf("space settings contain invalid minimap bounds")
	}

	return
}

// containsTeam reports whether the team identifier is already present.
func containsTeam(teamIDs []uint64, teamID uint64) (contains bool) {
	for _, existing := range teamIDs {
		if existing == teamID {
			contains = true
			return
		}
	}

	return
}

// encodePNG renders and encodes a frame as PNG.
func encodePNG(writer io.Writer, frame render.Frame) (err error) {
	var rendered image.Image
	if rendered, err = render.RenderFrame(frame); err != nil {
		return
	}

	err = png.Encode(writer, rendered)
	return
}
