package replayparser

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseSupportedBuildFixtures(t *testing.T) {
	var fixtures []struct {
		path    string
		version string
	} = []struct {
		path    string
		version string
	}{
		{path: "testdata/replays/15.8_236883_Yamato.wowsreplay", version: "15.8.0.13187581"},
		{path: "testdata/replays/15.9_20261011_201842.wowsreplay", version: "15.9.0.13357625"},
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
			assert.NotEmpty(t, replay.Packets)
			assert.NotEmpty(t, replay.Metadata.MapName)
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
