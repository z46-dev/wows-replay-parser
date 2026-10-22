package replayparser

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSupportedBuildFixtures(t *testing.T) {
	var fixtures []struct {
		path        string
		version     string
		gameType    string
		mapID       int
		playerShip  string
		playerCount int
	} = []struct {
		path        string
		version     string
		gameType    string
		mapID       int
		playerShip  string
		playerCount int
	}{
		{path: "testdata/replays/15.8_236883_Yamato.wowsreplay", version: "15.8.0.13187581", gameType: "EventBattle", mapID: 8, playerShip: "PJSB018-Yamato-1944", playerCount: 5},
		{path: "testdata/replays/15.9_20261011_201842.wowsreplay", version: "15.9.0.13357625", gameType: "ClanBattle", mapID: 11, playerShip: "PVSC708-Comandante-Aguirre", playerCount: 12},
	}

	for _, fixture := range fixtures {
		t.Run(fixture.version, func(t *testing.T) {
			var (
				data []byte
				err  error
			)

			if data, err = os.ReadFile(fixture.path); !assert.NoError(t, err) {
				return
			}

			var replay *Replay
			if replay, err = Parse(data); !assert.NoError(t, err) {
				return
			}

			assert.Equal(t, fixture.version, replay.ClientVersion)
			assert.Equal(t, fixture.gameType, replay.Metadata.GameType)
			assert.Equal(t, fixture.mapID, replay.Metadata.MapID)
			assert.Equal(t, fixture.playerShip, replay.Metadata.PlayerVehicle)
			assert.Equal(t, fixture.playerCount, replay.Metadata.PlayersPerTeam)
			assert.Equal(t, 2, replay.Metadata.TeamsCount)
			assert.Equal(t, 1200, replay.Metadata.BattleDuration)
			assert.NotEmpty(t, replay.Packets)
			assert.NotEmpty(t, replay.Metadata.MapName)
			assert.NotEmpty(t, replay.Metadata.Raw)
		})
	}
}

func TestParseRejectsInvalidMagic(t *testing.T) {
	var err error
	_, err = Parse(make([]byte, 8))
	assert.Error(t, err)
}

func TestParsePacketsRejectsTruncatedFrame(t *testing.T) {
	_, err := parsePackets(make([]byte, 11))
	assert.Error(t, err)
}
