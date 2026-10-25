package events

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestDecodeVictoryPoints reads the schema-named Avatar score total and rejects malformed values.
func TestDecodeVictoryPoints(t *testing.T) {
	var payload [4]byte
	binary.LittleEndian.PutUint32(payload[:], math.Float32bits(63420))
	var event Event
	var recognized bool
	var err error
	if event, recognized, err = decodeVictoryPoints(12.5, 7, EntityMember{EntityType: "Avatar", Name: "receive_victoryPoints", Payload: payload[:]}); err != nil {
		t.Fatalf("decodeVictoryPoints() error = %v", err)
	}
	if !recognized || event.Type != VictoryPointsEvent || event.VictoryPoints == nil || event.VictoryPoints.TotalPoints != 63420 {
		t.Fatalf("decoded score = %+v, recognized %t", event, recognized)
	}
	binary.LittleEndian.PutUint32(payload[:], math.Float32bits(-700))
	if event, recognized, err = decodeVictoryPoints(12.5, 7, EntityMember{EntityType: "Avatar", Name: "receive_victoryPoints", Payload: payload[:]}); err != nil {
		t.Fatalf("decodeVictoryPoints() rejected a negative score adjustment: %v", err)
	}
	if !recognized || event.VictoryPoints == nil || event.VictoryPoints.TotalPoints != -700 {
		t.Fatalf("decoded negative score adjustment = %+v, recognized %t", event, recognized)
	}
	if _, recognized, err = decodeVictoryPoints(12.5, 7, EntityMember{EntityType: "Avatar", Name: "receive_victoryPoints", Payload: payload[:3]}); err == nil || !recognized {
		t.Fatalf("truncated score error = %v, recognized %t", err, recognized)
	}
	binary.LittleEndian.PutUint32(payload[:], math.Float32bits(float32(math.NaN())))
	if _, recognized, err = decodeVictoryPoints(12.5, 7, EntityMember{EntityType: "Avatar", Name: "receive_victoryPoints", Payload: payload[:]}); err == nil || !recognized {
		t.Fatalf("non-finite score error = %v, recognized %t", err, recognized)
	}
}
