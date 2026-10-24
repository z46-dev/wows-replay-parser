package events

import (
	"encoding/binary"
	"testing"
)

// TestDecodeMinimapVisionInfoUnpacksWorldPositions checks the packed WG minimap update wire layout.
func TestDecodeMinimapVisionInfoUnpacksWorldPositions(t *testing.T) {
	var payload []byte = make([]byte, 10)
	payload[0] = 1
	binary.LittleEndian.PutUint32(payload[1:5], 502)

	var packed uint32 = 1024 | 1024<<11 | 128<<22
	binary.LittleEndian.PutUint32(payload[5:9], packed)
	payload[9] = 0

	var (
		member     EntityMember = EntityMember{EntityType: "Avatar", Name: "updateMinimapVisionInfo", Payload: payload}
		decoded    []Event
		recognized bool
		err        error
	)

	if decoded, recognized, err = decodeMinimapVisionInfo(25, 7, member); err != nil {
		t.Fatalf("decodeMinimapVisionInfo() error = %v", err)
	}

	if !recognized || len(decoded) != 1 || decoded[0].EntityID != 502 || decoded[0].ParentID != 7 || decoded[0].Minimap == nil {
		t.Fatalf("decoded minimap events = %#v, recognized = %t", decoded, recognized)
	}

	if decoded[0].Minimap.X < 1.2 || decoded[0].Minimap.X > 1.3 || decoded[0].Minimap.Z < 1.2 || decoded[0].Minimap.Z > 1.3 || decoded[0].Minimap.Heading != 0 {
		t.Fatalf("decoded minimap position = %#v; want near origin and heading 0", decoded[0].Minimap)
	}
}

// TestDecodeMinimapVisionInfoRejectsTruncatedArrays prevents malformed method payloads from being accepted.
func TestDecodeMinimapVisionInfoRejectsTruncatedArrays(t *testing.T) {
	var member EntityMember = EntityMember{EntityType: "Avatar", Name: "updateMinimapVisionInfo", Payload: []byte{1, 0}}
	if _, recognized, err := decodeMinimapVisionInfo(0, 1, member); err == nil || !recognized {
		t.Fatalf("decodeMinimapVisionInfo() = recognized %t, error %v; want a recognized malformed payload error", recognized, err)
	}
}
