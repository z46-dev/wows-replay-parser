package replayparser

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMethodExtensions checks Toolkit's handling of bytes after declared method arguments.
func TestMethodExtensions(t *testing.T) {
	var (
		decoder packetDecoder = packetDecoder{
			specs:    []EntitySpec{{Name: "Vehicle", ClientMethods: []MethodSpec{{Name: "receiveGunSyncRotations", Args: []ArgType{{Kind: "UINT8"}, {Kind: "UINT16"}}}}}},
			entities: map[uint32]uint16{7: 1},
		}
		event Event
		err   error
	)

	if event, err = decoder.decode(Packet{ID: 8, Type: "entity_method", Payload: appendTestMethod(7, 0, []byte{2, 4, 5, 0, 0, 0})}); !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, "receiveGunSyncRotations", event.Name)
	assert.Equal(t, []any{uint8(2), uint16(1284)}, event.Args)
	assert.Empty(t, event.Diagnostic)
	assert.Empty(t, event.Leftover)
	assert.Len(t, event.Raw, 18)
}
