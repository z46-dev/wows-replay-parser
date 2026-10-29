package replayparser

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestFixtureContainers checks metadata and packet framing against saved replays.
func TestFixtureContainers(t *testing.T) {
	var (
		files []string
		err   error
	)

	if files, err = filepath.Glob("testdata/replays/*.wowsreplay"); !assert.NoError(t, err) {
		return
	}

	if !assert.NotEmpty(t, files) {
		return
	}

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			var (
				metadata Metadata
				replay   *Replay
				iterator *PacketIterator
				err      error
				count    int
			)

			if metadata, err = ReadMetadata(path); !assert.NoError(t, err) {
				return
			}

			if replay, err = Open(path); !assert.NoError(t, err) {
				return
			}

			assert.Equal(t, metadata.ClientVersionFromExe, replay.Metadata.ClientVersionFromExe)
			assert.NotEmpty(t, metadata.ClientVersionFromExe)
			if iterator, err = replay.Packets(); !assert.NoError(t, err) {
				return
			}

			for {
				if _, err = iterator.Next(); errors.Is(err, io.EOF) {
					break
				}

				if !assert.NoError(t, err) {
					return
				}

				count++
			}

			assert.Positive(t, count)
		})
	}
}

// TestWalkPacketsCancellation checks that a caller can stop packet traversal.
func TestWalkPacketsCancellation(t *testing.T) {
	var (
		ctx    context.Context
		cancel context.CancelFunc
	)

	ctx, cancel = context.WithCancel(context.Background())
	cancel()

	var (
		replay *Replay = &Replay{Metadata: Metadata{ClientVersionFromExe: "15.9.0.13357625"}}
		err    error
	)

	err = replay.WalkPackets(ctx, func(Packet) error { return nil })
	assert.ErrorIs(t, err, context.Canceled)
}

// TestVersionedPacketIDs checks Toolkit's legacy and modern mapping boundary.
func TestVersionedPacketIDs(t *testing.T) {
	assert.Equal(t, "nested_property_update", packetType(0x22, false))
	assert.Equal(t, "battle_results", packetType(0x22, true))
}

// TestPacketIteratorRejectsOversizedPayload checks the untrusted length boundary.
func TestPacketIteratorRejectsOversizedPayload(t *testing.T) {
	var data []byte = make([]byte, 12)
	binary.LittleEndian.PutUint32(data, ^uint32(0))
	var iterator *PacketIterator = &PacketIterator{data: data}
	var err error

	_, err = iterator.Next()
	assert.Error(t, err)
	_, err = iterator.Next()
	assert.ErrorIs(t, err, io.EOF)
}
