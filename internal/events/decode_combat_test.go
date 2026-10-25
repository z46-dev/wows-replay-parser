package events

import (
	"encoding/binary"
	"testing"
)

func TestDecodeShipDeathReadsVictimKillerAndCause(t *testing.T) {
	var payload []byte = make([]byte, 12)
	binary.LittleEndian.PutUint32(payload[0:4], 270391)
	binary.LittleEndian.PutUint32(payload[4:8], 270159)
	binary.LittleEndian.PutUint32(payload[8:12], 18)

	var (
		event      Event
		recognized bool
		err        error
	)
	if event, recognized, err = decodeShipDeath(251.5, EntityMember{Name: "receiveVehicleDeath", Payload: payload}); err != nil {
		t.Fatalf("decodeShipDeath() error = %v", err)
	}
	if !recognized || event.Death == nil || event.Death.VictimID != 270391 || event.Death.KillerID != 270159 || event.Death.Cause != 18 {
		t.Fatalf("decodeShipDeath() = %+v, recognized %v", event, recognized)
	}
}

func TestDecodeDamageReceivedMatchesToolkitPacket(t *testing.T) {
	var (
		payload    []byte = []byte{1, 0x33, 0x1f, 0x04, 0x00, 0x00, 0xe8, 0x42, 0x46}
		decoded    []Event
		recognized bool
		err        error
	)
	if decoded, recognized, err = decodeDamageReceived(94.5, 270117, EntityMember{EntityType: "Vehicle", Name: "receiveDamagesOnShip", Payload: payload}); err != nil {
		t.Fatalf("decodeDamageReceived() error = %v", err)
	}
	if !recognized || len(decoded) != 1 || decoded[0].DamageDealt == nil {
		t.Fatalf("decodeDamageReceived() = %+v, recognized %v", decoded, recognized)
	}
	if decoded[0].EntityID != 270117 || decoded[0].DamageDealt.AggressorID != 270131 || decoded[0].DamageDealt.Damage != 12474 {
		t.Fatalf("decoded damage = %+v, want victim 270117, aggressor 270131, damage 12474", decoded[0])
	}
}

func TestDecodeDamageReceivedRejectsMalformedPayload(t *testing.T) {
	if _, _, err := decodeDamageReceived(0, 1, EntityMember{EntityType: "Vehicle", Name: "receiveDamagesOnShip", Payload: []byte{1, 1, 2, 3}}); err == nil {
		t.Fatal("decodeDamageReceived() accepted a truncated damage record")
	}
}
