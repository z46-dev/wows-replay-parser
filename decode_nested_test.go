package replayparser

import (
	"encoding/binary"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestNestedPropertyUpdateChecksState checks Toolkit's MSB-first path and state mutation.
func TestNestedPropertyUpdateChecksState(t *testing.T) {
	var (
		spec    EntitySpec    = EntitySpec{Name: "Avatar", Properties: []PropertySpec{{Name: "state", Type: ArgType{Kind: "FIXED_DICT", Properties: []ArgField{{Name: "count", Type: ArgType{Kind: "UINT8"}}}}}}}
		decoder packetDecoder = packetDecoder{specs: []EntitySpec{spec}, entities: map[uint32]uint16{7: 1}, properties: make(map[uint32][]any)}
		payload []byte        = make([]byte, 11)
	)

	binary.LittleEndian.PutUint32(payload, 7)
	binary.LittleEndian.PutUint32(payload[5:], 2)
	payload[9] = 0x80
	payload[10] = 5

	var (
		event Event
		err   error
	)

	event, err = decoder.decode(Packet{Type: "nested_property_update", Payload: payload})
	assert.NoError(t, err)
	assert.Empty(t, event.Diagnostic)
	assert.Equal(t, "state", event.Name)
	assert.Equal(t, NestedUpdateValue{Action: "set_key", Key: "count", Value: uint8(5)}, event.Value)
	assert.Equal(t, uint8(5), decoder.properties[7][0].(map[string]any)["count"])
}

// TestMethodSizeMismatchStopsWalk checks that a stale exposed-method map is fatal.
func TestMethodSizeMismatchStopsWalk(t *testing.T) {
	var (
		decoder packetDecoder = packetDecoder{
			specs:      []EntitySpec{{Name: "Vehicle", ClientMethods: []MethodSpec{{Name: "sync", Args: []ArgType{{Kind: "UINT32"}}, FixedSize: 4}}}},
			entities:   map[uint32]uint16{7: 1},
			properties: make(map[uint32][]any),
		}
		payload []byte = make([]byte, 15)
	)

	binary.LittleEndian.PutUint32(payload, 7)
	binary.LittleEndian.PutUint32(payload[8:], 3)

	var err error
	_, err = decoder.decode(Packet{Type: "entity_method", Payload: payload})
	assert.ErrorIs(t, err, errMethodMappingDrift)
}
