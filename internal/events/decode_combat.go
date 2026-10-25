package events

import (
	"encoding/binary"
	"fmt"
	"math"
)

const damageRecordSize int = 8

// decodeShipDeath decodes the victim, killer, and cause in receiveVehicleDeath.
func decodeShipDeath(clock float32, member EntityMember) (event Event, recognized bool, err error) {
	if member.Name != "receiveVehicleDeath" {
		return
	}
	if len(member.Payload) != 12 {
		err = fmt.Errorf("receiveVehicleDeath payload has %d bytes, expected 12", len(member.Payload))
		return
	}

	event = Event{
		Clock: clock,
		Type:  ShipDeathEvent,
		Death: &ShipDeath{
			VictimID: binary.LittleEndian.Uint32(member.Payload[0:4]),
			KillerID: binary.LittleEndian.Uint32(member.Payload[4:8]),
			Cause:    binary.LittleEndian.Uint32(member.Payload[8:12]),
		},
	}
	recognized = true
	return
}

// decodeDamageReceived decodes ARRAY<DAMAGES> updates sent to a victim vehicle.
func decodeDamageReceived(clock float32, victimID uint32, member EntityMember) (events []Event, recognized bool, err error) {
	if member.EntityType != "Vehicle" || member.Name != "receiveDamagesOnShip" {
		return
	}

	recognized = true
	if len(member.Payload) == 0 {
		err = fmt.Errorf("receiveDamagesOnShip payload is missing its array count")
		return
	}
	if len(member.Payload) != 1+int(member.Payload[0])*damageRecordSize {
		err = fmt.Errorf("receiveDamagesOnShip payload has %d bytes for %d damage records", len(member.Payload), member.Payload[0])
		return
	}

	events = make([]Event, 0, int(member.Payload[0]))
	for recordIndex := range int(member.Payload[0]) {
		var (
			offset      int     = 1 + recordIndex*damageRecordSize
			aggressorID uint32  = uint32(binary.LittleEndian.Uint32(member.Payload[offset : offset+4]))
			damage      float32 = math.Float32frombits(binary.LittleEndian.Uint32(member.Payload[offset+4 : offset+8]))
		)
		if aggressorID == 0 {
			err = fmt.Errorf("receiveDamagesOnShip record %d has an invalid aggressor ID", recordIndex)
			return
		}
		if math.IsNaN(float64(damage)) || math.IsInf(float64(damage), 0) || damage < 0 {
			err = fmt.Errorf("receiveDamagesOnShip record %d has invalid damage %v", recordIndex, damage)
			return
		}

		events = append(events, Event{
			Clock:    clock,
			Type:     ShipDamageDealtEvent,
			EntityID: victimID,
			DamageDealt: &ShipDamageDealt{
				VictimID: victimID, AggressorID: aggressorID, Damage: damage,
			},
		})
	}
	return
}
