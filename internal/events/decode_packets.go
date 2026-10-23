package events

import (
	"encoding/binary"
	"fmt"
	"github.com/z46-dev/wows-replay-parser"
	"math"
	"unicode/utf8"
)

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
