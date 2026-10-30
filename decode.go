package replayparser

import (
	"errors"
	"fmt"
)

var errMethodMappingDrift = errors.New("exposed method mapping drift")

// packetDecoder tracks entity types across one replay's packet stream.
type packetDecoder struct {
	specs      []EntitySpec
	entities   map[uint32]uint16
	properties map[uint32][]any
}

// decode returns a typed event or a raw invalid event with a diagnostic.
func (decoder *packetDecoder) decode(packet Packet) (event Event, err error) {
	event = Event{ID: packet.ID, Type: packet.Type, Clock: packet.Clock, Raw: packet.Payload}
	var reader wireReader = wireReader{data: packet.Payload}

	switch packet.Type {
	case "base_player_create", "base_player_create_stub", "cell_player_create", "entity_create", "entity_property", "entity_method", "nested_property_update", "entity_enter", "entity_leave", "entity_control":
		err = decoder.decodeEntity(&event, &reader)
	case "position", "non_volatile_position", "player_orientation", "camera", "camera_mode", "camera_free_look", "player_net_stats", "server_tick", "server_timestamp", "own_ship", "battle_results", "map", "version", "set_weapon_lock", "sub_controller", "cruise_state", "shot_tracking", "gun_marker", "init_flag", "init_marker":
		err = decoder.decodePlain(&event, &reader)
	case "unknown":
		reader.offset = len(reader.data)
	default:
		event.Diagnostic = "payload decoder not implemented"
		reader.offset = len(reader.data)
	}

	if errors.Is(err, errMethodMappingDrift) {
		return
	}

	if err != nil {
		event.Type = "invalid"
		event.Name = packet.Type
		event.Diagnostic = err.Error()
		event.Value = nil
		event.Args = nil
		err = nil
		return
	}

	if reader.remaining() > 0 {
		event.Leftover = reader.data[reader.offset:]
		if event.Diagnostic == "" {
			event.Diagnostic = fmt.Sprintf("%d trailing packet bytes", reader.remaining())
		}
	}

	return
}

// entitySpec finds the game definition associated with an observed entity.
func (decoder *packetDecoder) entitySpec(entityID uint32) (spec *EntitySpec, err error) {
	var (
		entityType uint16
		exists     bool
	)

	if entityType, exists = decoder.entities[entityID]; !exists || entityType == 0 || int(entityType) > len(decoder.specs) {
		err = fmt.Errorf("unknown entity %d", entityID)
		return
	}

	spec = &decoder.specs[int(entityType)-1]
	return
}
