package replaydata

import (
	"encoding/binary"
	"strconv"
	"strings"
)

type InitialTeamScore struct {
	TeamIndex int64
	Score     uint16
}

// DecodeBattleLogicInitialScores extracts teamsScore without decoding unrelated state.
func (schema *Schema) DecodeBattleLogicInitialScores(entityTypeID uint16, data []byte) (scores []InitialTeamScore, ok bool) {
	if schema.EntityName(entityTypeID) != "BattleLogic" || len(data) == 0 {
		return
	}

	var (
		properties []string = schema.properties["BattleLogic"]
		types      []string = schema.propertyTypes["BattleLogic"]
		offset     int      = 1
	)
	for range int(data[0]) {
		if offset >= len(data) {
			return nil, false
		}

		var index int = int(data[offset])
		offset++
		if index >= len(properties) || index >= len(types) {
			return nil, false
		}
		if properties[index] == "state" {
			return schema.readScorePath(types[index], data[offset:], []string{"missions", "teamsScore"})
		}
		var size int
		if size, ok = schema.skipValue(types[index], data[offset:]); !ok || size > len(data)-offset {
			return nil, false
		}
		offset += size
	}
	return nil, false
}

// readScorePath follows one named path through alias-backed dictionaries.
func (schema *Schema) readScorePath(typeName string, data []byte, path []string) (scores []InitialTeamScore, ok bool) {
	if len(path) == 0 {
		return nil, false
	}

	var node, exists = schema.aliasNodes[strings.ToUpper(strings.TrimSpace(typeName))]
	if !exists || strings.TrimSpace(node.Text) != "FIXED_DICT" {
		return nil, false
	}
	if allowNone, exists := childNode(node, "AllowNone"); exists && strings.EqualFold(strings.TrimSpace(allowNone.Text), "true") {
		if len(data) == 0 || data[0] == 0 {
			return nil, false
		}
		data = data[1:]
	}

	properties, exists := childNode(node, "Properties")
	if !exists {
		return nil, false
	}
	for _, field := range properties.Children {
		typeNode, exists := childNode(field, "Type")
		if !exists {
			return nil, false
		}
		if field.XMLName.Local == path[0] { // #nosec G602 -- the nonempty path guard at function entry protects this index.
			if len(path) == 1 {
				return schema.readTeamScores(typeNode, data)
			}
			return schema.readScorePath(strings.TrimSpace(typeNode.Text), data, path[1:]) // #nosec G602 -- path[0] matched and len(path) is greater than one.
		}
		var size int
		if size, ok = schema.skipTypeNode(typeNode, data); !ok || size > len(data) {
			return nil, false
		}
		data = data[size:]
	}
	return nil, false
}

// readTeamScores decodes the fixed team-score array and its uint16 score fields.
func (schema *Schema) readTeamScores(node xmlNode, data []byte) (scores []InitialTeamScore, ok bool) {
	if strings.TrimSpace(node.Text) != "ARRAY" || len(data) == 0 {
		return nil, false
	}
	var element, exists = childNode(node, "of")
	if !exists {
		return nil, false
	}
	count := int(data[0])
	data = data[1:]
	for index := range count {
		var teamNode, exists = schema.aliasNodes[strings.ToUpper(strings.TrimSpace(element.Text))]
		if !exists || strings.TrimSpace(teamNode.Text) != "FIXED_DICT" {
			return nil, false
		}
		var score uint16
		for _, field := range teamNode.Children {
			if field.XMLName.Local != "Properties" {
				continue
			}
			for _, property := range field.Children {
				typeNode, found := childNode(property, "Type")
				if !found {
					return nil, false
				}
				if property.XMLName.Local == "score" {
					if len(data) < 2 || strings.TrimSpace(typeNode.Text) != "UINT16" {
						return nil, false
					}
					score = binary.LittleEndian.Uint16(data)
				}
				size, valid := schema.skipValue(strings.TrimSpace(typeNode.Text), data)
				if !valid || size > len(data) {
					return nil, false
				}
				data = data[size:]
			}
		}
		scores = append(scores, InitialTeamScore{TeamIndex: int64(index), Score: score})
	}
	return scores, len(scores) > 0
}

