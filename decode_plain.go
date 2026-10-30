package replayparser

import (
	"encoding/binary"
	"fmt"
	"unicode/utf8"
)

// decodePlain handles Toolkit packet types that need no entity definition.
func (decoder *packetDecoder) decodePlain(event *Event, reader *wireReader) (err error) {
	switch event.Type {
	case "position", "non_volatile_position":
		var value PositionValue
		if event.EntityID, err = reader.uint32(); err != nil {
			return
		}

		if value.SpaceID, err = reader.uint32(); err != nil {
			return
		}

		if value.Position, err = reader.vector3(); err != nil {
			return
		}

		if event.Type == "position" {
			if value.Direction, err = reader.vector3(); err != nil {
				return
			}
		}

		if value.Rotation, err = reader.rotation3(); err != nil {
			return
		}

		if event.Type == "position" {
			var flag uint8
			if flag, err = reader.uint8(); err != nil {
				return
			}

			value.IsOnGround = flag != 0
		}

		event.Value = value
	case "player_orientation":
		if reader.remaining() != 32 {
			err = fmt.Errorf("player orientation is %d bytes, expected 32", reader.remaining())
			return
		}

		var value OrientationValue
		if event.EntityID, err = reader.uint32(); err != nil {
			return
		}

		if value.ParentID, err = reader.uint32(); err != nil {
			return
		}

		if value.Position, err = reader.vector3(); err != nil {
			return
		}

		value.Rotation, err = reader.rotation3()
		event.Value = value
	case "camera":
		var (
			value      CameraValue
			hasUnknown bool = reader.remaining() >= 60
		)

		for index := range value.Quaternion {
			if value.Quaternion[index], err = reader.float32(); err != nil {
				return
			}
		}

		if value.Camera, err = reader.vector3(); err != nil {
			return
		}

		if value.FOV, err = reader.float32(); err != nil {
			return
		}

		value.Unknown = -1
		if hasUnknown {
			if value.Unknown, err = reader.float32(); err != nil {
				return
			}
		}

		if value.Position, err = reader.vector3(); err != nil {
			return
		}

		value.Direction, err = reader.vector3()
		event.Value = value
	case "camera_mode":
		event.Value, err = reader.uint32()
	case "camera_free_look", "init_flag":
		event.Value, err = reader.uint8()
	case "player_net_stats":
		var raw []byte
		if raw, err = reader.take(4); err != nil {
			return
		}

		event.Value = map[string]any{"fps": raw[0], "ping": binary.LittleEndian.Uint16(raw[1:3]), "isLagging": raw[3]&1 != 0}
	case "server_tick", "server_timestamp":
		event.Value, err = reader.float64()
	case "own_ship":
		event.EntityID, err = reader.uint32()
	case "battle_results", "version":
		var length uint32
		if length, err = reader.uint32(); err != nil {
			return
		}

		if event.Type == "battle_results" && int64(length) != int64(reader.remaining()) {
			err = fmt.Errorf("battle results length %d differs from %d remaining bytes", length, reader.remaining())
			return
		}

		var text []byte
		if text, err = reader.takeSized(length); err != nil {
			return
		}

		if !utf8.Valid(text) {
			err = fmt.Errorf("%s contains invalid UTF-8", event.Type)
			return
		}

		event.Value = string(text)
	case "map":
		var value MapValue
		if value.SpaceID, err = reader.uint32(); err != nil {
			return
		}

		if value.ArenaID, err = reader.signed(8); err != nil {
			return
		}

		if _, err = reader.uint32(); err != nil {
			return
		}

		if _, err = reader.uint32(); err != nil {
			return
		}

		if value.Blob, err = reader.take(128); err != nil {
			return
		}

		var length uint32
		if length, err = reader.uint32(); err != nil {
			return
		}

		var name []byte
		if name, err = reader.takeSized(length); err != nil {
			return
		}

		if !utf8.Valid(name) {
			err = fmt.Errorf("map name contains invalid UTF-8")
			return
		}

		value.Name = string(name)
		if value.Matrix, err = reader.take(64); err != nil {
			return
		}

		if _, err = reader.uint8(); err != nil {
			return
		}

		event.Value = value
	case "set_weapon_lock":
		var value WeaponLockValue
		if value.WeaponType, err = reader.uint32(); err != nil {
			return
		}

		if value.LockType, err = reader.uint32(); err != nil {
			return
		}

		value.TargetID, err = reader.uint32()
		event.Value = value
	case "sub_controller":
		event.Value, err = reader.signed(2)
	case "cruise_state":
		var (
			key   uint32
			value int64
		)

		if key, err = reader.uint32(); err != nil {
			return
		}

		if value, err = reader.signed(4); err != nil {
			return
		}

		event.Value = map[string]any{"key": key, "value": value}
	case "shot_tracking":
		if event.EntityID, err = reader.uint32(); err != nil {
			return
		}

		event.Value, err = reader.signed(8)
	case "gun_marker":
		var value GunMarkerValue
		if value.TargetPoint, err = reader.vector3(); err != nil {
			return
		}

		if value.Diameter, err = reader.float32(); err != nil {
			return
		}

		if value.MarkerPosition, err = reader.vector3(); err != nil {
			return
		}

		if value.MarkerDirection, err = reader.vector3(); err != nil {
			return
		}

		if value.ArcadeMarkerSize, err = reader.float32(); err != nil {
			return
		}

		if value.SPGParameters[0], err = reader.float32(); err != nil {
			return
		}

		value.SPGParameters[1], err = reader.float32()
		event.Value = value
	case "init_marker":
		reader.offset = len(reader.data)
	}

	return
}
