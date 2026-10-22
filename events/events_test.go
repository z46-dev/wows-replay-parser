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
			var entityCreates []Event = Filter(timeline.Events, EntityCreateEvent, 0)
			var entityProperties []Event = Filter(timeline.Events, EntityPropertyEvent, 0)
			var entityMethods []Event = Filter(timeline.Events, EntityMethodEvent, 0)
			var nestedProperties []Event = Filter(timeline.Events, NestedPropertyEvent, 0)
			var leaves []Event = Filter(timeline.Events, EntityLeaveEvent, 0)
			var nonVolatilePositions []Event = Filter(timeline.Events, NonVolatilePositionEvent, 0)
			var weaponLocks []Event = Filter(timeline.Events, WeaponLockEvent, 0)
			var orientations []Event = Filter(timeline.Events, PlayerOrientationEvent, 0)
			var ownShipOrientations []Event = Filter(timeline.Events, PlayerOrientationEvent, timeline.OwnShipID)
			var results []Event = Filter(timeline.Events, BattleResultsEvent, 0)
			assert.NotEmpty(t, positions)
			assert.NotEmpty(t, entityCreates)
			assert.NotNil(t, entityCreates[0].Created)
			assert.NotZero(t, entityCreates[0].Created.EntityTypeID)
			assert.NotEmpty(t, entityProperties)
			assert.NotNil(t, entityProperties[0].Member)
			assert.Len(t, entityProperties[0].Member.Payload, int(entityProperties[0].Member.PayloadSize))
			assert.NotEmpty(t, entityMethods)
			assert.NotNil(t, entityMethods[0].Member)
			assert.Len(t, entityMethods[0].Member.Payload, int(entityMethods[0].Member.PayloadSize))
			assert.NotEmpty(t, nestedProperties)
			assert.NotNil(t, nestedProperties[0].Member)
			assert.Len(t, nestedProperties[0].Member.Payload, int(nestedProperties[0].Member.PayloadSize))
			assert.NotEmpty(t, leaves)
			assert.NotEmpty(t, nonVolatilePositions)
			assert.NotNil(t, nonVolatilePositions[0].Position)
			assert.NotEmpty(t, weaponLocks)
			assert.NotNil(t, weaponLocks[0].WeaponLock)
			assert.NotEmpty(t, orientations)
			assert.NotEmpty(t, ownShipOrientations)
			assert.Len(t, results, 1)
			assert.NotEmpty(t, results[0].Results.Raw)
			assert.NotZero(t, timeline.OwnShipID)
		})
	}
}
