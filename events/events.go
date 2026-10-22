// Package events decodes timeline events from supported World of Warships replay packets.
package events

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"

	replayparser "github.com/z46-dev/wows-replay-parser"
)

const (
	positionPacketType       uint32 = 10
	entityCreatePacketType   uint32 = 5
	entityControlPacketType  uint32 = 2
	entityLeavePacketType    uint32 = 4
	entityPropertyPacketType uint32 = 7
	entityMethodPacketType   uint32 = 8
	ownedShipPacketType      uint32 = 32
	playerOrientationType    uint32 = 44
	battleResultsPacketType  uint32 = 34
	nestedPropertyPacketType uint32 = 35
	nonVolatilePacketType    uint32 = 42
	weaponLockPacketType     uint32 = 48
	positionPayloadSize             = 45
	orientationPayloadSize          = 32
	entityCreateHeaderSize          = 42
	entityMemberHeaderSize          = 12
	nestedPropertyHeaderSize        = 9
	nonVolatilePayloadSize          = 32
	weaponLockPayloadSize           = 12
)

type (
	// Type identifies a decoded timeline event category.
	Type string

	// Position records an entity's world-space movement update.
	Position struct {
		X          float32 `json:"x"`
		Y          float32 `json:"y"`
		Z          float32 `json:"z"`
		DirectionX float32 `json:"directionX"`
		DirectionY float32 `json:"directionY"`
		DirectionZ float32 `json:"directionZ"`
		Yaw        float32 `json:"yaw"`
		Pitch      float32 `json:"pitch"`
		Roll       float32 `json:"roll"`
		IsOnGround bool    `json:"isOnGround"`
	}

	// BattleResults preserves the verified JSON battle-results packet for later typed decoding.
	BattleResults struct {
		Raw json.RawMessage `json:"raw"`
	}

	// EntityCreate records the fixed entity-creation envelope before its type-specific state.
	EntityCreate struct {
		EntityTypeID uint16    `json:"entityTypeID"`
		SpaceID      uint32    `json:"spaceID"`
		VehicleID    uint32    `json:"vehicleID"`
		Position     *Position `json:"position"`
		StateLength  uint32    `json:"stateLength"`
		StateSize    uint32    `json:"stateSize"`
	}

	// EntityMember records the stable envelope around a type-specific property or method payload.
	EntityMember struct {
		MemberID    uint32 `json:"memberID"`
		PayloadSize uint32 `json:"payloadSize"`
		IsSlice     *bool  `json:"isSlice,omitempty"`
	}

	// WeaponLock records the player's target lock state for one weapon type.
	WeaponLock struct {
		WeaponType uint32 `json:"weaponType"`
		LockType   uint32 `json:"lockType"`
		TargetID   uint32 `json:"targetID"`
	}

	// Event is one decoded replay event at a game clock value.
	Event struct {
		Clock      float32        `json:"clock"`
		Type       Type           `json:"type"`
		EntityID   uint32         `json:"entityID,omitempty"`
		ParentID   uint32         `json:"parentID,omitempty"`
		Controlled *bool          `json:"controlled,omitempty"`
		Position   *Position      `json:"position,omitempty"`
		Created    *EntityCreate  `json:"created,omitempty"`
		Member     *EntityMember  `json:"member,omitempty"`
		WeaponLock *WeaponLock    `json:"weaponLock,omitempty"`
		Results    *BattleResults `json:"results,omitempty"`
	}

	// Timeline contains decoded events and packet types that do not yet have event decoders.
	Timeline struct {
		Events         []Event        `json:"events"`
		OwnShipID      uint32         `json:"ownShipID,omitempty"`
		UnknownPackets map[uint32]int `json:"unknownPackets,omitempty"`
	}
)

const (
	// PositionEvent identifies entity movement updates.
	PositionEvent Type = "position"

	// EntityCreateEvent identifies an entity's initial replay envelope.
	EntityCreateEvent Type = "entity_create"

	// EntityLeaveEvent identifies an entity leaving the replay space.
	EntityLeaveEvent Type = "entity_leave"

	// EntityPropertyEvent identifies a type-specific property update envelope.
	EntityPropertyEvent Type = "entity_property"

	// EntityMethodEvent identifies a type-specific method invocation envelope.
	EntityMethodEvent Type = "entity_method"

	// NestedPropertyEvent identifies a type-specific nested property update envelope.
	NestedPropertyEvent Type = "nested_property_update"

	// NonVolatilePositionEvent identifies an entity transform update without a direction vector.
	NonVolatilePositionEvent Type = "nonvolatile_position"

	// WeaponLockEvent identifies a weapon system target-lock change.
	WeaponLockEvent Type = "weapon_lock"

	// EntityControlEvent identifies a client entity-control change.
	EntityControlEvent Type = "entity_control"

	// OwnShipEvent identifies the ship controlled by the replaying player.
	OwnShipEvent Type = "own_ship"

	// PlayerOrientationEvent identifies a replay-player or camera transform update.
	PlayerOrientationEvent Type = "player_orientation"

	// BattleResultsEvent identifies the replay's final results payload.
	BattleResultsEvent Type = "battle_results"
)

