package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"github.com/alexflint/go-arg"
	"github.com/z46-dev/golog"
	wowsextractor "github.com/z46-dev/wows-extractor"
	replayparser "github.com/z46-dev/wows-replay-parser"
)

type (
	inspectCommand struct {
		Replay string `arg:"positional,required" help:"Replay file"`
		JSON   bool   `arg:"--json,-j" help:"Print metadata as JSON"`
	}

	packetsCommand struct {
		Replay   string `arg:"positional,required" help:"Replay file"`
		GameData string `arg:"--gamedata,-g" help:"Opened extractor archive to verify against the replay"`
		JSON     bool   `arg:"--json,-j" help:"Print packet counts as JSON"`
	}

	arguments struct {
		Inspect *inspectCommand `arg:"subcommand:inspect"`
		Packets *packetsCommand `arg:"subcommand:packets"`
	}

	packetSummary struct {
		Metadata replayparser.Metadata `json:"metadata"`
		Total    int                   `json:"total"`
		ByType   map[string]int        `json:"byType"`
	}
)

// Helpful error reporting function that prints to stderr and exits with a non-zero status code.
func errorf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

// main parses the command and reports a single actionable error.
func main() {
	var (
		args   arguments
		parser *arg.Parser
		err    error
	)

	if parser = arg.MustParse(&args); parser.Subcommand() == nil {
		parser.Fail("missing subcommand")
	}

	if err = run(args, os.Stdout); err != nil {
		errorf("%v", err)
	}
}

func run(arguments arguments, output io.Writer) (err error) {
	switch {
	case arguments.Inspect != nil:
		err = runInspect(arguments, output)
	case arguments.Packets != nil:
		err = runPackets(arguments, output)
	default:
		err = fmt.Errorf("no subcommand provided")
	}

	return
}

func runInspect(args arguments, output io.Writer) (err error) {
	var (
		metadata replayparser.Metadata
	)

	if metadata, err = replayparser.ReadMetadata(args.Inspect.Replay); err != nil {
		err = fmt.Errorf("failed to read metadata: %v", err)
		return
	}

	if args.Inspect.JSON {
		err = json.NewEncoder(output).Encode(metadata)
	} else {
		_, err = fmt.Fprintf(output, "%s | %s | %s | %s\n", metadata.ClientVersionFromExe, metadata.DateTime, metadata.MapDisplayName, metadata.PlayerName)
	}

	return
}

func runPackets(args arguments, output io.Writer) (err error) {
	var (
		replay  *replayparser.Replay
		archive *wowsextractor.Archive
		data    *replayparser.GameData
	)

	if replay, err = loadReplay(args.Packets.Replay, !args.Packets.JSON); err != nil {
		err = fmt.Errorf("failed to load replay: %v", err)
		return
	}

	if args.Packets.GameData != "" {
		if archive, err = wowsextractor.Load(args.Packets.GameData); err != nil {
			err = fmt.Errorf("failed to load game data: %v", err)
			return
		}

		defer archive.Close()
		if data, err = replayparser.NewGameData(archive); err != nil {
			err = fmt.Errorf("failed to create game data: %v", err)
			return
		}

		if err = data.ValidateVersion(replay); err != nil {
			err = fmt.Errorf("game data version mismatch: %v", err)
			return
		}
	}

	var summary packetSummary = packetSummary{Metadata: replay.Metadata, ByType: make(map[string]int)}
	if err = replay.WalkPackets(context.Background(), func(packet replayparser.Packet) (err error) {
		summary.Total++
		summary.ByType[packet.Type]++
		return
	}); err != nil {
		err = fmt.Errorf("failed to walk packets: %v", err)
		return
	}

	if args.Packets.JSON {
		if err = json.NewEncoder(output).Encode(summary); err != nil {
			err = fmt.Errorf("failed to encode JSON: %v", err)
			return
		}

		return
	}

	if _, err = fmt.Fprintf(output, "%s: %d packets\n", summary.Metadata.ClientVersionFromExe, summary.Total); err != nil {
		return
	}

	var names []string = make([]string, 0, len(summary.ByType))
	for name := range summary.ByType {
		names = append(names, name)
	}

	sort.Strings(names)
	for _, name := range names {
		if _, err = fmt.Fprintf(output, "%-26s %d\n", name, summary.ByType[name]); err != nil {
			return
		}
	}

	return
}

// loadReplay shows a terminal spinner while reading large replay files.
func loadReplay(path string, progress bool) (replay *replayparser.Replay, err error) {
	var spinner *golog.Spinner
	if progress {
		var info os.FileInfo
		if info, err = os.Stdout.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			spinner = golog.New().Spinner("Reading replay...", golog.SpinnerRunner, 8).Start()
		}
	}

	if spinner != nil {
		defer spinner.Stop()
	}

	replay, err = replayparser.Open(path)
	return
}
