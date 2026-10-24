package events

import (
	"encoding/binary"
	"fmt"
	"math"
)

// decodeTorpedoPosition decodes the Avatar synchronization of a torpedo's owner, ID, and position.
func decodeTorpedoPosition(clock float32, member EntityMember) (event Event, recognized bool, err error) {
	if member.EntityType != "Avatar" || member.Name != "receiveTorpedoSynchronization" {
		return
	}

	if len(member.Payload) != 18 {
		err = fmt.Errorf("torpedo-synchronization payload has %d bytes, expected 18", len(member.Payload))
		return
	}

	var position Position = positionAt(member.Payload, 6)
	if binary.LittleEndian.Uint32(member.Payload[0:4]) == 0 || !finite(position.X) || !finite(position.Y) || !finite(position.Z) {
		err = fmt.Errorf("torpedo-synchronization payload contains invalid owner or coordinates")
		return
	}

	event = Event{
		Clock: clock,
		Type:  TorpedoPositionEvent,
		Torpedo: &TorpedoPosition{
			OwnerID:  binary.LittleEndian.Uint32(member.Payload[0:4]),
			ShotID:   binary.LittleEndian.Uint16(member.Payload[4:6]),
			Position: position,
		},
	}

	recognized = true
	return
}

// finite reports whether a decoded replay coordinate is finite.
func finite(value float32) (valid bool) {
	valid = !math.IsNaN(float64(value)) && !math.IsInf(float64(value), 0)
	return
}
