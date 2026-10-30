package replayparser

import (
	"context"
	"fmt"
)

// Vector3 is a world-space position or direction.
type Vector3 struct {
	X float32 `json:"x"`
	Y float32 `json:"y"`
	Z float32 `json:"z"`
}

// Rotation3 is yaw, pitch, and roll.
type Rotation3 struct {
	Yaw   float32 `json:"yaw"`
	Pitch float32 `json:"pitch"`
	Roll  float32 `json:"roll"`
}

// Event is one decoded packet; Raw borrows the replay's packet buffer.
type Event struct {
	ID         uint32  `json:"id"`
	Type       string  `json:"type"`
	Clock      float32 `json:"clock"`
	EntityID   uint32  `json:"entityId,omitempty"`
	EntityType string  `json:"entityType,omitempty"`
	Name       string  `json:"name,omitempty"`
	Value      any     `json:"value,omitempty"`
	Args       []any   `json:"args,omitempty"`
	Diagnostic string  `json:"diagnostic,omitempty"`
	Raw        []byte  `json:"-"`
	Leftover   []byte  `json:"-"`
}

// PositionValue is Toolkit's entity position packet.
type PositionValue struct {
	SpaceID    uint32    `json:"spaceId"`
	Position   Vector3   `json:"position"`
	Direction  Vector3   `json:"direction"`
	Rotation   Rotation3 `json:"rotation"`
	IsOnGround bool      `json:"isOnGround"`
}

// OrientationValue is a player or camera-linked position update.
type OrientationValue struct {
	ParentID uint32    `json:"parentId"`
	Position Vector3   `json:"position"`
	Rotation Rotation3 `json:"rotation"`
}

// WeaponLockValue is the weapon, lock mode, and target ID triplet.
type WeaponLockValue struct {
	WeaponType uint32 `json:"weaponType"`
	LockType   uint32 `json:"lockType"`
	TargetID   uint32 `json:"targetId"`
}

// GunMarkerValue contains aiming marker and artillery parameters.
type GunMarkerValue struct {
	TargetPoint      Vector3    `json:"targetPoint"`
	Diameter         float32    `json:"diameter"`
	MarkerPosition   Vector3    `json:"markerPosition"`
	MarkerDirection  Vector3    `json:"markerDirection"`
	ArcadeMarkerSize float32    `json:"arcadeMarkerSize"`
	SPGParameters    [2]float32 `json:"spgParameters"`
}

// EntityCreateValue describes an entity's initial world position and properties.
type EntityCreateValue struct {
	SpaceID     uint32         `json:"spaceId"`
	VehicleID   uint32         `json:"vehicleId"`
	Position    Vector3        `json:"position"`
	Rotation    Rotation3      `json:"rotation"`
	StateLength uint32         `json:"stateLength"`
	Properties  map[string]any `json:"properties"`
	Components  []byte         `json:"components,omitempty"`
}

// BootstrapValue contains a player's initial properties and component bytes.
type BootstrapValue struct {
	Properties map[string]any `json:"properties"`
	Components []byte         `json:"components,omitempty"`
}

// EntityEnterValue describes an entity entering a space.
type EntityEnterValue struct {
	SpaceID   uint32 `json:"spaceId"`
	VehicleID uint32 `json:"vehicleId"`
}

// NestedUpdateValue identifies one Toolkit property path and its change.
type NestedUpdateValue struct {
	Levels []any  `json:"levels,omitempty"`
	Action string `json:"action"`
	Key    string `json:"key,omitempty"`
	Index  int    `json:"index"`
	Start  int    `json:"start"`
	Stop   int    `json:"stop"`
	Value  any    `json:"value,omitempty"`
}

// CameraValue contains Toolkit's recorded camera state.
type CameraValue struct {
	Quaternion [4]float32 `json:"quaternion"`
	Camera     Vector3    `json:"camera"`
	FOV        float32    `json:"fov"`
	Unknown    float32    `json:"unknown"`
	Position   Vector3    `json:"position"`
	Direction  Vector3    `json:"direction"`
}

// MapValue contains a map packet's space and arena identifiers.
type MapValue struct {
	SpaceID uint32 `json:"spaceId"`
	ArenaID int64  `json:"arenaId"`
	Name    string `json:"name"`
	Blob    []byte `json:"-"`
	Matrix  []byte `json:"-"`
}

// WalkEvents decodes packets using a matching, caller-owned game-data archive.
func (replay *Replay) WalkEvents(ctx context.Context, data *GameData, visit func(Event) error) (err error) {
	if replay == nil || data == nil || ctx == nil || visit == nil {
		err = fmt.Errorf("replay, game data, context, and event visitor are required")
		return
	}

	if err = data.ValidateVersion(replay); err != nil {
		return
	}

	var specs []EntitySpec
	if specs, err = data.EntitySpecs(); err != nil {
		return
	}

	var (
		decoder *packetDecoder = &packetDecoder{specs: specs, entities: make(map[uint32]uint16), properties: make(map[uint32][]any)}
		index   int
	)

	err = replay.WalkPackets(ctx, func(packet Packet) (visitErr error) {
		var event Event
		if event, visitErr = decoder.decode(packet); visitErr != nil {
			visitErr = fmt.Errorf("packet %d (%s): %w", index, packet.Type, visitErr)
			return
		}

		index++
		visitErr = visit(event)
		return
	})

	return
}
