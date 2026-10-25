package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"

	"github.com/alexflint/go-arg"
	replayparser "github.com/z46-dev/wows-replay-parser"
	"github.com/z46-dev/wows-replay-parser/video"
)

const maxReplayFileSize int64 = 256 << 20

type arguments struct {
	ReplayPath         string  `arg:"positional,required" placeholder:"REPLAY" help:"Path to a .wowsreplay file"`
	EventsJSON         bool    `arg:"--events-json" help:"Write decoded events as JSON instead of the replay summary"`
	GameData           string  `arg:"--gamedata" placeholder:"PATH" help:"Extracted replay-data package matching this replay's client version"`
	ExportFrame        string  `arg:"--export-frame" placeholder:"PATH" help:"Export one rendered PNG frame instead of printing the replay summary"`
	ExportVideo        string  `arg:"--export-video" placeholder:"PATH" help:"Export a headless MP4 replay video using FFmpeg"`
	SpacesPath         string  `arg:"--spaces" placeholder:"PATH" help:"Unpacked game spaces directory (required for frame and video exports)"`
	FontPath           string  `arg:"--font" placeholder:"PATH" help:"Optional TTF/OTF font file for frame export"`
	At                 float64 `arg:"--at" placeholder:"SECONDS" default:"0" help:"Replay clock time to render with --export-frame"`
	VideoFPS           int     `arg:"--video-fps" default:"30" help:"Video frame rate (1-60)"`
	VideoSpeed         float64 `arg:"--video-speed" default:"20" help:"Replay speed multiplier for video export"`
	VideoCRF           int     `arg:"--video-crf" default:"20" help:"H.264 quality (16-35; lower is larger and sharper)"`
	VideoScale         float64 `arg:"--video-scale" default:"1" help:"Scale the complete video layout to control output quality (0.5-2.7)"`
	VideoWorkers       int     `arg:"--video-workers" default:"0" help:"Parallel frame-render workers (0=automatic; otherwise no arbitrary limit)"`
	FFmpegPath         string  `arg:"--ffmpeg" default:"ffmpeg" placeholder:"PATH" help:"FFmpeg executable used for video export"`
	AllowVersionMixing bool    `arg:"--allow-version-mixing" help:"Use replay data from another client version; resolved member names may be incorrect"`
}

// Description explains the replay inspection command.
func (arguments) Description() string {
	return "Parse a World of Warships replay and print the information currently supported."
}

// main parses command arguments and reports errors with a nonzero exit status.
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

// inspectReplay parses a replay and dispatches the requested summary, JSON, frame, or video output.
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

	if args.VideoWorkers < 0 {
		err = fmt.Errorf("--video-workers must not be negative")
		return
	}

	if args.At < 0 || math.IsNaN(args.At) || math.IsInf(args.At, 0) {
		err = fmt.Errorf("--at must be finite and not negative")
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

	if int64(len(data)) > maxReplayFileSize {
		err = fmt.Errorf("replay exceeds the %d-byte safety limit", maxReplayFileSize)
		return
	}

	var replay *replayparser.Replay
	if replay, err = replayparser.Parse(data); err != nil {
		return
	}

	var schema *video.ReplaySchema
	if args.GameData != "" {
		if schema, err = video.LoadReplaySchema(args.GameData, replay.ClientVersion, args.AllowVersionMixing); err != nil {
			return
		}

		if schema.ClientVersion != replay.ClientVersion {
			fmt.Fprintf(os.Stderr, "warning: using %s replay data for %s replay; resolved member names may be incorrect\n", schema.ClientVersion, replay.ClientVersion)
		}
	}

	var timeline *video.Timeline
	if timeline, err = video.DecodeReplayWithSchema(replay, schema); err != nil {
		return
	}

	var duration int = replay.Metadata.Duration
	for _, event := range timeline.Events {
		if event.Results != nil && event.Results.Summary != nil && event.Results.Summary.Common.Duration > 0 {
			duration = int(event.Results.Summary.Common.Duration)
			break
		}
	}
	fmt.Printf("Replay: %s\nClient version: %s\nRecorded at: %s\nPlayer: %s\nShip: %s\nMap: %s (%s, ID %d)\nBattle: %s / %s / %s (mode %d)\nTeams: %d x %d players\nDuration: %d seconds (battle limit %d seconds)\nPackets: %d\n", args.ReplayPath, replay.ClientVersion, replay.Metadata.DateTime, replay.Metadata.PlayerName, replay.Metadata.PlayerVehicle, replay.Metadata.MapDisplayName, replay.Metadata.MapName, replay.Metadata.MapID, replay.Metadata.GameType, replay.Metadata.MatchGroup, replay.Metadata.Scenario, replay.Metadata.GameMode, replay.Metadata.TeamsCount, replay.Metadata.PlayersPerTeam, duration, replay.Metadata.BattleDuration, len(replay.Packets))

	if args.ExportFrame != "" {
		return exportReplayFrame(args, replay, timeline, schema)
	}

	if args.ExportVideo != "" {
		return exportReplayVideo(args, replay, timeline, schema)
	}

	if args.EventsJSON {
		err = json.NewEncoder(os.Stdout).Encode(timeline)
	}
	return
}
