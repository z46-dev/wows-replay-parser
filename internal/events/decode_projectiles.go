package events

import (
	"encoding/binary"
	"fmt"
	"math"
)

type projectileReader struct {
	data   []byte
	offset int
}

// decodeProjectileMethod decodes the 15.9 Avatar projectile RPC payloads used by rendering.
func decodeProjectileMethod(clock float32, member EntityMember) (events []Event, recognized bool, err error) {
	if member.EntityType != "Avatar" {
		return
	}

	var reader projectileReader = projectileReader{data: member.Payload}
	switch member.Name {
	case "receiveArtilleryShots":
		recognized = true
		events, err = decodeArtillerySalvos(clock, &reader)
	case "receiveTorpedoes":
		recognized = true
		events, err = decodeTorpedoSalvos(clock, &reader)
	case "receiveTorpedoDirection":
		recognized = true
		events, err = decodeTorpedoDirection(clock, &reader)
	case "receiveShotKills":
		recognized = true
		events, err = decodeProjectileHits(clock, &reader)
	}

	if err == nil && recognized && reader.offset != len(reader.data) {
		err = fmt.Errorf("%s payload has %d trailing bytes", member.Name, len(reader.data)-reader.offset)
	}
	if err != nil && recognized {
		err = fmt.Errorf("%s (%d bytes): %w", member.Name, len(member.Payload), err)
	}

	return
}

// decodeArtillerySalvos reads packed salvos and emits one event per shell.
func decodeArtillerySalvos(clock float32, reader *projectileReader) (events []Event, err error) {
	var salvoCount uint8
	if salvoCount, err = reader.uint8(); err != nil {
		return
	}

	for range int(salvoCount) {
		var (
			paramsID  uint32
			ownerID   uint32
			salvoID   int32
			shotCount uint8
		)

		if paramsID, err = reader.uint32(); err != nil {
			return
		}
		if ownerID, err = reader.uint32(); err != nil {
			return
		}
		if salvoID, err = reader.int32(); err != nil {
			return
		}
		if shotCount, err = reader.uint8(); err != nil {
			return
		}

		for range int(shotCount) {
			var shot ArtilleryShot
			shot.OwnerID = ownerID
			shot.ParamsID = paramsID
			shot.SalvoID = salvoID
			if shot.Origin, err = reader.vector3(); err != nil {
				return
			}
			if _, err = reader.float32(); err != nil { // pitch
				return
			}
			if shot.Speed, err = reader.float32(); err != nil {
				return
			}
			if shot.Target, err = reader.vector3(); err != nil {
				return
			}
			if shot.ShotID, err = reader.uint16(); err != nil {
				return
			}
			if _, err = reader.uint16(); err != nil { // gun barrel ID
				return
			}
			if shot.ServerTimeLeft, err = reader.float32(); err != nil {
				return
			}
			if _, err = reader.float32(); err != nil { // shooter height
				return
			}
			if _, err = reader.float32(); err != nil { // hit distance
				return
			}

			events = append(events, Event{Clock: clock, Type: ArtilleryShotEvent, Shell: &shot})
		}
	}

	return
}

// decodeTorpedoSalvos reads launch position and velocity while safely skipping guidance fields.
func decodeTorpedoSalvos(clock float32, reader *projectileReader) (events []Event, err error) {
	var salvoCount uint8
	if salvoCount, err = reader.uint8(); err != nil {
		return
	}

	for range int(salvoCount) {
		var (
			ownerID   uint32
			shotCount uint8
		)
		if _, err = reader.uint32(); err != nil { // params ID
			return
		}
		if ownerID, err = reader.uint32(); err != nil {
			return
		}
		if _, err = reader.int32(); err != nil { // salvo ID
			return
		}
		if _, err = reader.uint32(); err != nil { // skin ID
			return
		}
		if shotCount, err = reader.uint8(); err != nil {
			return
		}

		for range int(shotCount) {
			var torpedo TorpedoLaunch
			torpedo.OwnerID = ownerID
			if torpedo.Position, err = reader.vector3(); err != nil {
				return
			}
			if torpedo.Direction, err = reader.vector3(); err != nil {
				return
			}
			if torpedo.ShotID, err = reader.uint16(); err != nil {
				return
			}
			if _, err = reader.uint8(); err != nil { // armed
				return
			}
			if err = reader.optional(44); err != nil { // maneuver dump
				return
			}
			if err = reader.optional(31); err != nil { // acoustic dump
				return
			}

			events = append(events, Event{Clock: clock, Type: TorpedoLaunchEvent, TorpedoLaunch: &torpedo})
		}
	}

	return
}

