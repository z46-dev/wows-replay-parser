package events

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	replayparser "github.com/z46-dev/wows-replay-parser"
)

// TestDecodeSupportedBuildFixtures verifies the initial timeline decoders on both supported builds.
func TestDecodeSupportedBuildFixtures(t *testing.T) {
	var fixtures []string = []string{
		"../testdata/replays/15.8_236883_Yamato.wowsreplay",
		"../testdata/replays/15.9_20261011_201842.wowsreplay",
	}

	for _, fixture := range fixtures {
		t.Run(fixture, func(t *testing.T) {
			var (
				data     []byte
				replay   *replayparser.Replay
				timeline *Timeline
				err      error
			)

			if data, err = os.ReadFile(fixture); !assert.NoError(t, err) {
				return
			}

			if replay, err = replayparser.Parse(data); !assert.NoError(t, err) {
				return
			}

			if timeline, err = Decode(replay); !assert.NoError(t, err) {
				return
			}

			var positions []Event = Filter(timeline.Events, PositionEvent, 0)
			var orientations []Event = Filter(timeline.Events, PlayerOrientationEvent, 0)
			var ownShipOrientations []Event = Filter(timeline.Events, PlayerOrientationEvent, timeline.OwnShipID)
			var results []Event = Filter(timeline.Events, BattleResultsEvent, 0)
			assert.NotEmpty(t, positions)
			assert.NotEmpty(t, orientations)
			assert.NotEmpty(t, ownShipOrientations)
			assert.Len(t, results, 1)
			assert.NotEmpty(t, results[0].Results.Raw)
			assert.NotZero(t, timeline.OwnShipID)
		})
	}
}
