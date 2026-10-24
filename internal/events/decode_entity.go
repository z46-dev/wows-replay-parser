package events

import (
	"encoding/binary"
	"fmt"
	"math"

	replayparser "github.com/z46-dev/wows-replay-parser"
)

// decodeBasePlayerCreate decodes a base entity bootstrap and preserves its schema-specific state.
func decodeBasePlayerCreate(packet replayparser.Packet, eventType Type) (event Event, err error) {
	if len(packet.Payload) < basePlayerHeaderSize {
		err = fmt.Errorf("%s packet has %d-byte payload, expected at least %d", eventType, len(packet.Payload), basePlayerHeaderSize)
		return
	}

	var (
		payload   []byte = packet.Payload
		stateSize uint32 = binary.LittleEndian.Uint32(packet.Payload[6:10])
	)

	if uint64(stateSize) != uint64(len(payload)-basePlayerHeaderSize) {
		err = fmt.Errorf("%s state length does not match packet", eventType)
		return
	}

	event = Event{
		Clock:    packet.Clock,
		Type:     eventType,
		EntityID: binary.LittleEndian.Uint32(payload[0:4]),
		Bootstrap: &EntityBootstrap{
			EntityTypeID: binary.LittleEndian.Uint16(payload[4:6]),
			StateSize:    stateSize,
			State:        payload[basePlayerHeaderSize:],
		},
	}

	return
}

// decodeCellPlayerCreate decodes an entity's cell bootstrap and preserves its schema-specific state.
func decodeCellPlayerCreate(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) < cellPlayerHeaderSize {
		err = fmt.Errorf("cell-player-create packet has %d-byte payload, expected at least %d", len(packet.Payload), cellPlayerHeaderSize)
		return
	}

	var (
		payload   []byte = packet.Payload
		stateSize uint32 = binary.LittleEndian.Uint32(payload[36:40])
	)

	if uint64(stateSize) != uint64(len(payload)-cellPlayerHeaderSize) {
		err = fmt.Errorf("cell-player-create state length does not match packet")
		return
	}

	event = Event{
		Clock:    packet.Clock,
		Type:     CellPlayerCreateEvent,
		EntityID: binary.LittleEndian.Uint32(payload[0:4]),
		Bootstrap: &EntityBootstrap{
			SpaceID:   binary.LittleEndian.Uint32(payload[4:8]),
			VehicleID: binary.LittleEndian.Uint32(payload[8:12]),
			Position: &Position{
				X:     float32At(payload, 12),
				Y:     float32At(payload, 16),
				Z:     float32At(payload, 20),
				Yaw:   float32At(payload, 24),
				Pitch: float32At(payload, 28),
				Roll:  float32At(payload, 32),
			},
			StateSize: stateSize,
			State:     payload[cellPlayerHeaderSize:],
		},
	}

	return
}

// decodeEntityEnter decodes an entity entering a replay space.
func decodeEntityEnter(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != entityEnterPayloadSize {
		err = fmt.Errorf("entity-enter packet has %d-byte payload, expected %d", len(packet.Payload), entityEnterPayloadSize)
		return
	}

	event = Event{
		Clock:    packet.Clock,
		Type:     EntityEnterEvent,
		EntityID: binary.LittleEndian.Uint32(packet.Payload[0:4]),
		Bootstrap: &EntityBootstrap{
			SpaceID:   binary.LittleEndian.Uint32(packet.Payload[4:8]),
			VehicleID: binary.LittleEndian.Uint32(packet.Payload[8:12]),
		},
	}

	return
}

// decodeEntityMember decodes the common fixed envelope around entity property and method payloads.
func decodeEntityMember(packet replayparser.Packet, eventType Type) (event Event, err error) {
	if len(packet.Payload) < entityMemberHeaderSize {
		err = fmt.Errorf("%s packet has %d-byte payload, expected at least %d", eventType, len(packet.Payload), entityMemberHeaderSize)
		return
	}

	var (
		payload     []byte = packet.Payload
		payloadSize uint32 = binary.LittleEndian.Uint32(payload[8:12])
	)

	if uint64(payloadSize) != uint64(len(payload)-entityMemberHeaderSize) {
		err = fmt.Errorf("%s payload length does not match packet", eventType)
		return
	}

	event = Event{
		Clock:    packet.Clock,
		Type:     eventType,
		EntityID: binary.LittleEndian.Uint32(payload[0:4]),
		Member: &EntityMember{
			MemberID:    binary.LittleEndian.Uint32(payload[4:8]),
			PayloadSize: payloadSize,
			Payload:     payload[entityMemberHeaderSize:],
		},
	}

	return
}

