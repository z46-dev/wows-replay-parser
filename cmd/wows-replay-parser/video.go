package main

import (
	"fmt"
	"os"
	"path/filepath"

	replayparser "github.com/z46-dev/wows-replay-parser"
	"github.com/z46-dev/wows-replay-parser/video"
)

// exportReplayFrame prepares the requested moment and writes a PNG to the selected destination.
func exportReplayFrame(args arguments, replay *replayparser.Replay, timeline *video.Timeline, schema *video.ReplaySchema) (err error) {
	var (
		frame   video.Frame
		mapName string
	)
	if frame, mapName, err = video.PrepareFrame(replay, timeline, schema, video.PrepareOptions{
		GameDataPath: args.GameData,
		FontPath:     args.FontPath,
		Clock:        args.At,
	}); err != nil {
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
	if err = video.EncodePNG(output, frame); err != nil {
		os.Remove(args.ExportFrame) // #nosec G104 -- preserve the original encoding error.
		return
	}
	fmt.Printf("Rendered %.1f-second frame for %s to %s\n", args.At, mapName, args.ExportFrame)
	return
}

// exportReplayVideo adapts command options and local assets to the reusable video library.
func exportReplayVideo(args arguments, replay *replayparser.Replay, timeline *video.Timeline, schema *video.ReplaySchema) (err error) {
	var frame video.Frame
	if frame, _, err = video.PrepareFrame(replay, timeline, schema, video.PrepareOptions{
		GameDataPath: args.GameData,
		FontPath:     args.FontPath,
	}); err != nil {
		return
	}

	if err = video.ExportMP4(timeline, frame, video.Options{
		OutputPath: args.ExportVideo,
		FFmpegPath: args.FFmpegPath,
		FPS:        args.VideoFPS,
		Speed:      args.VideoSpeed,
		CRF:        args.VideoCRF,
		Scale:      args.VideoScale,
		Workers:    args.VideoWorkers,
	}); err != nil {
		return
	}

	fmt.Printf("Rendered %.1f seconds of replay at %.1fx to %s\n", frame.Duration, args.VideoSpeed, args.ExportVideo)
	return
}
