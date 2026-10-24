package events

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestDecodeTorpedoPositionReadsOwnerShotAndWorldCoordinates checks the 15.9 synchronization layout.
func TestDecodeTorpedoPositionReadsOwnerShotAndWorldCoordinates(t *testing.T) {
	var payload []byte = make([]byte, 18)
	binary.LittleEndian.PutUint32(payload[0:4], 493709)
	binary.LittleEndian.PutUint16(payload[4:6], 28)
	binary.LittleEndian.PutUint32(payload[6:10], math.Float32bits(-555.4453))
	binary.LittleEndian.PutUint32(payload[10:14], math.Float32bits(-0.14))
	binary.LittleEndian.PutUint32(payload[14:18], math.Float32bits(-80.45235))

	var (
		event      Event
		recognized bool
		err        error
	)

	if event, recognized, err = decodeTorpedoPosition(487.4, EntityMember{EntityType: "Avatar", Name: "receiveTorpedoSynchronization", Payload: payload}); err != nil {
		t.Fatalf("decodeTorpedoPosition() error = %v", err)
	}

	if !recognized || event.Type != TorpedoPositionEvent || event.Torpedo == nil || event.Torpedo.OwnerID != 493709 || event.Torpedo.ShotID != 28 || event.Torpedo.Position.X != -555.4453 || event.Torpedo.Position.Z != -80.45235 {
		t.Fatalf("decoded torpedo = %#v, recognized %t; want owner, shot ID, and world position", event.Torpedo, recognized)
	}
}

// TestDecodeTorpedoPositionRejectsMalformedPayloads protects the method boundary.
func TestDecodeTorpedoPositionRejectsMalformedPayloads(t *testing.T) {
	if _, recognized, err := decodeTorpedoPosition(1, EntityMember{EntityType: "Avatar", Name: "receiveTorpedoSynchronization", Payload: make([]byte, 17)}); err == nil || recognized {
		t.Fatalf("malformed torpedo synchronization = recognized %t, error %v; want error", recognized, err)
	}

	var payload []byte = make([]byte, 18)
	binary.LittleEndian.PutUint32(payload[0:4], 493709)
	binary.LittleEndian.PutUint32(payload[6:10], math.Float32bits(float32(math.NaN())))
	if _, recognized, err := decodeTorpedoPosition(1, EntityMember{EntityType: "Avatar", Name: "receiveTorpedoSynchronization", Payload: payload}); err == nil || recognized {
		t.Fatalf("non-finite torpedo synchronization = recognized %t, error %v; want error", recognized, err)
	}
}
