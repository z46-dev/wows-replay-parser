package events

import (
	"encoding/binary"
	"fmt"
	"math"
)

// decodeVictoryPoints decodes the Avatar score total without estimating it from capture ownership.
func decodeVictoryPoints(clock float32, entityID uint32, member EntityMember) (event Event, recognized bool, err error) {
	if member.EntityType != "Avatar" || member.Name != "receive_victoryPoints" {
		return
	}

	recognized = true
	if len(member.Payload) != 4 {
		err = fmt.Errorf("receive_victoryPoints payload has %d bytes, expected 4", len(member.Payload))
		return
	}

	var totalPoints float32 = math.Float32frombits(binary.LittleEndian.Uint32(member.Payload))
	if math.IsNaN(float64(totalPoints)) || math.IsInf(float64(totalPoints), 0) {
		err = fmt.Errorf("receive_victoryPoints total is invalid: %v (0x%08x)", totalPoints, math.Float32bits(totalPoints))
		return
	}

	event = Event{
		Clock:    clock,
		Type:     VictoryPointsEvent,
		EntityID: entityID,
		VictoryPoints: &VictoryPointUpdate{
			TotalPoints: totalPoints,
		},
	}
	return
}