// decodeShipHealth decodes the stable Vehicle health property and emits its delta.
func decodeShipHealth(packet replayparser.Packet, entityID uint32, previous map[uint32]float32) (event Event, err error) {
	if len(packet.Payload) != entityMemberHeaderSize+4 {
		err = fmt.Errorf("ship-health packet has %d-byte payload, expected %d", len(packet.Payload), entityMemberHeaderSize+4)
		return
	}

	var (
		current  float32 = float32At(packet.Payload, entityMemberHeaderSize)
		prior    float32
		hasPrior bool
		health   *ShipHealth
	)

	if prior, hasPrior = previous[entityID]; hasPrior {
		health = &ShipHealth{Current: current, Previous: prior, Delta: current - prior}
	} else {
		health = &ShipHealth{Current: current}
	}

	previous[entityID] = current
	event = Event{Clock: packet.Clock, Type: ShipHealthEvent, EntityID: entityID, Health: health}
	return
}

// decodeSpottedEntities decodes a count followed by little-endian entity IDs.
func decodeSpottedEntities(clock float32, entityID uint32, payload []byte) (event Event, err error) {
	if len(payload) == 0 || len(payload) != 1+int(payload[0])*4 {
		err = fmt.Errorf("spotted-entities payload has invalid count or length")
		return
	}

	var spotted SpottedEntities = SpottedEntities{EntityIDs: make([]uint32, int(payload[0]))}
	for index := range spotted.EntityIDs {
		var offset int = 1 + index*4
		spotted.EntityIDs[index] = binary.LittleEndian.Uint32(payload[offset : offset+4])
	}

	event = Event{Clock: clock, Type: ShipVisibilityEvent, EntityID: entityID, Visibility: &spotted}
	return
}

// decodeNestedProperty decodes the stable nested-property envelope before its type-specific payload.
func decodeNestedProperty(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) < nestedPropertyHeaderSize {
		err = fmt.Errorf("nested-property packet has %d-byte payload, expected at least %d", len(packet.Payload), nestedPropertyHeaderSize)
		return
	}

	var (
		payload []byte = packet.Payload
		flags   byte   = payload[4]
	)

	if flags&0xfe != 0 {
		err = fmt.Errorf("nested-property packet has invalid flags %d", flags)
		return
	}

	var payloadSize uint32 = binary.LittleEndian.Uint32(payload[5:9])
	if uint64(payloadSize) != uint64(len(payload)-nestedPropertyHeaderSize) {
		err = fmt.Errorf("nested-property payload length does not match packet")
		return
	}

	var isSlice bool = flags == 1
	event = Event{
		Clock:    packet.Clock,
		Type:     NestedPropertyEvent,
		EntityID: binary.LittleEndian.Uint32(payload[0:4]),
		Member: &EntityMember{
			PayloadSize: payloadSize,
			IsSlice:     &isSlice,
			Payload:     payload[nestedPropertyHeaderSize:],
		},
	}

	return
}

// decodeEntityCreate decodes the fixed portion of an entity creation packet.
func decodeEntityCreate(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) < entityCreateHeaderSize {
		err = fmt.Errorf("entity-create packet has %d-byte payload, expected at least %d", len(packet.Payload), entityCreateHeaderSize)
		return
	}

	var payload []byte = packet.Payload
	event = Event{
		Clock:    packet.Clock,
		Type:     EntityCreateEvent,
		EntityID: binary.LittleEndian.Uint32(payload[0:4]),
		Created: &EntityCreate{
			EntityTypeID: binary.LittleEndian.Uint16(payload[4:6]),
			SpaceID:      binary.LittleEndian.Uint32(payload[6:10]),
			VehicleID:    binary.LittleEndian.Uint32(payload[10:14]),
			Position: &Position{
				X:     math.Float32frombits(binary.LittleEndian.Uint32(payload[14:18])),
				Y:     math.Float32frombits(binary.LittleEndian.Uint32(payload[18:22])),
				Z:     math.Float32frombits(binary.LittleEndian.Uint32(payload[22:26])),
				Yaw:   math.Float32frombits(binary.LittleEndian.Uint32(payload[26:30])),
				Pitch: math.Float32frombits(binary.LittleEndian.Uint32(payload[30:34])),
				Roll:  math.Float32frombits(binary.LittleEndian.Uint32(payload[34:38])),
			},
			StateLength: binary.LittleEndian.Uint32(payload[38:42]),
			StateSize:   uint32(len(payload) - entityCreateHeaderSize),
			State:       payload[entityCreateHeaderSize:],
		},
	}

	return
}

// decodeEntityLeave decodes an entity identifier leaving the replay space.
func decodeEntityLeave(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != 4 {
		err = fmt.Errorf("entity-leave packet has %d-byte payload, expected 4", len(packet.Payload))
		return
	}

	event = Event{Clock: packet.Clock, Type: EntityLeaveEvent, EntityID: binary.LittleEndian.Uint32(packet.Payload)}
	return
}

