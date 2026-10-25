package events

import (
	"encoding/binary"
	"math"
	"testing"
)

// TestDecodeProjectileMethods reads the 15.9 artillery, torpedo, and direction payload layouts.
func TestDecodeProjectileMethods(t *testing.T) {
	var artillery []byte = []byte{1}
	artillery = appendUint32(artillery, 4041)
	artillery = appendUint32(artillery, 493709)
	artillery = appendUint32(artillery, 7)
	artillery = append(artillery, 1)
	for _, value := range []float32{1, 2, 3, 0.1, 800, 11, 12, 13} {
		artillery = appendFloat32(artillery, value)
	}
	artillery = appendUint16(artillery, 28)
	artillery = appendUint16(artillery, 2)
	for _, value := range []float32{2.5, 4, 500} {
		artillery = appendFloat32(artillery, value)
	}

	var decoded []Event
	var recognized bool
	var err error
	if decoded, recognized, err = decodeProjectileMethod(12, EntityMember{EntityType: "Avatar", Name: "receiveArtilleryShots", Payload: artillery}); err != nil {
		t.Fatalf("decode artillery: %v", err)
	}
	if !recognized || len(decoded) != 1 || decoded[0].Shell == nil || decoded[0].Shell.OwnerID != 493709 || decoded[0].Shell.ShotID != 28 || decoded[0].Shell.Origin.X != 1 || decoded[0].Shell.Target.Z != 13 || decoded[0].Shell.Speed != 800 {
		t.Fatalf("decoded artillery = %#v, recognized %t", decoded, recognized)
	}

	var torpedoes []byte = []byte{1}
	torpedoes = appendUint32(torpedoes, 400)
	torpedoes = appendUint32(torpedoes, 493709)
	torpedoes = appendUint32(torpedoes, 3)
	torpedoes = appendUint32(torpedoes, 2)
	torpedoes = append(torpedoes, 1)
	for _, value := range []float32{-10, 0, 4, 0, 0, 36} {
		torpedoes = appendFloat32(torpedoes, value)
	}
	torpedoes = appendUint16(torpedoes, 31)
	torpedoes = append(torpedoes, 1, 0, 0)
	if decoded, recognized, err = decodeProjectileMethod(20, EntityMember{EntityType: "Avatar", Name: "receiveTorpedoes", Payload: torpedoes}); err != nil {
		t.Fatalf("decode torpedo: %v", err)
	}
	if !recognized || len(decoded) != 1 || decoded[0].TorpedoLaunch == nil || decoded[0].TorpedoLaunch.ShotID != 31 || decoded[0].TorpedoLaunch.Position.X != -10 || decoded[0].TorpedoLaunch.Direction.Z != 36 {
		t.Fatalf("decoded torpedo = %#v, recognized %t", decoded, recognized)
	}

	var direction []byte = []byte{}
	direction = appendUint32(direction, 493709)
	direction = appendUint16(direction, 31)
	for _, value := range []float32{5, 0, 6, 1.25, -20, 0.8, 0, 0} {
		direction = appendFloat32(direction, value)
	}
	direction = append(direction, 1)
	if decoded, recognized, err = decodeProjectileMethod(24, EntityMember{EntityType: "Avatar", Name: "receiveTorpedoDirection", Payload: direction}); err != nil {
		t.Fatalf("decode torpedo direction: %v", err)
	}
	if !recognized || len(decoded) != 1 || decoded[0].TorpedoDirection == nil || decoded[0].TorpedoDirection.ShotID != 31 || decoded[0].TorpedoDirection.TargetYaw != 1.25 {
		t.Fatalf("decoded torpedo direction = %#v, recognized %t", decoded, recognized)
	}
}

// TestDecodeProjectileMethodsRejectsTruncatedAndNonFinitePayloads protects packet boundaries.
func TestDecodeProjectileMethodsRejectsTruncatedAndNonFinitePayloads(t *testing.T) {
	if _, recognized, err := decodeProjectileMethod(1, EntityMember{EntityType: "Avatar", Name: "receiveArtilleryShots", Payload: []byte{1}}); err == nil || !recognized {
		t.Fatalf("truncated payload error = %v, recognized %t", err, recognized)
	}

	var payload []byte = []byte{1}
	for _, value := range []uint32{1, 2, 3} {
		payload = appendUint32(payload, value)
	}
	payload = append(payload, 1)
	for index := 0; index < 12; index++ {
		payload = appendFloat32(payload, 1)
	}
	copy(payload[18:22], appendFloat32(nil, float32(math.NaN())))
	if _, recognized, err := decodeProjectileMethod(1, EntityMember{EntityType: "Avatar", Name: "receiveArtilleryShots", Payload: payload}); err == nil || !recognized {
		t.Fatalf("non-finite payload error = %v, recognized %t", err, recognized)
	}
}

func appendUint16(data []byte, value uint16) []byte {
	var encoded [2]byte
	binary.LittleEndian.PutUint16(encoded[:], value)
	return append(data, encoded[:]...)
}

func appendUint32(data []byte, value uint32) []byte {
	var encoded [4]byte
	binary.LittleEndian.PutUint32(encoded[:], value)
	return append(data, encoded[:]...)
}

func appendFloat32(data []byte, value float32) []byte {
	return appendUint32(data, math.Float32bits(value))
}
