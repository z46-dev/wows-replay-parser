package events

import (
	"encoding/binary"
	"fmt"
	"math"
)

const minimapInfoEntrySize int = 8

// decodeMinimapVisionInfo decodes the two MINIMAPINFO array arguments on Avatar.updateMinimapVisionInfo.
func decodeMinimapVisionInfo(clock float32, avatarID uint32, member EntityMember) (decoded []Event, recognized bool, err error) {
	if member.EntityType != "Avatar" || member.Name != "updateMinimapVisionInfo" {
		return
	}

	recognized = true

	var (
		payload []byte = member.Payload
		offset  int
	)

	for argumentIndex := 0; argumentIndex < 2; argumentIndex++ {
		if offset >= len(payload) {
			err = fmt.Errorf("minimap-vision argument %d is missing its array count", argumentIndex)
			return
		}

		var count int = int(payload[offset])
		offset++
		if count > (len(payload)-offset)/minimapInfoEntrySize {
			err = fmt.Errorf("minimap-vision argument %d has a truncated array", argumentIndex)
			return
		}

		if argumentIndex == 0 {
			for index := 0; index < count; index++ {
				var (
					entry     []byte = payload[offset : offset+minimapInfoEntrySize]
					vehicleID uint32 = binary.LittleEndian.Uint32(entry[0:4])
					packed    uint32 = binary.LittleEndian.Uint32(entry[4:8])
					rawX      uint32 = packed & 0x7ff
					rawY      uint32 = (packed >> 11) & 0x7ff
				)

				if vehicleID == 0 {
					err = fmt.Errorf("minimap-vision entry %d has an invalid entity ID", index)
					return
				}

				var position MinimapShipPosition = MinimapShipPosition{
					VehicleID:    vehicleID,
					X:            float32(float64(rawX)/2047*5000 - 2500),
					Z:            float32(float64(rawY)/2047*5000 - 2500),
					Heading:      float32((packed>>22)&0xff)/256*360 - 180,
					IsSentinel:   rawX == 0 && rawY == 0,
					Disappearing: packed&(1<<31) != 0,
				}

				if math.IsNaN(float64(position.X)) || math.IsInf(float64(position.X), 0) || math.IsNaN(float64(position.Z)) || math.IsInf(float64(position.Z), 0) {
					err = fmt.Errorf("minimap-vision entry %d has invalid coordinates", index)
					return
				}

				decoded = append(decoded, Event{Clock: clock, Type: MinimapShipPositionEvent, EntityID: vehicleID, ParentID: avatarID, Minimap: &position})
				offset += minimapInfoEntrySize
			}
		} else {
			offset += count * minimapInfoEntrySize
		}
	}

	if offset != len(payload) {
		err = fmt.Errorf("minimap-vision payload has %d unexpected trailing bytes", len(payload)-offset)
	}

	return
}
