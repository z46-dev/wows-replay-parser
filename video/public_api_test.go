package video_test

import (
	"context"
	"image"
	"testing"

	"github.com/z46-dev/wows-replay-parser/video"
)

// TestPublicFrameAPIIsUsableOutsideThePackage exercises the public aliases and frame callback API.
func TestPublicFrameAPIIsUsableOutsideThePackage(t *testing.T) {
	var (
		timeline *video.Timeline = &video.Timeline{}
		frame    video.Frame     = video.Frame{
			Map:         image.NewRGBA(image.Rect(0, 0, 2, 2)),
			Duration:    0.1,
			WorldBounds: video.Bounds{MinX: -1, MaxX: 1, MinZ: -1, MaxZ: 1},
		}
		frameCount int
	)
	if err := video.RenderFrames(context.Background(), timeline, frame, video.Options{FPS: 1, Speed: 20, Scale: 0.75, Workers: 2}, func(_ int, _ float64, _ image.Image) error {
		frameCount++
		return nil
	}); err != nil {
		t.Fatalf("RenderFrames() error = %v", err)
	}
	if frameCount != 2 {
		t.Fatalf("rendered %d frames, want 2", frameCount)
	}
}
