package replayparser

import "fmt"

// decodeEntity dispatches Toolkit's entity lifecycle and member packets.
func (decoder *packetDecoder) decodeEntity(event *Event, reader *wireReader) (err error) {
	if event.EntityID, err = reader.uint32(); err != nil {
		return
	}

	switch event.Type {
	case "base_player_create", "base_player_create_stub":
		err = decoder.decodeBasePlayer(event, reader)
	case "cell_player_create":
		err = decoder.decodeCellPlayer(event, reader)
	case "entity_create":
		err = decoder.decodeEntityCreate(event, reader)
	case "entity_property", "entity_method":
		err = decoder.decodeEntityMember(event, reader)
	case "nested_property_update":
		err = decoder.decodeNestedProperty(event, reader)
	case "entity_enter":
		var value EntityEnterValue
		if value.SpaceID, err = reader.uint32(); err != nil {
			return
		}

		value.VehicleID, err = reader.uint32()
		event.Value = value
	case "entity_control":
		var controlled uint8
		if controlled, err = reader.uint8(); err != nil {
			return
		}

		event.Value = controlled != 0
	}

	return
}

// specByID resolves BigWorld's one-based entity type IDs.
func (decoder *packetDecoder) specByID(typeID uint16) (spec *EntitySpec, err error) {
	if typeID == 0 || int(typeID) > len(decoder.specs) {
		err = fmt.Errorf("entity type %d is outside %d definitions", typeID, len(decoder.specs))
		return
	}

	spec = &decoder.specs[int(typeID)-1]
	return
}

// decodeBasePlayer reads the length-delimited base properties before components.
func (decoder *packetDecoder) decodeBasePlayer(event *Event, reader *wireReader) (err error) {
	var (
		typeID uint16
		length uint32
		blob   []byte
		spec   *EntitySpec
	)

	if typeID, err = reader.uint16(); err != nil {
		return
	}

	if spec, err = decoder.specByID(typeID); err != nil {
		return
	}

	if length, err = reader.uint32(); err != nil {
		return
	}

	if blob, err = reader.takeSized(length); err != nil {
		return
	}

	var value BootstrapValue = BootstrapValue{Properties: make(map[string]any)}
	if event.Type == "base_player_create" {
		var inner wireReader = wireReader{data: blob}
		for _, property := range spec.BaseProperties {
			if value.Properties[property.Name], err = property.Type.parseValue(&inner); err != nil {
				err = fmt.Errorf("base property %s: %w", property.Name, err)
				return
			}
		}

		value.Components = inner.data[inner.offset:]
	} else {
		value.Components = blob
	}

	decoder.entities[event.EntityID] = typeID
	decoder.properties[event.EntityID] = nil
	event.EntityType, event.Value = spec.Name, value
	return
}

// decodeCellPlayer reads internal properties for an already-created player entity.
func (decoder *packetDecoder) decodeCellPlayer(event *Event, reader *wireReader) (err error) {
	var spec *EntitySpec
	if spec, err = decoder.entitySpec(event.EntityID); err != nil {
		return
	}

	var value EntityCreateValue = EntityCreateValue{Properties: make(map[string]any)}
	if value.SpaceID, err = reader.uint32(); err != nil {
		return
	}

	if value.VehicleID, err = reader.uint32(); err != nil {
		return
	}

	if value.Position, err = reader.vector3(); err != nil {
		return
	}

	if value.Rotation, err = reader.rotation3(); err != nil {
		return
	}

	var length uint32
	if length, err = reader.uint32(); err != nil {
		return
	}

	var blob []byte
	if blob, err = reader.takeSized(length); err != nil {
		return
	}

	var inner wireReader = wireReader{data: blob}
	for _, property := range spec.InternalProperties {
		if value.Properties[property.Name], err = property.Type.parseValue(&inner); err != nil {
			err = fmt.Errorf("cell property %s: %w", property.Name, err)
			return
		}
	}

	value.Components = inner.data[inner.offset:]
	event.EntityType, event.Value = spec.Name, value
	return
}

