package replayparser

import (
	"fmt"
	"math/bits"
)

// bitReader follows Toolkit's most-significant-bit-first nested paths.
type bitReader struct {
	data   []byte
	offset int
}

// read takes a bounded number of path bits.
func (reader *bitReader) read(count int) (value int, err error) {
	if count < 0 || count > 8 || count > len(reader.data)*8-reader.offset {
		err = fmt.Errorf("nested path needs %d bits", count)
		return
	}

	for range count {
		value = value<<1 | int((reader.data[reader.offset/8]>>(7-reader.offset%8))&1)
		reader.offset++
	}

	return
}

// aligned returns the payload following a nested path's padding bits.
func (reader *bitReader) aligned() (payload *wireReader) {
	if reader.offset = (reader.offset + 7) &^ 7; reader.offset > len(reader.data)*8 {
		reader.offset = len(reader.data) * 8
	}

	payload = &wireReader{data: reader.data[reader.offset/8:]}
	reader.offset = len(reader.data) * 8
	return
}

// defaultValue materializes an uninitialized Toolkit property for a nested update.
func (kind ArgType) defaultValue() (value any) {
	switch kind.Kind {
	case "NAMED", "USER_TYPE":
		if kind.Inner != nil {
			value = kind.Inner.defaultValue()
		}
	case "FIXED_DICT":
		var fields map[string]any = make(map[string]any, len(kind.Properties))
		for _, field := range kind.Properties {
			fields[field.Name] = field.Type.defaultValue()
		}

		value = fields
	case "ARRAY", "TUPLE":
		var count int = max(0, kind.Count)
		var elements []any = make([]any, count)
		if kind.Inner != nil {
			for index := range elements {
				elements[index] = kind.Inner.defaultValue()
			}
		}

		value = elements
	case "STRING", "UNICODE_STRING", "BLOB":
		value = []byte{}
	case "VECTOR2":
		value = []float32{0, 0}
	case "VECTOR3":
		value = []float32{0, 0, 0}
	case "FLOAT", "FLOAT32":
		value = float32(0)
	case "FLOAT64":
		value = float64(0)
	default:
		value = int64(0)
	}

	return
}

// peeled removes alias wrappers while navigating a property value.
func (kind ArgType) peeled() (inner ArgType) {
	inner = kind
	for (inner.Kind == "NAMED" || inner.Kind == "USER_TYPE") && inner.Inner != nil {
		inner = *inner.Inner
	}

	return
}

// decodeNestedProperty resolves a bit-coded update against current entity state.
func (decoder *packetDecoder) decodeNestedProperty(event *Event, reader *wireReader) (err error) {
	var (
		flag    uint8
		length  uint32
		payload []byte
		spec    *EntitySpec
	)

	if flag, err = reader.uint8(); err != nil {
		return
	}

	if flag > 1 {
		err = fmt.Errorf("invalid nested slice flag %d", flag)
		return
	}

	if length, err = reader.uint32(); err != nil {
		return
	}

	if int64(length) != int64(reader.remaining()) {
		err = fmt.Errorf("nested payload length %d differs from %d", length, reader.remaining())
		return
	}

	if payload, err = reader.takeSized(length); err != nil {
		return
	}

	if spec, err = decoder.entitySpec(event.EntityID); err != nil {
		return
	}

	if len(spec.Properties) == 0 {
		err = fmt.Errorf("%s has no client properties", spec.Name)
		return
	}

	var (
		path                bitReader = bitReader{data: payload}
		continuation, index int
	)

	if continuation, err = path.read(1); err != nil || continuation != 1 {
		if err == nil {
			err = fmt.Errorf("nested property path has no root")
		}

		return
	}

	if index, err = path.read(bits.Len(uint(len(spec.Properties) - 1))); err != nil {
		return
	}

	if index >= len(spec.Properties) {
		err = fmt.Errorf("nested property %d is outside %s", index, spec.Name)
		return
	}

	var values []any = decoder.properties[event.EntityID]
	if len(values) < len(spec.Properties) {
		var grown []any = make([]any, len(spec.Properties))
		copy(grown, values)
		values = grown
		decoder.properties[event.EntityID] = values
	}

	if values[index] == nil {
		values[index] = spec.Properties[index].Type.defaultValue()
	}

	var update NestedUpdateValue
	if values[index], update, err = applyNested(spec.Properties[index].Type, values[index], flag == 1, &path); err != nil {
		return
	}

	event.EntityType = spec.Name
	event.Name = spec.Properties[index].Name
	event.Value = update
	return
}

