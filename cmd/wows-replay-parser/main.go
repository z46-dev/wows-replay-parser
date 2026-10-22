package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/alexflint/go-arg"
	replayparser "github.com/z46-dev/wows-replay-parser"
	"github.com/z46-dev/wows-replay-parser/events"
)

const maxReplayFileSize int64 = 256 << 20

type arguments struct {
	ReplayPath string `arg:"positional,required" placeholder:"REPLAY" help:"Path to a .wowsreplay file"`
	EventsJSON bool   `arg:"--events-json" help:"Write decoded events as JSON instead of the replay summary"`
	EventType  string `arg:"--event-type" placeholder:"TYPE" help:"Filter JSON events by type"`
	EntityID   uint32 `arg:"--entity-id" placeholder:"ID" help:"Filter JSON events by entity ID"`
	MyShip     bool   `arg:"--my-ship" help:"Filter JSON events to player-ship orientation updates by default"`
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

	if args.EventsJSON {
		var timeline *events.Timeline
		if timeline, err = events.Decode(replay); err != nil {
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