// decodeEntityCreate reads the game entity's position and indexed initial properties.
func (decoder *packetDecoder) decodeEntityCreate(event *Event, reader *wireReader) (err error) {
	var typeID uint16
	if typeID, err = reader.uint16(); err != nil {
		return
	}

	var spec *EntitySpec
	if spec, err = decoder.specByID(typeID); err != nil {
		return
	}

	var value EntityCreateValue = EntityCreateValue{Properties: make(map[string]any)}
	if value.SpaceID, err = reader.uint32(); err != nil {
		return
	}

	if value.VehicleID, err = reader.uint32(); err != nil {
		return
	}

	if value.Position, err = reader.vector3(); err != nil {
		return
	}

	if value.Rotation, err = reader.rotation3(); err != nil {
		return
	}

	if value.StateLength, err = reader.uint32(); err != nil {
		return
	}

	var (
		state wireReader = wireReader{data: reader.data[reader.offset:]}
		count uint8
	)

	reader.offset = len(reader.data)
	if count, err = state.uint8(); err != nil {
		return
	}

	for range count {
		var propertyID uint8
		if propertyID, err = state.uint8(); err != nil {
			return
		}

		if int(propertyID) >= len(spec.Properties) {
			err = fmt.Errorf("property %d is outside %s definition", propertyID, spec.Name)
			return
		}

		var property PropertySpec = spec.Properties[propertyID]
		if value.Properties[property.Name], err = property.Type.parseValue(&state); err != nil {
			err = fmt.Errorf("initial property %s: %w", property.Name, err)
			return
		}
	}

	value.Components = state.data[state.offset:]
	decoder.entities[event.EntityID] = typeID
	decoder.properties[event.EntityID] = make([]any, len(spec.Properties))
	for index, property := range spec.Properties {
		decoder.properties[event.EntityID][index] = value.Properties[property.Name]
	}

	event.EntityType, event.Value = spec.Name, value
	return
}

// decodeEntityMember resolves property or client-method IDs through its entity spec.
func (decoder *packetDecoder) decodeEntityMember(event *Event, reader *wireReader) (err error) {
	var spec *EntitySpec
	if spec, err = decoder.entitySpec(event.EntityID); err != nil {
		return
	}

	var memberID, length uint32
	if memberID, err = reader.uint32(); err != nil {
		return
	}

	if length, err = reader.uint32(); err != nil {
		return
	}

	var payload []byte
	if payload, err = reader.takeSized(length); err != nil {
		return
	}

	var inner wireReader = wireReader{data: payload}
	event.EntityType = spec.Name
	if event.Type == "entity_property" {
		if int64(memberID) >= int64(len(spec.Properties)) {
			err = fmt.Errorf("property %d is outside %s definition", memberID, spec.Name)
			return
		}

		var property PropertySpec = spec.Properties[memberID]
		event.Name = property.Name
		if event.Value, err = property.Type.parseValue(&inner); err != nil {
			err = fmt.Errorf("property %s: %w", property.Name, err)
			return
		}

		if int(memberID) < len(decoder.properties[event.EntityID]) {
			decoder.properties[event.EntityID][memberID] = event.Value
		}
	} else {
		if reader.remaining() != 0 {
			err = fmt.Errorf("method packet has %d trailing bytes", reader.remaining())
			return
		}

		if int64(memberID) >= int64(len(spec.ClientMethods)) {
			err = fmt.Errorf("method %d is outside %s definition", memberID, spec.Name)
			return
		}

		var method MethodSpec = spec.ClientMethods[memberID]
		event.Name = method.Name
		if method.FixedSize < variableSortSize && len(payload) < method.FixedSize {
			err = fmt.Errorf("%w: %s.%s id=%d expected=%d got=%d", errMethodMappingDrift, spec.Name, method.Name, memberID, method.FixedSize, len(payload))
			return
		}

		event.Args = make([]any, len(method.Args))
		for index, argument := range method.Args {
			if event.Args[index], err = argument.parseValue(&inner); err != nil {
				err = fmt.Errorf("method %s arg %d: %w", method.Name, index, err)
				return
			}
		}
	}

	// Toolkit accepts method payload extensions after the declared arguments.
	if event.Type == "entity_property" && inner.remaining() > 0 {
		event.Leftover = inner.data[inner.offset:]
		event.Diagnostic = fmt.Sprintf("%s.%s left %d payload bytes", spec.Name, event.Name, inner.remaining())
	}

	return
}