// decodeTorpedoDirection emits authoritative heading changes for active torpedoes.
func decodeTorpedoDirection(clock float32, reader *projectileReader) (events []Event, err error) {
	var direction TorpedoDirection
	if direction.OwnerID, err = reader.uint32(); err != nil {
		return
	}
	if direction.ShotID, err = reader.uint16(); err != nil {
		return
	}
	if direction.Position, err = reader.vector3(); err != nil {
		return
	}
	if direction.TargetYaw, err = reader.float32(); err != nil {
		return
	}
	if _, err = reader.float32(); err != nil { // target depth
		return
	}
	if direction.SpeedCoef, err = reader.float32(); err != nil {
		return
	}
	if _, err = reader.float32(); err != nil { // current yaw speed
		return
	}
	if _, err = reader.float32(); err != nil { // current pitch speed
		return
	}
	if _, err = reader.uint8(); err != nil { // can reach depth
		return
	}

	events = append(events, Event{Clock: clock, Type: TorpedoDirectionEvent, TorpedoDirection: &direction})
	return
}

// decodeProjectileHits emits removals and impact points from shot-kill batches.
func decodeProjectileHits(clock float32, reader *projectileReader) (events []Event, err error) {
	var salvoCount uint8
	if salvoCount, err = reader.uint8(); err != nil {
		return
	}

	for range int(salvoCount) {
		var (
			ownerID uint32
			count   uint8
		)
		if ownerID, err = reader.uint32(); err != nil {
			return
		}
		if _, err = reader.uint8(); err != nil { // packed hit type
			return
		}
		if count, err = reader.uint8(); err != nil {
			return
		}

		for range int(count) {
			var hit ProjectileHit
			hit.OwnerID = ownerID
			if hit.Position, err = reader.vector3(); err != nil {
				return
			}
			if hit.ShotID, err = reader.uint16(); err != nil {
				return
			}
			if err = reader.optional(29); err != nil { // terminal ballistics: position, velocity, detonator, angle
				return
			}
			events = append(events, Event{Clock: clock, Type: ProjectileHitEvent, ProjectileHit: &hit})
		}
	}

	return
}

// need advances over a fixed-width field only when the full field is available.
func (reader *projectileReader) need(size int) (err error) {
	if size < 0 || size > len(reader.data)-reader.offset {
		err = fmt.Errorf("projectile payload is truncated at byte %d", reader.offset)
		return
	}

	reader.offset += size
	return
}

func (reader *projectileReader) uint8() (value uint8, err error) {
	if err = reader.need(1); err == nil {
		value = reader.data[reader.offset-1]
	}
	return
}

func (reader *projectileReader) uint16() (value uint16, err error) {
	if err = reader.need(2); err == nil {
		value = binary.LittleEndian.Uint16(reader.data[reader.offset-2 : reader.offset])
	}
	return
}

func (reader *projectileReader) uint32() (value uint32, err error) {
	if err = reader.need(4); err == nil {
		value = binary.LittleEndian.Uint32(reader.data[reader.offset-4 : reader.offset])
	}
	return
}

func (reader *projectileReader) int32() (value int32, err error) {
	var raw uint32
	if raw, err = reader.uint32(); err == nil {
		value = int32(raw)
	}
	return
}

func (reader *projectileReader) float32() (value float32, err error) {
	var raw uint32
	if raw, err = reader.uint32(); err == nil {
		value = math.Float32frombits(raw)
		if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
			err = fmt.Errorf("projectile payload contains a non-finite float")
		}
	}
	return
}

func (reader *projectileReader) vector3() (value Position, err error) {
	if value.X, err = reader.float32(); err != nil {
		return
	}
	if value.Y, err = reader.float32(); err != nil {
		return
	}
	value.Z, err = reader.float32()
	return
}

func (reader *projectileReader) optional(size int) (err error) {
	var present uint8
	if present, err = reader.uint8(); err != nil || present == 0 {
		return
	}
	if present != 1 {
		err = fmt.Errorf("projectile payload has invalid optional flag %d", present)
		return
	}

	err = reader.need(size)
	return
}