// skipValue advances over an inline entity property using its schema type.
func (schema *Schema) skipValue(typeName string, data []byte) (size int, ok bool) {
	typeName = strings.ToUpper(strings.TrimSpace(typeName))
	if typeName == "BOOL" || typeName == "STRING" || typeName == "UNICODE_STRING" || typeName == "BLOB" {
		return schema.skipNode(xmlNode{Text: typeName}, data)
	}
	if node, exists := schema.aliasNodes[strings.ToUpper(strings.TrimSpace(typeName))]; exists {
		return schema.skipNode(node, data)
	}
	size = primitiveSizes[typeName]
	ok = size > 0 && size <= len(data)
	return
}

func (schema *Schema) skipTypeNode(node xmlNode, data []byte) (size int, ok bool) {
	if len(node.Children) > 0 {
		return schema.skipNode(node, data)
	}
	return schema.skipValue(strings.TrimSpace(node.Text), data)
}

// skipNode skips one schema value, including variable-length arrays and dictionaries.
func (schema *Schema) skipNode(node xmlNode, data []byte) (size int, ok bool) {
	typeName := strings.ToUpper(strings.TrimSpace(node.Text))
	if primitiveSize := primitiveSizes[typeName]; primitiveSize > 0 {
		return primitiveSize, primitiveSize <= len(data)
	}
	switch typeName {
	case "BOOL", "STRING", "UNICODE_STRING", "BLOB":
		if len(data) == 0 {
			return 0, false
		}
		size = 1 + int(data[0])
		if data[0] == 0xff {
			if len(data) < 4 {
				return 0, false
			}
			size = 4 + int(binary.LittleEndian.Uint16(data[1:3]))
		}
		return size, size <= len(data)
	case "ARRAY":
		element, exists := childNode(node, "of")
		if !exists {
			return 0, false
		}
		var count int
		if fixed, exists := childNode(node, "size"); exists {
			var err error
			if count, err = strconv.Atoi(strings.TrimSpace(fixed.Text)); err != nil || count < 0 {
				return 0, false
			}
		} else {
			if len(data) == 0 {
				return 0, false
			}
			count = int(data[0])
			size = 1
		}
		for range count {
			if size > len(data) {
				return 0, false
			}
			itemSize, valid := schema.skipValue(strings.TrimSpace(element.Text), data[size:])
			if !valid {
				return 0, false
			}
			size += itemSize
		}
		return size, size <= len(data)
	case "FIXED_DICT":
		if allowNone, exists := childNode(node, "AllowNone"); exists && strings.EqualFold(strings.TrimSpace(allowNone.Text), "true") {
			if len(data) == 0 {
				return 0, false
			}
			size = 1
			if data[0] == 0 {
				return size, true
			}
		}
		properties, exists := childNode(node, "Properties")
		if !exists {
			return 0, false
		}
		for _, field := range properties.Children {
			typeNode, exists := childNode(field, "Type")
			if !exists {
				return 0, false
			}
			fieldSize, valid := schema.skipTypeNode(typeNode, data[size:])
			if !valid || fieldSize > len(data)-size {
				return 0, false
			}
			size += fieldSize
		}
		return size, true
	case "TUPLE":
		element, hasElement := childNode(node, "of")
		fixed, hasSize := childNode(node, "size")
		if !hasElement || !hasSize {
			return 0, false
		}
		count, err := strconv.Atoi(strings.TrimSpace(fixed.Text))
		if err != nil || count < 0 {
			return 0, false
		}
		for range count {
			if size > len(data) {
				return 0, false
			}
			itemSize, valid := schema.skipValue(strings.TrimSpace(element.Text), data[size:])
			if !valid {
				return 0, false
			}
			size += itemSize
		}
		return size, true
	case "USER_TYPE":
		if wrapped, exists := childNode(node, "Type"); exists {
			return schema.skipValue(strings.TrimSpace(wrapped.Text), data)
		}
		return 0, false
	}
	return 0, false
}

func childNode(parent xmlNode, name string) (node xmlNode, ok bool) {
	for _, child := range parent.Children {
		if child.XMLName.Local == name {
			return child, true
		}
	}
	return
}
