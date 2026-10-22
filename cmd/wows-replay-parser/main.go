package main

import (
	"fmt"
	"io"
	"os"
	"slices"

	"github.com/alexflint/go-arg"
	replayparser "github.com/z46-dev/wows-replay-parser"
)

const maxReplayFileSize int64 = 256 << 20

type arguments struct {
	ReplayPath string `arg:"positional,required" placeholder:"REPLAY" help:"Path to a .wowsreplay file"`
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

	if err = inspectReplay(args.ReplayPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

// inspectReplay parses a replay and prints its metadata and raw packet-type counts.
func inspectReplay(path string) (err error) {
	var file *os.File
	if file, err = os.Open(path); err != nil {
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

	var packetCounts map[uint32]int = make(map[uint32]int)
	for _, packet := range replay.Packets {
		packetCounts[packet.Type]++
	}

	var packetTypes []uint32 = make([]uint32, 0, len(packetCounts))
	for packetType := range packetCounts {
		packetTypes = append(packetTypes, packetType)
	}

	slices.Sort(packetTypes)

	fmt.Printf("Replay: %s\nClient version: %s\nPlayer: %s\nMap: %s\nPackets: %d\n", path, replay.ClientVersion, replay.Metadata.PlayerName, replay.Metadata.MapName, len(replay.Packets))
	fmt.Println("Raw packet type counts:")
	for _, packetType := range packetTypes {
		fmt.Printf("  %d: %d\n", packetType, packetCounts[packetType])
	}

	return
}