// applyNested follows one container level and applies Toolkit's leaf action.
func applyNested(kind ArgType, value any, isSlice bool, path *bitReader) (updated any, action NestedUpdateValue, err error) {
	var continuation int
	if continuation, err = path.read(1); err != nil {
		return
	}

	kind = kind.peeled()
	if continuation == 0 {
		updated, action, err = applyNestedLeaf(kind, value, isSlice, path)
		return
	}

	if kind.Kind == "FIXED_DICT" {
		var (
			fields map[string]any
			okay   bool
		)

		if fields, okay = value.(map[string]any); !okay || len(kind.Properties) == 0 {
			err = fmt.Errorf("nested dictionary value is unavailable")
			return
		}

		var index int
		if index, err = path.read(bits.Len(uint(len(kind.Properties) - 1))); err != nil {
			return
		}

		if index >= len(kind.Properties) {
			err = fmt.Errorf("nested dictionary key %d is outside definition", index)
			return
		}

		var field ArgField = kind.Properties[index]
		if field.Type.peeled().isPrimitive() {
			var payload *wireReader = path.aligned()
			if fields[field.Name], err = field.Type.parseValue(payload); err != nil {
				return
			}

			action = NestedUpdateValue{Action: "set_key", Key: field.Name, Value: fields[field.Name]}
		} else {
			if fields[field.Name] == nil {
				fields[field.Name] = field.Type.defaultValue()
			}

			fields[field.Name], action, err = applyNested(field.Type, fields[field.Name], isSlice, path)
			if err != nil {
				return
			}

			action.Levels = append([]any{field.Name}, action.Levels...)
		}

		updated = fields
		return
	}

	if kind.Kind == "ARRAY" && kind.Inner != nil {
		var (
			elements []any
			okay     bool
		)

		if elements, okay = value.([]any); !okay {
			err = fmt.Errorf("nested array value is unavailable")
			return
		}

		var index int
		if index, err = path.read(bits.Len(uint(max(1, len(elements)) - 1))); err != nil {
			return
		}

		for len(elements) <= index {
			elements = append(elements, kind.Inner.defaultValue())
		}

		if kind.Inner.peeled().isPrimitive() {
			var payload *wireReader = path.aligned()
			if elements[index], err = kind.Inner.parseValue(payload); err != nil {
				return
			}

			action = NestedUpdateValue{Action: "set_element", Index: index, Value: elements[index]}
		} else {
			elements[index], action, err = applyNested(*kind.Inner, elements[index], isSlice, path)
			if err != nil {
				return
			}

			action.Levels = append([]any{index}, action.Levels...)
		}

		updated = elements
		return
	}

	err = fmt.Errorf("cannot descend into nested %s", kind.Kind)
	return
}

// isPrimitive reports whether a nested child is encoded as a scalar leaf.
func (kind ArgType) isPrimitive() (primitive bool) {
	switch kind.Kind {
	case "ARRAY", "TUPLE", "FIXED_DICT":
		primitive = false
	default:
		primitive = true
	}

	return
}

// applyNestedLeaf changes one dictionary key or array range.
func applyNestedLeaf(kind ArgType, value any, isSlice bool, path *bitReader) (updated any, action NestedUpdateValue, err error) {
	if kind.Kind == "FIXED_DICT" {
		var (
			fields map[string]any
			okay   bool
		)

		if fields, okay = value.(map[string]any); !okay || len(kind.Properties) == 0 {
			err = fmt.Errorf("nested dictionary value is unavailable")
			return
		}

		var index int
		if index, err = path.read(bits.Len(uint(len(kind.Properties) - 1))); err != nil {
			return
		}

		if index >= len(kind.Properties) {
			err = fmt.Errorf("nested dictionary key %d is outside definition", index)
			return
		}

		var (
			payload *wireReader = path.aligned()
			field   ArgField    = kind.Properties[index]
		)

		if fields[field.Name], err = field.Type.parseValue(payload); err != nil {
			return
		}

		updated = fields
		action = NestedUpdateValue{Action: "set_key", Key: field.Name, Value: fields[field.Name]}
		return
	}

	if kind.Kind != "ARRAY" || kind.Inner == nil {
		err = fmt.Errorf("cannot update nested %s", kind.Kind)
		return
	}

	var (
		elements []any
		okay     bool
	)

	if elements, okay = value.([]any); !okay {
		err = fmt.Errorf("nested array value is unavailable")
		return
	}

	var (
		width       int = bits.Len(uint(max(1, len(elements)+boolInt(isSlice)) - 1))
		start, stop int
	)

	if start, err = path.read(width); err != nil {
		return
	}

	if isSlice {
		if stop, err = path.read(width); err != nil {
			return
		}
	}

	var payload *wireReader = path.aligned()
	if payload.remaining() == 0 {
		if !isSlice {
			err = fmt.Errorf("empty nested element update")
			return
		}

		start = min(start, len(elements))
		stop = min(max(start, stop), len(elements))
		elements = append(elements[:start], elements[stop:]...)
		action = NestedUpdateValue{Action: "remove_range", Start: start, Stop: stop}
	} else {
		var incoming []any
		for payload.remaining() > 0 {
			var (
				before int = payload.remaining()
				item   any
			)

			if item, err = kind.Inner.parseValue(payload); err != nil {
				return
			}

			if payload.remaining() == before {
				err = fmt.Errorf("nested element did not advance")
				return
			}

			incoming = append(incoming, item)
		}

		if isSlice {
			start = min(start, len(elements))
			stop = min(max(start, stop), len(elements))

			var replacement []any = make([]any, 0, len(elements)-(stop-start)+len(incoming))
			replacement = append(replacement, elements[:start]...)
			replacement = append(replacement, incoming...)
			elements = append(replacement, elements[stop:]...)
			action = NestedUpdateValue{Action: "set_range", Start: start, Stop: stop, Value: incoming}
		} else {
			for len(elements) <= start {
				elements = append(elements, kind.Inner.defaultValue())
			}

			elements[start] = incoming[0]
			action = NestedUpdateValue{Action: "set_element", Index: start, Value: incoming[0]}
		}
	}

	updated = elements
	return
}

// boolInt converts a slice flag for an index-width calculation.
func boolInt(value bool) (number int) {
	if value {
		number = 1
	}

	return
}
