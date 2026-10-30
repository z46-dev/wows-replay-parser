package replayparser

import "fmt"

const variableSortSize int = 0xffff

// ArgType describes one RPC argument's wire representation.
type ArgType struct {
	Kind       string
	Name       string
	Inner      *ArgType
	Count      int
	AllowNone  bool
	Properties []ArgField
}

// ArgField is one named field in a FIXED_DICT.
type ArgField struct {
	Name string
	Type ArgType
}

// sortSize follows Toolkit's exposed-method and property ordering rules.
func (kind ArgType) sortSize() (size int) {
	switch kind.Kind {
	case "UINT8", "INT8":
		size = 1
	case "UINT16", "INT16":
		size = 2
	case "UINT32", "INT32", "FLOAT", "FLOAT32":
		size = 4
	case "UINT64", "INT64", "FLOAT64", "VECTOR2":
		size = 8
	case "VECTOR3":
		size = 12
	case "ARRAY", "TUPLE":
		if kind.Count < 0 || kind.Inner == nil {
			size = variableSortSize
			return
		}

		if size = kind.Inner.sortSize(); size >= variableSortSize || kind.Count > variableSortSize/max(1, size) {
			size = variableSortSize
		} else {
			size *= kind.Count
		}
	case "FIXED_DICT":
		if kind.AllowNone {
			size = variableSortSize
			return
		}

		for _, field := range kind.Properties {
			if size += field.Type.sortSize(); size >= variableSortSize {
				size = variableSortSize
				return
			}
		}
	case "NAMED":
		if kind.Inner != nil {
			size = kind.Inner.sortSize()
		}
	default:
		size = variableSortSize
	}

	return
}

// parseValue reads one Toolkit RPC value from its declared wire type.
func (kind ArgType) parseValue(reader *wireReader) (value any, err error) {
	switch kind.Kind {
	case "UINT8":
		value, err = reader.uint8()
	case "UINT16":
		value, err = reader.uint16()
	case "UINT32":
		value, err = reader.uint32()
	case "UINT64":
		value, err = reader.uint64()
	case "INT8":
		value, err = reader.signed(1)
	case "INT16":
		value, err = reader.signed(2)
	case "INT32":
		value, err = reader.signed(4)
	case "INT64":
		value, err = reader.signed(8)
	case "FLOAT", "FLOAT32":
		value, err = reader.float32()
	case "FLOAT64":
		value, err = reader.float64()
	case "VECTOR2", "VECTOR3":
		var count int = 2
		if kind.Kind == "VECTOR3" {
			count = 3
		}

		var coordinates []float32 = make([]float32, count)
		for index := range coordinates {
			if coordinates[index], err = reader.float32(); err != nil {
				return
			}
		}

		value = coordinates
	case "STRING", "UNICODE_STRING", "BLOB":
		var length uint8
		if length, err = reader.uint8(); err != nil {
			return
		}

		var count int = int(length)
		if length == 0xff {
			var extended uint16
			if extended, err = reader.uint16(); err != nil {
				return
			}

			if _, err = reader.uint8(); err != nil {
				return
			}

			count = int(extended)
		}

		value, err = reader.take(count)
	case "ARRAY", "TUPLE":
		if kind.Inner == nil {
			err = fmt.Errorf("%s element type is missing", kind.Kind)
			return
		}

		var count int = kind.Count
		if count < 0 {
			var encoded uint8
			if encoded, err = reader.uint8(); err != nil {
				return
			}

			count = int(encoded)
		}

		if count > 1_000_000 {
			err = fmt.Errorf("%s count %d is too large", kind.Kind, count)
			return
		}

		var values []any = make([]any, count)
		for index := range values {
			if values[index], err = kind.Inner.parseValue(reader); err != nil {
				return
			}
		}

		value = values
	case "FIXED_DICT":
		if kind.AllowNone {
			var flag uint8
			if flag, err = reader.uint8(); err != nil {
				return
			}

			if flag == 0 {
				return
			}

			if flag != 1 {
				err = fmt.Errorf("invalid FIXED_DICT flag %d", flag)
				return
			}
		}

		var fields map[string]any = make(map[string]any, len(kind.Properties))
		for _, field := range kind.Properties {
			if fields[field.Name], err = field.Type.parseValue(reader); err != nil {
				return
			}
		}

		value = fields
	case "NAMED", "USER_TYPE":
		if kind.Inner == nil {
			err = fmt.Errorf("%s inner type is missing", kind.Kind)
			return
		}

		value, err = kind.Inner.parseValue(reader)
	default:
		err = fmt.Errorf("unsupported RPC type %q", kind.Kind)
	}

	return
}
