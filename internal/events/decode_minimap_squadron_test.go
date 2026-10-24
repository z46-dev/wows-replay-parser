package events

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestDecodeMinimapSquadronMethodsReadsStableFields verifies the 15.9 Avatar minimap methods.
func TestDecodeMinimapSquadronMethodsReadsStableFields(t *testing.T) {
	var addPayload []byte = make([]byte, 22)
	binary.LittleEndian.PutUint64(addPayload[0:8], 493705)
	addPayload[8] = 1
	binary.LittleEndian.PutUint32(addPayload[9:13], 3348987344)
	binary.LittleEndian.PutUint32(addPayload[13:17], math.Float32bits(-232.74))
	binary.LittleEndian.PutUint32(addPayload[17:21], math.Float32bits(-531.17))

	var (
		decoded    Event
		recognized bool
		err        error
	)

	if decoded, recognized, err = decodeMinimapSquadron(30, 7, EntityMember{EntityType: "Avatar", Name: "receive_addMinimapSquadron", Payload: addPayload}); err != nil {
		t.Fatalf("decode add: %v", err)
	}

	if !recognized || decoded.Type != MinimapSquadronEvent || decoded.Squadron == nil || decoded.Squadron.PlaneID != 493705 || decoded.Squadron.TeamID != 1 || decoded.Squadron.GameParamsID != 3348987344 || decoded.Squadron.X != -232.74 || decoded.Squadron.Z != -531.17 {
		t.Fatalf("decoded add = %#v, recognized %t; want parsed marker fields", decoded, recognized)
	}

	var updatePayload []byte = make([]byte, 16)
	binary.LittleEndian.PutUint64(updatePayload[0:8], 493705)
	binary.LittleEndian.PutUint32(updatePayload[8:12], math.Float32bits(17.5))
	binary.LittleEndian.PutUint32(updatePayload[12:16], math.Float32bits(-20.25))

	if decoded, recognized, err = decodeMinimapSquadron(31, 7, EntityMember{EntityType: "Avatar", Name: "receive_updateMinimapSquadron", Payload: updatePayload}); err != nil {
		t.Fatalf("decode update: %v", err)
	}

	if !recognized || decoded.Type != MinimapSquadronPositionEvent || decoded.Squadron == nil || decoded.Squadron.PlaneID != 493705 || decoded.Squadron.X != 17.5 || decoded.Squadron.Z != -20.25 {
		t.Fatalf("decoded update = %#v, recognized %t; want updated marker position", decoded, recognized)
	}

	var removePayload []byte = make([]byte, 8)
	binary.LittleEndian.PutUint64(removePayload, 493705)
	if decoded, recognized, err = decodeMinimapSquadron(32, 7, EntityMember{EntityType: "Avatar", Name: "receive_removeMinimapSquadron", Payload: removePayload}); err != nil {
		t.Fatalf("decode remove: %v", err)
	}

	if !recognized || decoded.Type != MinimapSquadronRemoveEvent || decoded.Squadron == nil || decoded.Squadron.PlaneID != 493705 || !decoded.Squadron.Removed {
		t.Fatalf("decoded remove = %#v, recognized %t; want marker removal", decoded, recognized)
	}
}

// TestDecodeMinimapSquadronMethodsRejectsMalformedPayloads protects replay parsing from bad method lengths.
func TestDecodeMinimapSquadronMethodsRejectsMalformedPayloads(t *testing.T) {
	var payload []byte = make([]byte, 15)
	if _, recognized, err := decodeMinimapSquadron(1, 7, EntityMember{EntityType: "Avatar", Name: "receive_updateMinimapSquadron", Payload: payload}); err == nil || recognized {
		t.Fatalf("decode malformed update = recognized %t, error %v; want error", recognized, err)
	}
}
