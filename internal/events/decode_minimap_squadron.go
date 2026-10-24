package events

import (
	"encoding/binary"
	"fmt"
	"math"
)

// decodeMinimapSquadron decodes the Avatar methods that add, move, or remove minimap aircraft.
func decodeMinimapSquadron(clock float32, entityID uint32, member EntityMember) (event Event, recognized bool, err error) {
	if member.EntityType != "Avatar" {
		return
	}

	var (
		payload []byte = member.Payload
		planeID int64
		typeID  Type
	)

	switch member.Name {
	case "receive_addMinimapSquadron":
		if len(payload) != 22 || payload[21] > 1 {
			err = fmt.Errorf("add-minimap-squadron payload has invalid size or boolean")
			return
		}

		planeID = int64(binary.LittleEndian.Uint64(payload[0:8]))
		typeID = MinimapSquadronEvent
		event.Squadron = &MinimapSquadron{
			PlaneID:      planeID,
			TeamID:       int8(payload[8]),
			GameParamsID: binary.LittleEndian.Uint32(payload[9:13]),
			X:            math.Float32frombits(binary.LittleEndian.Uint32(payload[13:17])),
			Z:            math.Float32frombits(binary.LittleEndian.Uint32(payload[17:21])),
		}
	case "receive_updateMinimapSquadron":
		if len(payload) != 16 {
			err = fmt.Errorf("update-minimap-squadron payload has %d bytes, expected 16", len(payload))
			return
		}

		planeID = int64(binary.LittleEndian.Uint64(payload[0:8]))
		typeID = MinimapSquadronPositionEvent
		event.Squadron = &MinimapSquadron{
			PlaneID: planeID,
			X:       math.Float32frombits(binary.LittleEndian.Uint32(payload[8:12])),
			Z:       math.Float32frombits(binary.LittleEndian.Uint32(payload[12:16])),
		}
	case "receive_removeMinimapSquadron":
		if len(payload) != 8 {
			err = fmt.Errorf("remove-minimap-squadron payload has %d bytes, expected 8", len(payload))
			return
		}

		planeID = int64(binary.LittleEndian.Uint64(payload))
		typeID = MinimapSquadronRemoveEvent
		event.Squadron = &MinimapSquadron{PlaneID: planeID, Removed: true}
	default:
		return
	}

	if planeID <= 0 || math.IsNaN(float64(event.Squadron.X)) || math.IsInf(float64(event.Squadron.X), 0) || math.IsNaN(float64(event.Squadron.Z)) || math.IsInf(float64(event.Squadron.Z), 0) {
		err = fmt.Errorf("minimap-squadron payload contains invalid ID or coordinates")
		return
	}

	event.Clock = clock
	event.Type = typeID
	event.EntityID = entityID
	recognized = true
	return
}
