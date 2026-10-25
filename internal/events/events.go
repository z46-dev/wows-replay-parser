// Package events decodes timeline events from supported World of Warships replay packets.
package events

import (
	"encoding/json"
	"math"
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
		ShipID          string  `json:"shipID,omitempty"`
		ShipName        string  `json:"shipName,omitempty"`
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
		EntityTypeID         uint16         `json:"entityTypeID"`
		EntityType           string         `json:"entityType,omitempty"`
		SpaceID              uint32         `json:"spaceID"`
		VehicleID            uint32         `json:"vehicleID"`
		Position             *Position      `json:"position"`
		StateLength          uint32         `json:"stateLength"`
		StateSize            uint32         `json:"stateSize"`
		State                []byte         `json:"state,omitempty"`
		InitialProperties    map[string]any `json:"initialProperties,omitempty"`
		InitialStateComplete bool           `json:"initialStateComplete,omitempty"`
	}

	// EntityBootstrap records the base or cell state supplied before an entity enters the world.
	EntityBootstrap struct {
		EntityTypeID         uint16         `json:"entityTypeID,omitempty"`
		SpaceID              uint32         `json:"spaceID,omitempty"`
		VehicleID            uint32         `json:"vehicleID,omitempty"`
		Position             *Position      `json:"position,omitempty"`
		StateSize            uint32         `json:"stateSize"`
		State                []byte         `json:"state,omitempty"`
		InitialProperties    map[string]any `json:"initialProperties,omitempty"`
		InitialStateComplete bool           `json:"initialStateComplete,omitempty"`
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
		Name        string `json:"name,omitempty"`
		EntityType  string `json:"entityType,omitempty"`
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

	// ShipDeath records the victim and killer entity IDs from receiveVehicleDeath.
	ShipDeath struct {
		VictimID uint32 `json:"victimID"`
		KillerID uint32 `json:"killerID"`
		Cause    uint32 `json:"cause"`
	}

	// ShipDamageDealt records one server-attributed aggressor update on a victim ship.
	ShipDamageDealt struct {
		VictimID    uint32  `json:"victimID"`
		AggressorID uint32  `json:"aggressorID"`
		Damage      float32 `json:"damage"`
	}

	// SpottedEntities is the complete set of ships currently visible to the replay client.
	SpottedEntities struct {
		EntityIDs []uint32 `json:"entityIDs"`
	}

	// MinimapSquadron records a carrier aircraft marker and its minimap position.
	MinimapSquadron struct {
		PlaneID      int64   `json:"planeID"`
		TeamID       int8    `json:"teamID,omitempty"`
		GameParamsID uint32  `json:"gameParamsID,omitempty"`
		X            float32 `json:"x"`
		Z            float32 `json:"z"`
		Removed      bool    `json:"removed,omitempty"`
	}

	// MinimapShipPosition records a quantized minimap sighting from updateMinimapVisionInfo.
	MinimapShipPosition struct {
		VehicleID    uint32  `json:"vehicleID"`
		X            float32 `json:"x"`
		Z            float32 `json:"z"`
		Heading      float32 `json:"heading"`
		IsSentinel   bool    `json:"isSentinel,omitempty"`
		Disappearing bool    `json:"disappearing,omitempty"`
	}

	// TorpedoPosition records one server synchronization for an active torpedo.
	TorpedoPosition struct {
		OwnerID  uint32   `json:"ownerID"`
		ShotID   uint16   `json:"shotID"`
		Position Position `json:"position"`
	}

	// ArtilleryShot records one shell in a received salvo.
	ArtilleryShot struct {
		OwnerID        uint32   `json:"ownerID"`
		ParamsID       uint32   `json:"paramsID"`
		SalvoID        int32    `json:"salvoID"`
		ShotID         uint16   `json:"shotID"`
		Origin         Position `json:"origin"`
		Target         Position `json:"target"`
		Speed          float32  `json:"speed"`
		ServerTimeLeft float32  `json:"serverTimeLeft"`
	}

	// TorpedoLaunch records initial position and velocity for one launched torpedo.
	TorpedoLaunch struct {
		OwnerID   uint32   `json:"ownerID"`
		ShotID    uint16   `json:"shotID"`
		Position  Position `json:"position"`
		Direction Position `json:"direction"`
	}

	// TorpedoDirection records a heading and origin update for one torpedo.
	TorpedoDirection struct {
		OwnerID   uint32   `json:"ownerID"`
		ShotID    uint16   `json:"shotID"`
		Position  Position `json:"position"`
		TargetYaw float32  `json:"targetYaw"`
		SpeedCoef float32  `json:"speedCoef"`
	}

	// ProjectileHit identifies a shell or torpedo removed by a shot-kill update.
	ProjectileHit struct {
		OwnerID  uint32   `json:"ownerID"`
		ShotID   uint16   `json:"shotID"`
		Position Position `json:"position"`
	}

	// VictoryPointUpdate preserves the score total transmitted to the replay player's client.
	VictoryPointUpdate struct {
		TotalPoints float32 `json:"totalPoints"`
	}

	// TeamScoreUpdate records an authoritative score assigned to one battle team.
	TeamScoreUpdate struct {
		TeamIndex int64  `json:"teamIndex"`
		Score     uint16 `json:"score"`
	}

	// Event is one decoded replay event at a game clock value.
	Event struct {
		Clock            float32              `json:"clock"`
		Type             Type                 `json:"type"`
		EntityID         uint32               `json:"entityID,omitempty"`
		ParentID         uint32               `json:"parentID,omitempty"`
		Controlled       *bool                `json:"controlled,omitempty"`
		Position         *Position            `json:"position,omitempty"`
		Created          *EntityCreate        `json:"created,omitempty"`
		Bootstrap        *EntityBootstrap     `json:"bootstrap,omitempty"`
		Member           *EntityMember        `json:"member,omitempty"`
		Map              *MapState            `json:"map,omitempty"`
		Payload          []byte               `json:"payload,omitempty"`
		WeaponLock       *WeaponLock          `json:"weaponLock,omitempty"`
		GunMarker        *GunMarker           `json:"gunMarker,omitempty"`
		Camera           *Camera              `json:"camera,omitempty"`
		NetStats         *PlayerNetStats      `json:"netStats,omitempty"`
		Scalar           *Scalar              `json:"scalar,omitempty"`
		Cruise           *CruiseState         `json:"cruise,omitempty"`
		Health           *ShipHealth          `json:"health,omitempty"`
		Death            *ShipDeath           `json:"death,omitempty"`
		DamageDealt      *ShipDamageDealt     `json:"damageDealt,omitempty"`
		Visibility       *SpottedEntities     `json:"visibility,omitempty"`
		Squadron         *MinimapSquadron     `json:"squadron,omitempty"`
		Minimap          *MinimapShipPosition `json:"minimap,omitempty"`
		Torpedo          *TorpedoPosition     `json:"torpedo,omitempty"`
		Shell            *ArtilleryShot       `json:"shell,omitempty"`
		TorpedoLaunch    *TorpedoLaunch       `json:"torpedoLaunch,omitempty"`
		TorpedoDirection *TorpedoDirection    `json:"torpedoDirection,omitempty"`
		ProjectileHit    *ProjectileHit       `json:"projectileHit,omitempty"`
		VictoryPoints    *VictoryPointUpdate  `json:"victoryPoints,omitempty"`
		TeamScore        *TeamScoreUpdate     `json:"teamScore,omitempty"`
		Results          *BattleResults       `json:"results,omitempty"`
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

	// ShipDeathEvent identifies the replay's authoritative vehicle death notification.
	ShipDeathEvent Type = "ship_death"

	// ShipDamageDealtEvent identifies damage attributed to an aggressor by the game server.
	ShipDamageDealtEvent Type = "ship_damage_dealt"

	// VictoryPointsEvent identifies a score total sent to the player's client.
	VictoryPointsEvent Type = "victory_points"
	// TeamScoreEvent identifies an authoritative BattleLogic team score update.
	TeamScoreEvent Type = "team_score"

	// ShipVisibilityEvent identifies a complete update to the replay client's spotted ships.
	ShipVisibilityEvent Type = "ship_visibility"

	// MinimapSquadronEvent identifies an aircraft squadron entering the minimap.
	MinimapSquadronEvent Type = "minimap_squadron"

	// MinimapSquadronPositionEvent identifies a squadron minimap position update.
	MinimapSquadronPositionEvent Type = "minimap_squadron_position"

	// MinimapSquadronRemoveEvent identifies a squadron leaving the minimap.
	MinimapSquadronRemoveEvent Type = "minimap_squadron_remove"

	// MinimapShipPositionEvent identifies a ship's minimap-only position update.
	MinimapShipPositionEvent Type = "minimap_ship_position"

	// TorpedoPositionEvent identifies an authoritative torpedo position synchronization.
	TorpedoPositionEvent Type = "torpedo_position"

	// ArtilleryShotEvent identifies a newly fired shell.
	ArtilleryShotEvent Type = "artillery_shot"

	// TorpedoLaunchEvent identifies a newly launched torpedo.
	TorpedoLaunchEvent Type = "torpedo_launch"

	// TorpedoDirectionEvent identifies an updated torpedo trajectory.
	TorpedoDirectionEvent Type = "torpedo_direction"

	// ProjectileHitEvent identifies a shell/torpedo impact or removal.
	ProjectileHitEvent Type = "projectile_hit"

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
