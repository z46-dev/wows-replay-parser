// Package events decodes timeline events from supported World of Warships replay packets.
package events

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"unicode/utf8"

	replayparser "github.com/z46-dev/wows-replay-parser"
)

const (
	basePlayerCreatePacketType uint32 = 0
	cellPlayerCreatePacketType uint32 = 1
	positionPacketType         uint32 = 10
	entityEnterPacketType      uint32 = 3
	entityCreatePacketType     uint32 = 5
	entityControlPacketType    uint32 = 2
	entityLeavePacketType      uint32 = 4
	entityPropertyPacketType   uint32 = 7
	entityMethodPacketType     uint32 = 8
	serverTickPacketType       uint32 = 14
	serverTimestampPacketType  uint32 = 15
	initFlagPacketType         uint32 = 16
	initMarkerPacketType       uint32 = 19
	versionPacketType          uint32 = 22
	gunMarkerPacketType        uint32 = 24
	playerNetStatsPacketType   uint32 = 29
	ownedShipPacketType        uint32 = 32
	playerOrientationType      uint32 = 44
	battleResultsPacketType    uint32 = 34
	nestedPropertyPacketType   uint32 = 35
	nonVolatilePacketType      uint32 = 42
	cameraPacketType           uint32 = 37
	cameraModePacketType       uint32 = 39
	mapPacketType              uint32 = 40
	legacyControllerPacketType uint32 = 46
	cameraFreeLookPacketType   uint32 = 47
	weaponLockPacketType       uint32 = 48
	subControllerPacketType    uint32 = 49
	cruiseStatePacketType      uint32 = 50
	shotTrackingPacketType     uint32 = 51
	basePlayerCreateStubType   uint32 = 38
	replayMarkerPacketType     uint32 = math.MaxUint32
	basePlayerHeaderSize              = 10
	cellPlayerHeaderSize              = 40
	entityEnterPayloadSize            = 12
	mapPacketHeaderSize               = 152
	mapPacketTrailerSize              = 65
	positionPayloadSize               = 45
	orientationPayloadSize            = 32
	entityCreateHeaderSize            = 42
	entityMemberHeaderSize            = 12
	nestedPropertyHeaderSize          = 9
	nonVolatilePayloadSize            = 32
	weaponLockPayloadSize             = 12
	gunMarkerPayloadSize              = 52
	cameraPayloadSize                 = 60
	playerNetStatsPayloadSize         = 4
	float64PayloadSize                = 8
	cameraModePayloadSize             = 4
	cruiseStatePayloadSize            = 8
	shotTrackingPayloadSize           = 12
	cameraFreeLookPayloadSize         = 1
	subControllerPayloadSize          = 2
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
		Raw     json.RawMessage `json:"raw"`
		Summary *BattleSummary  `json:"summary,omitempty"`
	}

	// BattleSummary records stable post-battle match and player statistics.
	BattleSummary struct {
		ArenaUniqueID uint64                        `json:"arenaUniqueID"`
		AccountDBID   uint64                        `json:"accountDBID"`
		Common        BattleCommon                  `json:"common"`
		Players       map[uint64]BattlePlayerResult `json:"players"`
	}

	// BattleCommon records the stable common-list fields shared by every player result.
	BattleCommon struct {
		ArenaID       uint64 `json:"arenaID"`
		WinnerTeamID  uint64 `json:"winnerTeamID"`
		WinTypeID     uint64 `json:"winTypeID"`
		Duration      uint64 `json:"duration"`
		MapTypeID     uint64 `json:"mapTypeID"`
		ScenarioName  string `json:"scenarioName"`
		GameMode      uint64 `json:"gameMode"`
		ClusterID     uint64 `json:"clusterID"`
		StartUnixTime uint64 `json:"startUnixTime"`
	}

	// BattlePlayerResult records stable public post-battle statistics for one account.
	BattlePlayerResult struct {
		AccountDBID     uint64  `json:"accountDBID"`
		Name            string  `json:"name"`
		ClanTag         string  `json:"clanTag"`
		TeamID          uint64  `json:"teamID"`
		VehicleTypeID   uint64  `json:"vehicleTypeID"`
		MaxHealth       float64 `json:"maxHealth"`
		RemainingHealth float64 `json:"remainingHealth"`
		IsAlive         bool    `json:"isAlive"`
		LifetimeSeconds float64 `json:"lifetimeSeconds"`
		Distance        float64 `json:"distance"`
		CapturePoints   float64 `json:"capturePoints"`
		DefencePoints   float64 `json:"defencePoints"`
		ShipsKilled     float64 `json:"shipsKilled"`
		Damage          float64 `json:"damage"`
		SpottingDamage  float64 `json:"spottingDamage"`
		KillerAccountID uint64  `json:"killerAccountID"`
		KillerVehicleID uint64  `json:"killerVehicleID"`
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

	// EntityBootstrap records the base or cell state supplied before an entity enters the world.
	EntityBootstrap struct {
		EntityTypeID uint16    `json:"entityTypeID,omitempty"`
		SpaceID      uint32    `json:"spaceID,omitempty"`
		VehicleID    uint32    `json:"vehicleID,omitempty"`
		Position     *Position `json:"position,omitempty"`
		StateSize    uint32    `json:"stateSize"`
		State        []byte    `json:"state,omitempty"`
	}

	// MapState records the map metadata and fixed map-transform data for the replay space.
	MapState struct {
		SpaceID   uint32 `json:"spaceID"`
		ArenaID   int64  `json:"arenaID"`
		Unknown1  uint32 `json:"unknown1"`
		Unknown2  uint32 `json:"unknown2"`
		Blob      []byte `json:"blob"`
		Name      string `json:"name"`
		Matrix    []byte `json:"matrix"`
		IsUnknown bool   `json:"isUnknown"`
	}

	// EntityMember records the stable envelope around a type-specific property or method payload.
	EntityMember struct {
		MemberID    uint32 `json:"memberID"`
		PayloadSize uint32 `json:"payloadSize"`
		IsSlice     *bool  `json:"isSlice,omitempty"`
		Payload     []byte `json:"payload,omitempty"`
	}

	// WeaponLock records the player's target lock state for one weapon type.
	WeaponLock struct {
		WeaponType uint32 `json:"weaponType"`
		LockType   uint32 `json:"lockType"`
		TargetID   uint32 `json:"targetID"`
	}

	// GunMarker records the player's aiming marker state.
	GunMarker struct {
		Target      Position `json:"target"`
		Diameter    float32  `json:"diameter"`
		Marker      Position `json:"marker"`
		Direction   Position `json:"direction"`
		ArcadeSize  float32  `json:"arcadeSize"`
		SPGParamOne float32  `json:"spgParamOne"`
		SPGParamTwo float32  `json:"spgParamTwo"`
	}

	// Camera records one full camera transform and view state.
	Camera struct {
		RotationX float32  `json:"rotationX"`
		RotationY float32  `json:"rotationY"`
		RotationZ float32  `json:"rotationZ"`
		RotationW float32  `json:"rotationW"`
		Position  Position `json:"position"`
		FOV       float32  `json:"fov"`
		Unknown   float32  `json:"unknown"`
		Player    Position `json:"player"`
		Direction Position `json:"direction"`
	}

	// PlayerNetStats records the client network state captured by the replay.
	PlayerNetStats struct {
		FPS       uint8  `json:"fps"`
		Ping      uint16 `json:"ping"`
		IsLagging bool   `json:"isLagging"`
	}

	// Scalar records a typed protocol scalar value.
	Scalar struct {
		Integer *int64   `json:"integer,omitempty"`
		Number  *float64 `json:"number,omitempty"`
		Text    string   `json:"text,omitempty"`
	}

	// CruiseState records a cruise-control key and value update.
	CruiseState struct {
		Key   uint32 `json:"key"`
		Value int32  `json:"value"`
	}

	// ShipHealth records a vehicle health update and its change from the prior update.
	ShipHealth struct {
		Current  float32 `json:"current"`
		Previous float32 `json:"previous,omitempty"`
		Delta    float32 `json:"delta"`
	}

	// Event is one decoded replay event at a game clock value.
	Event struct {
		Clock      float32          `json:"clock"`
		Type       Type             `json:"type"`
		EntityID   uint32           `json:"entityID,omitempty"`
		ParentID   uint32           `json:"parentID,omitempty"`
		Controlled *bool            `json:"controlled,omitempty"`
		Position   *Position        `json:"position,omitempty"`
		Created    *EntityCreate    `json:"created,omitempty"`
		Bootstrap  *EntityBootstrap `json:"bootstrap,omitempty"`
		Member     *EntityMember    `json:"member,omitempty"`
		Map        *MapState        `json:"map,omitempty"`
		Payload    []byte           `json:"payload,omitempty"`
		WeaponLock *WeaponLock      `json:"weaponLock,omitempty"`
		GunMarker  *GunMarker       `json:"gunMarker,omitempty"`
		Camera     *Camera          `json:"camera,omitempty"`
		NetStats   *PlayerNetStats  `json:"netStats,omitempty"`
		Scalar     *Scalar          `json:"scalar,omitempty"`
		Cruise     *CruiseState     `json:"cruise,omitempty"`
		Health     *ShipHealth      `json:"health,omitempty"`
		Results    *BattleResults   `json:"results,omitempty"`
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

	// BasePlayerCreateEvent identifies an entity's base property bootstrap envelope.
	BasePlayerCreateEvent Type = "base_player_create"

	// BasePlayerCreateStubEvent identifies a base bootstrap with no decoded properties.
	BasePlayerCreateStubEvent Type = "base_player_create_stub"

	// CellPlayerCreateEvent identifies an entity's cell property bootstrap envelope.
	CellPlayerCreateEvent Type = "cell_player_create"

	// EntityEnterEvent identifies an entity entering a replay space.
	EntityEnterEvent Type = "entity_enter"

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

	// GunMarkerEvent identifies a player aiming marker update.
	GunMarkerEvent Type = "gun_marker"

	// CameraEvent identifies a full camera transform update.
	CameraEvent Type = "camera"

	// PlayerNetStatsEvent identifies a captured client network state.
	PlayerNetStatsEvent Type = "player_net_stats"

	// ServerTickEvent identifies a captured server tick-rate update.
	ServerTickEvent Type = "server_tick"

	// ServerTimestampEvent identifies the replay's server timestamp.
	ServerTimestampEvent Type = "server_timestamp"

	// InitFlagEvent identifies the replay initialization flag.
	InitFlagEvent Type = "init_flag"

	// InitMarkerEvent identifies the empty replay initialization marker.
	InitMarkerEvent Type = "init_marker"

	// VersionEvent identifies the version string packet.
	VersionEvent Type = "version"

	// CameraModeEvent identifies the active camera mode.
	CameraModeEvent Type = "camera_mode"

	// MapEvent identifies map metadata for the replay space.
	MapEvent Type = "map"

	// CameraFreeLookEvent identifies a camera free-look state change.
	CameraFreeLookEvent Type = "camera_free_look"

	// LegacyControllerEvent identifies a fixed-width legacy controller packet with opaque state.
	LegacyControllerEvent Type = "legacy_controller"

	// SubControllerEvent identifies a submarine controller state change.
	SubControllerEvent Type = "sub_controller"

	// CruiseStateEvent identifies a cruise-control update.
	CruiseStateEvent Type = "cruise_state"

	// ShotTrackingEvent identifies a player ship's shot-tracking update.
	ShotTrackingEvent Type = "shot_tracking"

	// ShipHealthEvent identifies a verified vehicle health property update.
	ShipHealthEvent Type = "ship_health"

	// ShipDamageEvent identifies a verified negative vehicle health delta.
	ShipDamageEvent Type = "ship_damage"

	// ShipRepairEvent identifies a verified positive vehicle health delta.
	ShipRepairEvent Type = "ship_repair"

	// EntityControlEvent identifies a client entity-control change.
	EntityControlEvent Type = "entity_control"

	// OwnShipEvent identifies the ship controlled by the replaying player.
	OwnShipEvent Type = "own_ship"

	// PlayerOrientationEvent identifies a replay-player or camera transform update.
	PlayerOrientationEvent Type = "player_orientation"

	// BattleResultsEvent identifies the replay's final results payload.
	BattleResultsEvent Type = "battle_results"

	// ReplayMarkerEvent identifies an untyped replay stream marker.
	ReplayMarkerEvent Type = "replay_marker"
)

// Decode converts supported packet payloads into a timeline without discarding unknown packets.
func Decode(replay *replayparser.Replay) (timeline *Timeline, err error) {
	timeline = &Timeline{
		Events:         make([]Event, 0),
		UnknownPackets: make(map[uint32]int),
	}

	var (
		entityTypes map[uint32]uint16  = make(map[uint32]uint16)
		shipHealth  map[uint32]float32 = make(map[uint32]float32)
	)

	for index, packet := range replay.Packets {
		var event Event

		switch packet.Type {
		case basePlayerCreatePacketType:
			if event, err = decodeBasePlayerCreate(packet, BasePlayerCreateEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case basePlayerCreateStubType:
			if event, err = decodeBasePlayerCreate(packet, BasePlayerCreateStubEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case cellPlayerCreatePacketType:
			if event, err = decodeCellPlayerCreate(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case entityEnterPacketType:
			if event, err = decodeEntityEnter(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case serverTickPacketType:
			if event, err = decodeFloat64(packet, ServerTickEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case serverTimestampPacketType:
			if event, err = decodeFloat64(packet, ServerTimestampEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case initFlagPacketType:
			if event, err = decodeInitFlag(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case initMarkerPacketType:
			if event, err = decodeInitMarker(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case versionPacketType:
			if event, err = decodeVersion(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case gunMarkerPacketType:
			if event, err = decodeGunMarker(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case playerNetStatsPacketType:
			if event, err = decodePlayerNetStats(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
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

			entityTypes[event.EntityID] = event.Created.EntityTypeID
			timeline.Events = append(timeline.Events, event)
		case entityPropertyPacketType:
			if event, err = decodeEntityMember(packet, EntityPropertyEvent); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
			if entityTypes[event.EntityID] == 2 && event.Member.MemberID == 28 && event.Member.PayloadSize == 4 {
				var healthEvent Event
				if healthEvent, err = decodeShipHealth(packet, event.EntityID, shipHealth); err != nil {
					err = fmt.Errorf("packet %d: %w", index, err)
					return
				}

				timeline.Events = append(timeline.Events, healthEvent)
				if healthEvent.Health.Delta < 0 {
					healthEvent.Type = ShipDamageEvent
					timeline.Events = append(timeline.Events, healthEvent)
				} else if healthEvent.Health.Delta > 0 {
					healthEvent.Type = ShipRepairEvent
					timeline.Events = append(timeline.Events, healthEvent)
				}
			}
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
		case cameraPacketType:
			if event, err = decodeCamera(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case cameraModePacketType:
			if event, err = decodeCameraMode(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case mapPacketType:
			if event, err = decodeMap(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case legacyControllerPacketType:
			if event, err = decodeLegacyController(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case cameraFreeLookPacketType:
			if event, err = decodeCameraFreeLook(packet); err != nil {
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
		case subControllerPacketType:
			if event, err = decodeSubController(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case cruiseStatePacketType:
			if event, err = decodeCruiseState(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case shotTrackingPacketType:
			if event, err = decodeShotTracking(packet); err != nil {
				err = fmt.Errorf("packet %d: %w", index, err)
				return
			}

			timeline.Events = append(timeline.Events, event)
		case replayMarkerPacketType:
			event = Event{Clock: packet.Clock, Type: ReplayMarkerEvent, Payload: packet.Payload}
			timeline.Events = append(timeline.Events, event)
		default:
			timeline.UnknownPackets[packet.Type]++
		}
	}

	return
}

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

// decodeGunMarker decodes a fixed player aiming-marker packet.
func decodeGunMarker(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != gunMarkerPayloadSize {
		err = fmt.Errorf("gun-marker packet has %d-byte payload, expected %d", len(packet.Payload), gunMarkerPayloadSize)
		return
	}

	var payload []byte = packet.Payload
	event = Event{
		Clock: packet.Clock,
		Type:  GunMarkerEvent,
		GunMarker: &GunMarker{
			Target:      positionAt(payload, 0),
			Diameter:    float32At(payload, 12),
			Marker:      positionAt(payload, 16),
			Direction:   positionAt(payload, 28),
			ArcadeSize:  float32At(payload, 40),
			SPGParamOne: float32At(payload, 44),
			SPGParamTwo: float32At(payload, 48),
		},
	}

	return
}

// decodePlayerNetStats decodes the packed client FPS, ping, and lagging state.
func decodePlayerNetStats(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != playerNetStatsPayloadSize {
		err = fmt.Errorf("player-net-stats packet has %d-byte payload, expected %d", len(packet.Payload), playerNetStatsPayloadSize)
		return
	}

	var packed uint32 = binary.LittleEndian.Uint32(packet.Payload)
	event = Event{
		Clock: packet.Clock,
		Type:  PlayerNetStatsEvent,
		NetStats: &PlayerNetStats{
			FPS:       uint8(packed),
			Ping:      uint16(packed >> 8),
			IsLagging: packed&(1<<24) != 0,
		},
	}

	return
}

// decodeFloat64 decodes a fixed protocol float64 packet.
func decodeFloat64(packet replayparser.Packet, eventType Type) (event Event, err error) {
	if len(packet.Payload) != float64PayloadSize {
		err = fmt.Errorf("%s packet has %d-byte payload, expected %d", eventType, len(packet.Payload), float64PayloadSize)
		return
	}

	var number float64 = math.Float64frombits(binary.LittleEndian.Uint64(packet.Payload))
	event = Event{Clock: packet.Clock, Type: eventType, Scalar: &Scalar{Number: &number}}
	return
}

// decodeInitFlag decodes the fixed replay initialization flag.
func decodeInitFlag(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != 1 {
		err = fmt.Errorf("init-flag packet has %d-byte payload, expected 1", len(packet.Payload))
		return
	}

	var value int64 = int64(packet.Payload[0])
	event = Event{Clock: packet.Clock, Type: InitFlagEvent, Scalar: &Scalar{Integer: &value}}
	return
}

// decodeInitMarker validates and decodes the empty replay initialization marker.
func decodeInitMarker(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != 0 {
		err = fmt.Errorf("init-marker packet has %d-byte payload, expected 0", len(packet.Payload))
		return
	}

	event = Event{Clock: packet.Clock, Type: InitMarkerEvent}
	return
}

// decodeVersion decodes the length-prefixed replay version string.
func decodeVersion(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) < 4 {
		err = fmt.Errorf("version packet has %d-byte payload, expected length prefix", len(packet.Payload))
		return
	}

	var data []byte = packet.Payload[4:]
	if uint64(binary.LittleEndian.Uint32(packet.Payload[0:4])) != uint64(len(data)) {
		err = fmt.Errorf("version text length does not match payload")
		return
	}

	if !utf8.Valid(data) {
		err = fmt.Errorf("version packet is not valid UTF-8")
		return
	}

	event = Event{Clock: packet.Clock, Type: VersionEvent, Scalar: &Scalar{Text: string(data)}}
	return
}

// decodeCameraMode decodes the active camera mode identifier.
func decodeCameraMode(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != cameraModePayloadSize {
		err = fmt.Errorf("camera-mode packet has %d-byte payload, expected %d", len(packet.Payload), cameraModePayloadSize)
		return
	}

	var value int64 = int64(binary.LittleEndian.Uint32(packet.Payload))
	event = Event{Clock: packet.Clock, Type: CameraModeEvent, Scalar: &Scalar{Integer: &value}}
	return
}

// decodeMap decodes replay-space map metadata, retaining opaque fixed protocol fields.
func decodeMap(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) < mapPacketHeaderSize+mapPacketTrailerSize {
		err = fmt.Errorf("map packet has %d-byte payload, expected at least %d", len(packet.Payload), mapPacketHeaderSize+mapPacketTrailerSize)
		return
	}

	var (
		payload  []byte = packet.Payload
		nameSize uint32 = binary.LittleEndian.Uint32(payload[148:152])
		offset   uint64 = mapPacketHeaderSize + uint64(nameSize)
	)

	if offset+mapPacketTrailerSize != uint64(len(payload)) {
		err = fmt.Errorf("map name length does not match packet")
		return
	}

	var nameData []byte = payload[mapPacketHeaderSize:offset]
	if !utf8.Valid(nameData) {
		err = fmt.Errorf("map name is not valid UTF-8")
		return
	}

	event = Event{
		Clock: packet.Clock,
		Type:  MapEvent,
		Map: &MapState{
			SpaceID:   binary.LittleEndian.Uint32(payload[0:4]),
			ArenaID:   int64(binary.LittleEndian.Uint64(payload[4:12])),
			Unknown1:  binary.LittleEndian.Uint32(payload[12:16]),
			Unknown2:  binary.LittleEndian.Uint32(payload[16:20]),
			Blob:      payload[20:148],
			Name:      string(nameData),
			Matrix:    payload[offset : offset+64],
			IsUnknown: payload[offset+64] != 0,
		},
	}

	return
}

// decodeCameraFreeLook decodes the camera free-look state flag.
func decodeCameraFreeLook(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != cameraFreeLookPayloadSize {
		err = fmt.Errorf("camera-free-look packet has %d-byte payload, expected %d", len(packet.Payload), cameraFreeLookPayloadSize)
		return
	}

	var value int64 = int64(packet.Payload[0])
	event = Event{Clock: packet.Clock, Type: CameraFreeLookEvent, Scalar: &Scalar{Integer: &value}}
	return
}

// decodeLegacyController preserves the fixed-width controller payload used by 15.8 replays.
func decodeLegacyController(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != 8 {
		err = fmt.Errorf("legacy-controller packet has %d-byte payload, expected 8", len(packet.Payload))
		return
	}

	event = Event{Clock: packet.Clock, Type: LegacyControllerEvent, Payload: packet.Payload}
	return
}

// decodeSubController decodes the submarine controller mode.
func decodeSubController(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != subControllerPayloadSize {
		err = fmt.Errorf("sub-controller packet has %d-byte payload, expected %d", len(packet.Payload), subControllerPayloadSize)
		return
	}

	var value int64 = int64(int16(binary.LittleEndian.Uint16(packet.Payload)))
	event = Event{Clock: packet.Clock, Type: SubControllerEvent, Scalar: &Scalar{Integer: &value}}
	return
}

// decodeCruiseState decodes a cruise-control key and value update.
func decodeCruiseState(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != cruiseStatePayloadSize {
		err = fmt.Errorf("cruise-state packet has %d-byte payload, expected %d", len(packet.Payload), cruiseStatePayloadSize)
		return
	}

	event = Event{
		Clock:  packet.Clock,
		Type:   CruiseStateEvent,
		Cruise: &CruiseState{Key: binary.LittleEndian.Uint32(packet.Payload[0:4]), Value: int32(binary.LittleEndian.Uint32(packet.Payload[4:8]))},
	}

	return
}

// decodeShotTracking decodes a player ship shot-tracking value.
func decodeShotTracking(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != shotTrackingPayloadSize {
		err = fmt.Errorf("shot-tracking packet has %d-byte payload, expected %d", len(packet.Payload), shotTrackingPayloadSize)
		return
	}

	var value int64 = int64(binary.LittleEndian.Uint64(packet.Payload[4:12]))
	event = Event{Clock: packet.Clock, Type: ShotTrackingEvent, EntityID: binary.LittleEndian.Uint32(packet.Payload[0:4]), Scalar: &Scalar{Integer: &value}}
	return
}

// decodeCamera decodes a fixed full camera transform packet.
func decodeCamera(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) != cameraPayloadSize {
		err = fmt.Errorf("camera packet has %d-byte payload, expected %d", len(packet.Payload), cameraPayloadSize)
		return
	}

	var payload []byte = packet.Payload
	event = Event{
		Clock: packet.Clock,
		Type:  CameraEvent,
		Camera: &Camera{
			RotationX: float32At(payload, 0),
			RotationY: float32At(payload, 4),
			RotationZ: float32At(payload, 8),
			RotationW: float32At(payload, 12),
			Position:  positionAt(payload, 16),
			FOV:       float32At(payload, 28),
			Unknown:   float32At(payload, 32),
			Player:    positionAt(payload, 36),
			Direction: positionAt(payload, 48),
		},
	}

	return
}

// positionAt decodes one three-component position from a fixed little-endian payload.
func positionAt(payload []byte, offset int) (position Position) {
	position = Position{X: float32At(payload, offset), Y: float32At(payload, offset+4), Z: float32At(payload, offset+8)}
	return
}

// float32At decodes one float32 from a fixed little-endian payload.
func float32At(payload []byte, offset int) (number float32) {
	number = math.Float32frombits(binary.LittleEndian.Uint32(payload[offset : offset+4]))
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

	var summary *BattleSummary
	if summary, err = decodeBattleSummary(data); err != nil {
		return
	}

	event = Event{
		Clock: packet.Clock,
		Type:  BattleResultsEvent,
		Results: &BattleResults{
			Raw:     append(json.RawMessage(nil), data...),
			Summary: summary,
		},
	}

	return
}

// decodeBattleSummary extracts stable result fields while retaining the complete raw result object.
func decodeBattleSummary(data []byte) (summary *BattleSummary, err error) {
	var result struct {
		ArenaUniqueID     json.RawMessage              `json:"arenaUniqueID"`
		AccountDBID       json.RawMessage              `json:"accountDBID"`
		CommonList        []json.RawMessage            `json:"commonList"`
		PlayersPublicInfo map[string][]json.RawMessage `json:"playersPublicInfo"`
	}

	if err = json.Unmarshal(data, &result); err != nil {
		err = fmt.Errorf("battle-results JSON object is invalid: %w", err)
		return
	}

	summary = &BattleSummary{
		ArenaUniqueID: uintValue(result.ArenaUniqueID),
		AccountDBID:   uintValue(result.AccountDBID),
		Common:        decodeBattleCommon(result.CommonList),
		Players:       make(map[uint64]BattlePlayerResult, len(result.PlayersPublicInfo)),
	}

	for accountID, values := range result.PlayersPublicInfo {
		var player BattlePlayerResult = decodeBattlePlayer(values)
		if player.AccountDBID == 0 {
			player.AccountDBID = uintValue(json.RawMessage(accountID))
		}

		if player.AccountDBID != 0 {
			summary.Players[player.AccountDBID] = player
		}
	}

	return
}

// decodeBattleCommon maps the documented stable common-list indices.
func decodeBattleCommon(values []json.RawMessage) (common BattleCommon) {
	common = BattleCommon{
		ArenaID:       uintValue(valueAt(values, 0)),
		ClusterID:     uintValue(valueAt(values, 1)),
		StartUnixTime: uintValue(valueAt(values, 2)),
		WinnerTeamID:  uintValue(valueAt(values, 3)),
		WinTypeID:     uintValue(valueAt(values, 4)),
		Duration:      uintValue(valueAt(values, 8)),
		MapTypeID:     uintValue(valueAt(values, 9)),
		ScenarioName:  stringValue(valueAt(values, 10)),
		GameMode:      uintValue(valueAt(values, 12)),
	}

	return
}

// decodeBattlePlayer maps stable public-player result indices shared by supported 15.x builds.
func decodeBattlePlayer(values []json.RawMessage) (player BattlePlayerResult) {
	player = BattlePlayerResult{
		AccountDBID:     uintValue(valueAt(values, 0)),
		Name:            stringValue(valueAt(values, 1)),
		ClanTag:         stringValue(valueAt(values, 3)),
		TeamID:          uintValue(valueAt(values, 6)),
		VehicleTypeID:   uintValue(valueAt(values, 7)),
		MaxHealth:       floatValue(valueAt(values, 15)),
		RemainingHealth: floatValue(valueAt(values, 20)),
		IsAlive:         boolValue(valueAt(values, 21)),
		LifetimeSeconds: floatValue(valueAt(values, 22)),
		Distance:        floatValue(valueAt(values, 23)),
		CapturePoints:   floatValue(valueAt(values, 24)),
		DefencePoints:   floatValue(valueAt(values, 25)),
		ShipsKilled:     floatValue(valueAt(values, 32)),
		KillerAccountID: uintValue(valueAt(values, 408)),
		KillerVehicleID: uintValue(valueAt(values, 409)),
		SpottingDamage:  floatValue(valueAt(values, 412)),
		Damage:          floatValue(valueAt(values, 426)),
	}

	return
}

// valueAt returns an empty raw value when an optional results list is shorter than expected.
func valueAt(values []json.RawMessage, index int) (value json.RawMessage) {
	if index >= 0 && index < len(values) {
		value = values[index]
	}

	return
}

// uintValue reads an optional JSON numeric value without treating malformed optional data as fatal.
func uintValue(value json.RawMessage) (number uint64) {
	var parsed float64
	if json.Unmarshal(value, &parsed) == nil && parsed >= 0 {
		number = uint64(parsed)
	}

	return
}

// floatValue reads an optional JSON numeric value without treating malformed optional data as fatal.
func floatValue(value json.RawMessage) (number float64) {
	json.Unmarshal(value, &number)
	return
}

// boolValue reads an optional JSON boolean value without treating malformed optional data as fatal.
func boolValue(value json.RawMessage) (flag bool) {
	json.Unmarshal(value, &flag)
	return
}

// stringValue reads an optional JSON string value without treating malformed optional data as fatal.
func stringValue(value json.RawMessage) (text string) {
	json.Unmarshal(value, &text)
	return
}