// Decode converts supported packet payloads into a timeline without discarding unknown packets.
func Decode(replay *replayparser.Replay) (timeline *Timeline, err error) {
	timeline = &Timeline{
		Events:         make([]Event, 0),
		UnknownPackets: make(map[uint32]int),
	}

	for index, packet := range replay.Packets {
		var event Event

		switch packet.Type {
		case entityLeavePacketType:
			if event, err = decodeEntityLeave(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case entityCreatePacketType:
			if event, err = decodeEntityCreate(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case entityPropertyPacketType:
			if event, err = decodeEntityMember(packet, EntityPropertyEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case entityMethodPacketType:
			if event, err = decodeEntityMember(packet, EntityMethodEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case entityControlPacketType:
			if event, err = decodeEntityControl(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case positionPacketType:
			if event, err = decodePosition(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case ownedShipPacketType:
			if event, err = decodeOwnShip(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.OwnShipID = event.EntityID
			timeline.Events = append(timeline.Events, event)
		case playerOrientationType:
			if event, err = decodePlayerOrientation(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case battleResultsPacketType:
			if event, err = decodeBattleResults(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case nestedPropertyPacketType:
			if event, err = decodeNestedProperty(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case nonVolatilePacketType:
			if event, err = decodeNonVolatilePosition(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case weaponLockPacketType:
			if event, err = decodeWeaponLock(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		default:
			timeline.UnknownPackets[packet.Type]++
		}
	}

	return
}

// decodeEntityMember decodes the common fixed envelope around entity property and method payloads.
func decodeEntityMember(packet replayparser.Packet, eventType Type) (event Event, err error) {
	if len(packet.Payload) < entityMemberHeaderSize {
		err = fmt.Errorf("%s packet has %d-byte payload, expected at least %d", eventType, len(packet.Payload), entityMemberHeaderSize)
		return
	}

	var payload []byte = packet.Payload
	var payloadSize uint32 = binary.LittleEndian.Uint32(payload[8:12])
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
		},
	}

	return
}

// decodeNestedProperty decodes the stable nested-property envelope before its type-specific payload.
func decodeNestedProperty(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) < nestedPropertyHeaderSize {
		err = fmt.Errorf("nested-property packet has %d-byte payload, expected at least %d", len(packet.Payload), nestedPropertyHeaderSize)
		return
	}

	var payload []byte = packet.Payload
	var flags byte = payload[4]
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

// decodeWeaponLock decodes a weapon-system target-lock change.
func decodeWeaponLock(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != weaponLockPayloadSize {
		err = fmt.Errorf("weapon-lock packet has %d-byte payload, expected %d", len(packet.Payload), weaponLockPayloadSize)
		return
	}

	var payload []byte = packet.Payload
	event = Event{
		Clock: packet.Clock,
		Type:  WeaponLockEvent,
		WeaponLock: &WeaponLock{
			WeaponType: binary.LittleEndian.Uint32(payload[0:4]),
			LockType:   binary.LittleEndian.Uint32(payload[4:8]),
			TargetID:   binary.LittleEndian.Uint32(payload[8:12]),
		},
	}

	return
}

// Filter returns timeline events that meet every supplied filter.
func Filter(events []Event, eventType Type, entityID uint32) (filtered []Event) {
	filtered = make([]Event, 0, len(events))

	for _, event := range events {
		if eventType != "" && event.Type != eventType {
			continue
		}

		if entityID != 0 && event.EntityID != entityID {
			continue
		}

		filtered = append(filtered, event)
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

// decodeBattleResults verifies and retains the packet's length-prefixed JSON payload.
func decodeBattleResults(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) < 4 {
		err = fmt.Errorf("battle-results packet has %d-byte payload, expected length prefix", len(packet.Payload))
		return
	}

	var data []byte = packet.Payload[4:]
	if uint64(binary.LittleEndian.Uint32(packet.Payload[0:4])) != uint64(len(data)) {
		err = fmt.Errorf("battle-results JSON length does not match payload")
		return
	}

	if !json.Valid(data) {
		err = fmt.Errorf("battle-results packet is not valid JSON")
		return
	}

	event = Event{
		Clock: packet.Clock,
		Type:  BattleResultsEvent,
		Results: &BattleResults{
			Raw: append(json.RawMessage(nil), data...),
		},
	}

	return
}
