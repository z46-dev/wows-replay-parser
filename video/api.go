// Package video turns decoded World of Warships replay events into rendered frames or MP4 videos.
package video

import (
	"context"
	"fmt"
	"image"
	"math"
	"sync"

	replayparser "github.com/z46-dev/wows-replay-parser"
	"github.com/z46-dev/wows-replay-parser/internal/events"
	"github.com/z46-dev/wows-replay-parser/internal/render"
	"github.com/z46-dev/wows-replay-parser/internal/replaydata"
)

type (
	// Bounds describes world coordinates covered by a minimap image.
	Bounds = render.Bounds
	// Layout reserves frame regions for the map, sidebars, and footer.
	Layout = render.Layout
	// Player contains lineup details displayed in a team sidebar.
	Player = render.Player
	// Ship is one vehicle marker at a replay time.
	Ship = render.Ship
	// CapturePoint marks an objective at its world-space position.
	CapturePoint = render.CapturePoint
	// Squadron is an aircraft minimap marker.
	Squadron = render.Squadron
	// Torpedo marks a moving projectile.
	Torpedo = render.Torpedo
	// ShellTracer marks a visible shell flight segment.
	ShellTracer = render.ShellTracer
	// ReplaySchema contains version-specific replay data used to resolve event members.
	ReplaySchema = replaydata.Schema
)

// DecodeReplay decodes replay events, optionally using extracted game data to resolve member schemas.
func DecodeReplay(replay *replayparser.Replay, gameDataPath string, allowVersionMixing bool) (timeline *Timeline, err error) {
	if replay == nil {
		err = fmt.Errorf("replay is required")
		return
	}

	var schema *replaydata.Schema
	if gameDataPath != "" {
		if schema, err = replaydata.Load(gameDataPath, replay.ClientVersion, allowVersionMixing); err != nil {
			return
		}
	}

	timeline, err = DecodeReplayWithSchema(replay, schema)
	return
}

// DecodeReplayWithSchema decodes replay events with an already-loaded game-data schema.
func DecodeReplayWithSchema(replay *replayparser.Replay, schema *ReplaySchema) (timeline *Timeline, err error) {
	if replay == nil {
		err = fmt.Errorf("replay is required")
		return
	}

	timeline, err = events.DecodeWithSchema(replay, schema)
	return
}

// RenderFrame draws one complete frame using the caller-provided map, font, and lineup assets.
func RenderFrame(frame Frame) (result image.Image, err error) {
	return render.RenderFrame(frame)
}

// RenderFrames advances replay state in order, renders concurrently, and delivers frames in order.
func RenderFrames(ctx context.Context, timeline *Timeline, baseFrame Frame, options Options, consume func(index int, clock float64, frame image.Image) error) (err error) {
	if ctx == nil || timeline == nil || consume == nil {
		err = fmt.Errorf("video context, timeline, and frame consumer are required")
		return
	}
	var frame Frame
	if frame, options, err = prepareFrame(baseFrame, options); err != nil {
		return
	}

	if frame.Duration <= 0 {
		err = fmt.Errorf("replay has no positive battle duration")
		return
	}

	var frameCount int
	if frameCount, err = videoFrameCount(frame.Duration, options.FPS, options.Speed, frame.Layout.Width, frame.Layout.Height); err != nil {
		return
	}

	var workerCount int
	if workerCount, err = renderWorkerCount(options.Workers); err != nil {
		return
	}
	workerCount = min(workerCount, frameCount)

	var (
		jobs    chan frameRenderJob    = make(chan frameRenderJob, workerCount)
		results chan frameRenderResult = make(chan frameRenderResult, workerCount)
		workers sync.WaitGroup
		state   *FrameState = NewFrameState(timeline)
	)
	for range workerCount {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for job := range jobs {
				var (
					imageFrame image.Image
					renderErr  error
				)
				imageFrame, renderErr = RenderFrame(job.frame)
				results <- frameRenderResult{index: job.index, image: imageFrame, err: renderErr}
			}
		}()
	}

	var (
		submitted int
		received  int
		nextFrame int
		pending   map[int]frameRenderResult = make(map[int]frameRenderResult, workerCount)
	)
	for nextFrame < frameCount && err == nil {
		if err = ctx.Err(); err != nil {
			break
		}
		for submitted < frameCount && submitted-nextFrame < workerCount {
			var clock float64 = math.Min(frame.Duration, float64(submitted)*options.Speed/float64(options.FPS))
			jobs <- frameRenderJob{index: submitted, frame: state.FrameAt(frame, clock)}
			submitted++
		}

		var result frameRenderResult
		if ready, exists := pending[nextFrame]; exists {
			result = ready
			delete(pending, nextFrame)
		} else {
			result = <-results
			received++
			if result.index != nextFrame {
				pending[result.index] = result
				continue
			}
		}

		if result.err != nil {
			err = fmt.Errorf("render video frame %d: %w", result.index, result.err)
			break
		}
		if err = consume(result.index, math.Min(frame.Duration, float64(result.index)*options.Speed/float64(options.FPS)), result.image); err == nil {
			nextFrame++
		}
	}

	for received < submitted {
		<-results
		received++
	}
	close(jobs)
	workers.Wait()
	return
}

// prepareFrame applies quality scaling to the caller's layout and validates sampling settings.
func prepareFrame(baseFrame Frame, options Options) (frame Frame, normalized Options, err error) {
	normalized = options
	if normalized.FPS == 0 {
		normalized.FPS = 30
	}
	if normalized.Speed == 0 {
		normalized.Speed = 20
	}
	if normalized.Scale == 0 {
		normalized.Scale = 1
	}
	if normalized.FPS < 1 || normalized.FPS > 60 || normalized.Speed <= 0 || math.IsNaN(normalized.Speed) || math.IsInf(normalized.Speed, 0) || normalized.Scale < 0.5 || normalized.Scale > 2.7 || math.IsNaN(normalized.Scale) || math.IsInf(normalized.Scale, 0) || normalized.Workers < 0 {
		err = fmt.Errorf("frame settings are out of range (fps 1-60, speed > 0, scale 0.5-2.7, workers >= 0)")
		return
	}

	frame = baseFrame
	var baseLayout Layout = frame.Layout
	if baseLayout.Width <= 0 {
		baseLayout.Width = 1328
	}
	if baseLayout.Height <= 0 {
		baseLayout.Height = 800
	}
	frame.Layout.Width = scaledEven(baseLayout.Width, normalized.Scale)
	frame.Layout.Height = scaledEven(baseLayout.Height, normalized.Scale)
	frame.Layout.LeftPanelWidth = int(math.Round(float64(baseLayout.LeftPanelWidth) * normalized.Scale))
	frame.Layout.RightPanelWidth = int(math.Round(float64(baseLayout.RightPanelWidth) * normalized.Scale))
	frame.Layout.FooterHeight = int(math.Round(float64(baseLayout.FooterHeight) * normalized.Scale))
	frame.Layout.TopBarHeight = int(math.Round(float64(baseLayout.TopBarHeight) * normalized.Scale))
	if frame.Layout.Width > 3840 || frame.Layout.Height > 2160 {
		err = fmt.Errorf("scaled layout exceeds 3840x2160 pixels")
	}
	return
}

// scaledEven rounds dimensions to even pixel sizes required by common video encoders.
func scaledEven(dimension int, scale float64) (scaled int) {
	scaled = int(math.Round(float64(dimension) * scale))
	if scaled%2 != 0 {
		scaled++
	}
	return
}