// decodeNonVolatilePosition decodes a position update without dead-reckoning direction.
func decodeNonVolatilePosition(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != nonVolatilePayloadSize {
		err = fmt.Errorf("nonvolatile-position packet has %d-byte payload, expected %d", len(packet.Payload), nonVolatilePayloadSize)
		return
	}

	var payload []byte = packet.Payload
	event = Event{
		Clock:    packet.Clock,
		Type:     NonVolatilePositionEvent,
		EntityID: binary.LittleEndian.Uint32(payload[0:4]),
		Position: &Position{
			X:     math.Float32frombits(binary.LittleEndian.Uint32(payload[8:12])),
			Y:     math.Float32frombits(binary.LittleEndian.Uint32(payload[12:16])),
			Z:     math.Float32frombits(binary.LittleEndian.Uint32(payload[16:20])),
			Yaw:   math.Float32frombits(binary.LittleEndian.Uint32(payload[20:24])),
			Pitch: math.Float32frombits(binary.LittleEndian.Uint32(payload[24:28])),
			Roll:  math.Float32frombits(binary.LittleEndian.Uint32(payload[28:32])),
		},
	}

	return
}

// decodeEntityControl decodes the replay controller's entity ownership change.
func decodeEntityControl(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != 5 {
		err = fmt.Errorf("entity-control packet has %d-byte payload, expected 5", len(packet.Payload))
		return
	}

	if packet.Payload[4] > 1 {
		err = fmt.Errorf("entity-control packet has invalid control flag %d", packet.Payload[4])
		return
	}

	var controlled bool = packet.Payload[4] == 1
	event = Event{
		Clock:      packet.Clock,
		Type:       EntityControlEvent,
		EntityID:   binary.LittleEndian.Uint32(packet.Payload[0:4]),
		Controlled: &controlled,
	}

	return
}

// decodeOwnShip decodes the player ship entity identifier.
func decodeOwnShip(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != 4 {
		err = fmt.Errorf("own-ship packet has %d-byte payload, expected 4", len(packet.Payload))
		return
	}

	event = Event{
		Clock:    packet.Clock,
		Type:     OwnShipEvent,
		EntityID: binary.LittleEndian.Uint32(packet.Payload),
	}

	return
}

// decodePlayerOrientation decodes the fixed player-or-camera orientation packet layout.
func decodePlayerOrientation(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != orientationPayloadSize {
		err = fmt.Errorf("player-orientation packet has %d-byte payload, expected %d", len(packet.Payload), orientationPayloadSize)
		return
	}

	var payload []byte = packet.Payload
	event = Event{
		Clock:    packet.Clock,
		Type:     PlayerOrientationEvent,
		EntityID: binary.LittleEndian.Uint32(payload[0:4]),
		ParentID: binary.LittleEndian.Uint32(payload[4:8]),
		Position: &Position{
			X:     math.Float32frombits(binary.LittleEndian.Uint32(payload[8:12])),
			Y:     math.Float32frombits(binary.LittleEndian.Uint32(payload[12:16])),
			Z:     math.Float32frombits(binary.LittleEndian.Uint32(payload[16:20])),
			Yaw:   math.Float32frombits(binary.LittleEndian.Uint32(payload[20:24])),
			Pitch: math.Float32frombits(binary.LittleEndian.Uint32(payload[24:28])),
			Roll:  math.Float32frombits(binary.LittleEndian.Uint32(payload[28:32])),
		},
	}

	return
}

// decodePosition decodes the fixed modern BigWorld position-packet layout.
func decodePosition(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != positionPayloadSize {
		err = fmt.Errorf("position packet has %d-byte payload, expected %d", len(packet.Payload), positionPayloadSize)
		return
	}

	var payload []byte = packet.Payload
	event = Event{
		Clock:    packet.Clock,
		Type:     PositionEvent,
		EntityID: binary.LittleEndian.Uint32(payload[0:4]),
		Position: &Position{
			X:          math.Float32frombits(binary.LittleEndian.Uint32(payload[8:12])),
			Y:          math.Float32frombits(binary.LittleEndian.Uint32(payload[12:16])),
			Z:          math.Float32frombits(binary.LittleEndian.Uint32(payload[16:20])),
			DirectionX: math.Float32frombits(binary.LittleEndian.Uint32(payload[20:24])),
			DirectionY: math.Float32frombits(binary.LittleEndian.Uint32(payload[24:28])),
			DirectionZ: math.Float32frombits(binary.LittleEndian.Uint32(payload[28:32])),
			Yaw:        math.Float32frombits(binary.LittleEndian.Uint32(payload[32:36])),
			Pitch:      math.Float32frombits(binary.LittleEndian.Uint32(payload[36:40])),
			Roll:       math.Float32frombits(binary.LittleEndian.Uint32(payload[40:44])),
			IsOnGround: payload[44] != 0,
		},
	}

	return
}
