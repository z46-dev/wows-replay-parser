package replayparser

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"strings"
)

// Packet is one replay frame; Payload borrows storage owned by Replay.
type Packet struct {
	ID      uint32  `json:"id"`
	Type    string  `json:"type"`
	Clock   float32 `json:"clock"`
	Payload []byte  `json:"payload"`
}

// PacketIterator walks packet headers without copying payloads.
type PacketIterator struct {
	data   []byte
	offset int
	modern bool
	failed bool
}

// Packets returns a fresh iterator over this replay's decrypted packet stream.
func (replay *Replay) Packets() (iterator *PacketIterator, err error) {
	if replay == nil {
		err = fmt.Errorf("replay is required")
		return
	}

	var (
		major int
		minor int
		patch int
		count int
	)

	if count, err = fmt.Sscanf(strings.ReplaceAll(replay.Metadata.ClientVersionFromExe, ",", "."), "%d.%d.%d", &major, &minor, &patch); err != nil || count != 3 {
		err = fmt.Errorf("invalid replay version %q", replay.Metadata.ClientVersionFromExe)
		return
	}

	iterator = &PacketIterator{data: replay.packets, modern: major > 12 || major == 12 && (minor > 6 || minor == 6 && patch >= 0)}
	return
}

// Next returns the next raw packet or io.EOF at the end of the stream.
func (iterator *PacketIterator) Next() (packet Packet, err error) {
	if iterator == nil {
		err = fmt.Errorf("packet iterator is required")
		return
	}

	if iterator.failed {
		err = io.EOF
		return
	}

	if iterator.offset == len(iterator.data) {
		err = io.EOF
		return
	}

	if len(iterator.data)-iterator.offset < 12 {
		err = fmt.Errorf("truncated packet header at offset %d", iterator.offset)
		iterator.failed = true
		return
	}

	var (
		data []byte = iterator.data[iterator.offset:]
		size int    = int(data[0]) | int(data[1])<<8 | int(data[2])<<16 | int(data[3])<<24
	)

	if size < 0 || size > len(data)-12 {
		err = fmt.Errorf("packet at offset %d exceeds stream", iterator.offset)
		iterator.failed = true
		return
	}

	var end int = iterator.offset + 12 + size
	packet.ID = binary.LittleEndian.Uint32(iterator.data[iterator.offset+4:])
	packet.Type = packetType(packet.ID, iterator.modern)
	packet.Clock = math.Float32frombits(binary.LittleEndian.Uint32(iterator.data[iterator.offset+8:]))
	packet.Payload = iterator.data[iterator.offset+12 : end]
	iterator.offset = end
	return
}

// WalkPackets visits each packet in order and stops on cancellation or callback error.
func (replay *Replay) WalkPackets(ctx context.Context, visit func(Packet) error) (err error) {
	if ctx == nil || visit == nil {
		err = fmt.Errorf("context and packet visitor are required")
		return
	}

	var iterator *PacketIterator
	if iterator, err = replay.Packets(); err != nil {
		return
	}

	for {
		if err = ctx.Err(); err != nil {
			return
		}

		var packet Packet
		if packet, err = iterator.Next(); err == io.EOF {
			err = nil
			return
		}

		if err != nil {
			return
		}

		if err = visit(packet); err != nil {
			return
		}
	}
}

// packetType follows Toolkit's version-keyed BigWorld packet ID mapping.
func packetType(raw uint32, modern bool) (name string) {
	switch raw {
	case 0x00:
		name = "base_player_create"
	case 0x26:
		if modern {
			name = "base_player_create_stub"
		}
	case 0x01:
		name = "cell_player_create"
	case 0x02:
		name = "entity_control"
	case 0x03:
		name = "entity_enter"
	case 0x04:
		name = "entity_leave"
	case 0x05:
		name = "entity_create"
	case 0x07:
		name = "entity_property"
	case 0x08:
		name = "entity_method"
	case 0x0a:
		name = "position"
	case 0x0e:
		name = "server_tick"
	case 0x0f:
		name = "server_timestamp"
	case 0x10:
		name = "init_flag"
	case 0x13:
		name = "init_marker"
	case 0x16:
		name = "version"
	case 0x18:
		name = "gun_marker"
	case 0x1d:
		name = "player_net_stats"
	case 0x20:
		name = "own_ship"
	case 0x22:
		if modern {
			name = "battle_results"
		} else {
			name = "nested_property_update"
		}
	case 0x23:
		if modern {
			name = "nested_property_update"
		}
	case 0x24:
		if !modern {
			name = "camera"
		}
	case 0x25:
		if modern {
			name = "camera"
		}
	case 0x27:
		if modern {
			name = "camera_mode"
		} else {
			name = "map"
		}
	case 0x28:
		if modern {
			name = "map"
		}
	case 0x29:
		if !modern {
			name = "non_volatile_position"
		}
	case 0x2a:
		if modern {
			name = "non_volatile_position"
		}
	case 0x2b:
		if !modern {
			name = "player_orientation"
		}
	case 0x2c:
		if modern {
			name = "player_orientation"
		}
	case 0x2e:
		if !modern {
			name = "camera_free_look"
		}
	case 0x2f:
		if modern {
			name = "camera_free_look"
		} else {
			name = "set_weapon_lock"
		}
	case 0x30:
		if modern {
			name = "set_weapon_lock"
		} else {
			name = "sub_controller"
		}
	case 0x31:
		if modern {
			name = "sub_controller"
		} else {
			name = "cruise_state"
		}
	case 0x32:
		if modern {
			name = "cruise_state"
		} else {
			name = "shot_tracking"
		}
	case 0x33:
		if modern {
			name = "shot_tracking"
		}
	}

	if name == "" {
		name = "unknown"
	}

	return
}
