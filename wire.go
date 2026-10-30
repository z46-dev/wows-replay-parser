package replayparser

import (
	"encoding/binary"
	"fmt"
	"math"
)

// wireReader bounds all reads from one packet or RPC value.
type wireReader struct {
	data   []byte
	offset int
}

// remaining returns the number of unread bytes.
func (reader *wireReader) remaining() (count int) {
	count = len(reader.data) - reader.offset
	return
}

// take returns the next n bytes without copying them.
func (reader *wireReader) take(n int) (value []byte, err error) {
	if n < 0 || n > reader.remaining() {
		err = fmt.Errorf("need %d bytes, have %d", n, reader.remaining())
		return
	}

	value = reader.data[reader.offset : reader.offset+n]
	reader.offset += n
	return
}

// takeSized reads a u32-sized section without narrowing its untrusted length.
func (reader *wireReader) takeSized(size uint32) (value []byte, err error) {
	if int64(size) > int64(reader.remaining()) {
		err = fmt.Errorf("need %d bytes, have %d", size, reader.remaining())
		return
	}

	value = reader.data[reader.offset:][:size]
	reader.offset += len(value)
	return
}

// uint8 reads one unsigned byte.
func (reader *wireReader) uint8() (value uint8, err error) {
	var raw []byte
	if raw, err = reader.take(1); err != nil {
		return
	}

	value = raw[0]
	return
}

// uint16 reads one little-endian unsigned integer.
func (reader *wireReader) uint16() (value uint16, err error) {
	var raw []byte
	if raw, err = reader.take(2); err != nil {
		return
	}

	value = binary.LittleEndian.Uint16(raw)
	return
}

// uint32 reads one little-endian unsigned integer.
func (reader *wireReader) uint32() (value uint32, err error) {
	var raw []byte
	if raw, err = reader.take(4); err != nil {
		return
	}

	value = binary.LittleEndian.Uint32(raw)
	return
}

// uint64 reads one little-endian unsigned integer.
func (reader *wireReader) uint64() (value uint64, err error) {
	var raw []byte
	if raw, err = reader.take(8); err != nil {
		return
	}

	value = binary.LittleEndian.Uint64(raw)
	return
}

// float32 reads one little-endian IEEE-754 value.
func (reader *wireReader) float32() (value float32, err error) {
	var raw uint32
	if raw, err = reader.uint32(); err != nil {
		return
	}

	value = math.Float32frombits(raw)
	return
}

// float64 reads one little-endian IEEE-754 value.
func (reader *wireReader) float64() (value float64, err error) {
	var raw uint64
	if raw, err = reader.uint64(); err != nil {
		return
	}

	value = math.Float64frombits(raw)
	return
}

// signed reads a fixed-width two's-complement integer.
func (reader *wireReader) signed(size int) (value int64, err error) {
	var raw []byte
	if raw, err = reader.take(size); err != nil {
		return
	}

	switch size {
	case 1:
		value = int64(raw[0])
		if value >= 128 {
			value -= 256
		}
	case 2:
		value = int64(binary.LittleEndian.Uint16(raw))
		if value >= 1<<15 {
			value -= 1 << 16
		}
	case 4:
		value = int64(binary.LittleEndian.Uint32(raw))
		if value >= 1<<31 {
			value -= 1 << 32
		}
	case 8:
		if _, err = binary.Decode(raw, binary.LittleEndian, &value); err != nil {
			return
		}
	default:
		err = fmt.Errorf("unsupported signed width %d", size)
	}

	return
}

// vector3 reads three coordinates in Toolkit's x/y/z order.
func (reader *wireReader) vector3() (value Vector3, err error) {
	if value.X, err = reader.float32(); err != nil {
		return
	}

	if value.Y, err = reader.float32(); err != nil {
		return
	}

	value.Z, err = reader.float32()
	return
}

// rotation3 reads yaw, pitch, and roll from a packet.
func (reader *wireReader) rotation3() (value Rotation3, err error) {
	if value.Yaw, err = reader.float32(); err != nil {
		return
	}

	if value.Pitch, err = reader.float32(); err != nil {
		return
	}

	value.Roll, err = reader.float32()
	return
}
