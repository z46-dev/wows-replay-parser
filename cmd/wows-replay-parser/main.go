package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"image"
	"image/draw"
	"image/png"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/alexflint/go-arg"
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
	SpacesPath         string  `arg:"--spaces" placeholder:"PATH" help:"Unpacked game spaces directory (required with --export-frame)"`
	FontPath           string  `arg:"--font" placeholder:"PATH" help:"Optional TTF/OTF font file for frame export"`
	At                 float64 `arg:"--at" placeholder:"SECONDS" default:"0" help:"Replay clock time to render with --export-frame"`
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
	if args.EventsJSON && args.ExportFrame != "" {
		err = fmt.Errorf("--events-json and --export-frame cannot be used together")
		return
	}
	if args.AllowVersionMixing && ((!args.EventsJSON && args.ExportFrame == "") || args.GameData == "") {
		err = fmt.Errorf("--allow-version-mixing requires --events-json or --export-frame and --gamedata")
		return
	}
	if args.ExportFrame != "" && (args.SpacesPath == "" || args.GameData == "") {
		err = fmt.Errorf("--export-frame requires --spaces and --gamedata")
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

	if args.EventsJSON || args.ExportFrame != "" {
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
			if err = exportReplayFrame(args, replay, timeline); err != nil {
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
		MinX float64 `xml:"minX,attr"`
		MaxX float64 `xml:"maxX,attr"`
		MinY float64 `xml:"minY,attr"`
		MaxY float64 `xml:"maxY,attr"`
	} `xml:"bounds"`
}

// exportReplayFrame renders the requested replay clock into a PNG file.
func exportReplayFrame(args arguments, replay *replayparser.Replay, timeline *events.Timeline) (err error) {
	var (
		mapName  string = filepath.Base(strings.TrimRight(replay.Metadata.MapName, "/\\"))
		mapPath  string = filepath.Join(args.SpacesPath, mapName)
		mapImage image.Image
		bounds   render.Bounds
	)
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

	var frame render.Frame
	if frame, err = buildFrame(args, replay, timeline, mapImage, mapName, bounds, fontSource.Face(13)); err != nil {
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
		os.Remove(args.ExportFrame)
		return
	}
	fmt.Printf("Rendered %.1f-second frame for %s to %s\n", args.At, mapName, args.ExportFrame)
	return
}

// buildFrame combines decoded results and vehicle positions into render content.
func buildFrame(args arguments, replay *replayparser.Replay, timeline *events.Timeline, mapImage image.Image, mapName string, bounds render.Bounds, font text.Face) (frame render.Frame, err error) {
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
	if len(teamIDs) != 2 {
		err = fmt.Errorf("expected two teams in battle results, found %d", len(teamIDs))
		return
	}
	var (
		leftTeam  []render.Player
		rightTeam []render.Player
		playerIDs []uint64
	)
	for accountID := range summary.Players {
		playerIDs = append(playerIDs, accountID)
	}
	sort.Slice(playerIDs, func(left int, right int) bool { return playerIDs[left] < playerIDs[right] })
	for _, accountID := range playerIDs {
		var player events.BattlePlayerResult = summary.Players[accountID]
		var renderedPlayer render.Player = render.Player{Name: player.Name, ClanTag: player.ClanTag, ShipName: player.ShipName, Health: player.RemainingHealth, MaxHealth: player.MaxHealth, Damage: player.Damage, Alive: player.IsAlive}
		if player.TeamID == teamIDs[0] {
			leftTeam = append(leftTeam, renderedPlayer)
		} else if player.TeamID == teamIDs[1] {
			rightTeam = append(rightTeam, renderedPlayer)
		}
	}
	render.SortPlayers(leftTeam)
	render.SortPlayers(rightTeam)
	var ships map[uint32]render.Ship = make(map[uint32]render.Ship)
	for _, event := range timeline.Events {
		if float64(event.Clock) > args.At {
			continue
		}
		switch event.Type {
		case events.EntityCreateEvent:
			if event.Created == nil || event.Created.EntityType != "Vehicle" || event.Created.Position == nil {
				continue
			}
			ships[event.EntityID] = render.Ship{X: float64(event.Created.Position.X), Z: float64(event.Created.Position.Z), Heading: float64(event.Created.Position.Yaw)}
		case events.PositionEvent, events.NonVolatilePositionEvent:
			if event.Position == nil {
				continue
			}
			if ship, exists := ships[event.EntityID]; exists {
				ship.X = float64(event.Position.X)
				ship.Z = float64(event.Position.Z)
				ship.Heading = float64(event.Position.Yaw)
				ships[event.EntityID] = ship
			}
		case events.EntityLeaveEvent:
			delete(ships, event.EntityID)
		}
	}
	var shipIDs []uint32
	for entityID := range ships {
		shipIDs = append(shipIDs, entityID)
	}
	sort.Slice(shipIDs, func(left int, right int) bool { return shipIDs[left] < shipIDs[right] })
	var shipPositions []render.Ship
	for _, entityID := range shipIDs {
		shipPositions = append(shipPositions, ships[entityID])
	}
	var duration float64 = float64(replay.Metadata.Duration)
	if summary.Common.Duration > 0 {
		duration = float64(summary.Common.Duration)
	}
	frame = render.Frame{Layout: render.Layout{Width: 1328, Height: 800, LeftPanelWidth: 280, RightPanelWidth: 280, FooterHeight: 48}, Map: mapImage, MapName: mapName, Clock: args.At, Duration: duration, WorldBounds: bounds, LeftTeamName: fmt.Sprintf("TEAM %d", teamIDs[0]), RightTeamName: fmt.Sprintf("TEAM %d", teamIDs[1]), LeftTeam: leftTeam, RightTeam: rightTeam, Ships: shipPositions, FooterText: fmt.Sprintf("Lineups from post-battle results • live ship stats are shown only at battle end • %d map markers", len(shipPositions)), Font: font, ShowResultsStats: args.At >= duration}
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
	if file, err = os.Open(path); err != nil {
		return
	}
	defer file.Close()
	loaded, _, err = image.Decode(file)
	return
}

// loadWorldBounds converts space settings bounds to game coordinates.
func loadWorldBounds(path string) (bounds render.Bounds, err error) {
	var data []byte
	if data, err = os.ReadFile(path); err != nil {
		return
	}
	var settings spaceSettings
	if err = xml.Unmarshal(data, &settings); err != nil {
		return
	}
	bounds = render.Bounds{MinX: settings.Bounds.MinX * 100, MaxX: settings.Bounds.MaxX * 100, MinZ: settings.Bounds.MinY * 100, MaxZ: settings.Bounds.MaxY * 100}
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
