package replaydata

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type (
	Manifest struct {
		FormatVersion int               `json:"formatVersion"`
		ClientVersion string            `json:"clientVersion"`
		Files         map[string]string `json:"files"`
	}

	Schema struct {
		ClientVersion   string
		entities        map[uint16]string
		properties      map[string][]string
		propertyTypes   map[string][]string
		methods         map[string][]string
		aliases         map[string]int
		aliasPrimitives map[string]string
		vehicles        map[uint64]Vehicle
	}

	xmlNode struct {
		XMLName  xml.Name
		Text     string    `xml:",chardata"`
		Children []xmlNode `xml:",any"`
	}

	property struct {
		Name string
		Type string
		Flag string
	}

	method struct {
		Name string
		Size int
	}
)

var primitiveSizes = map[string]int{
	"INT8": 1, "UINT8": 1, "BOOL": 1, "INT16": 2, "UINT16": 2,
	"INT32": 4, "UINT32": 4, "FLOAT": 4, "FLOAT32": 4, "INT64": 8,
	"UINT64": 8, "FLOAT64": 8, "VECTOR2": 8, "VECTOR3": 12,
}

// Load validates an extracted replay-data package and builds its wire-index tables.
func Load(directory string, clientVersion string, allowVersionMixing bool) (schema *Schema, err error) {
	var root *os.Root
	if root, err = os.OpenRoot(directory); err != nil {
		return
	}
	defer root.Close()

	var data []byte
	if data, err = root.ReadFile("manifest.json"); err != nil {
		return
	}

	var manifest Manifest
	if err = json.Unmarshal(data, &manifest); err != nil {
		err = fmt.Errorf("decode replay-data manifest: %w", err)
		return
	}

	if manifest.FormatVersion != 1 {
		err = fmt.Errorf("unsupported replay-data package format %d", manifest.FormatVersion)
		return
	}

	if manifest.ClientVersion != clientVersion && !allowVersionMixing {
		err = fmt.Errorf("replay-data package is for %s, replay is %s", manifest.ClientVersion, clientVersion)
		return
	}

	for name, expectedHash := range manifest.Files {
		var relativePath string = filepath.FromSlash(name)
		if !filepath.IsLocal(relativePath) || relativePath == "." {
			err = fmt.Errorf("schema file path %q is not package-relative", name)
			return
		}

		var fileData []byte
		if fileData, err = root.ReadFile(relativePath); err != nil {
			err = fmt.Errorf("read schema file %s: %w", name, err)
			return
		}

		var (
			fileHash   [32]byte = sha256.Sum256(fileData)
			actualHash string   = hex.EncodeToString(fileHash[:])
		)

		if actualHash != expectedHash {
			err = fmt.Errorf("schema file %s failed SHA-256 validation", name)
			return
		}
	}

	var (
		entitiesXML xmlNode
		aliasesXML  xmlNode
		defsPath    string = "entity_defs"
	)

	if err = readXML(root, "entities.xml", &entitiesXML); err != nil {
		return
	}

	if err = readXML(root, filepath.Join(defsPath, "alias.xml"), &aliasesXML); err != nil {
		return
	}

	var (
		aliases         map[string]int    = aliasSizes(aliasesXML.Children)
		aliasPrimitives map[string]string = primitiveAliases(aliasesXML.Children)
	)

	schema = &Schema{
		ClientVersion:   manifest.ClientVersion,
		entities:        make(map[uint16]string),
		properties:      make(map[string][]string),
		propertyTypes:   make(map[string][]string),
		methods:         make(map[string][]string),
		aliases:         aliases,
		aliasPrimitives: aliasPrimitives,
	}

	schema.vehicles = make(map[uint64]Vehicle)
	if vehicleData, readErr := root.ReadFile("vehicles.json"); readErr == nil {
		var vehicles []Vehicle
		if err = json.Unmarshal(vehicleData, &vehicles); err != nil {
			err = fmt.Errorf("decode vehicle catalog: %w", err)
			return
		}

		for _, vehicle := range vehicles {
			schema.vehicles[vehicle.ID] = vehicle
		}
	} else if !os.IsNotExist(readErr) {
		err = readErr
		return
	}

	var entityNames []string
	for _, section := range entitiesXML.Children {
		if section.XMLName.Local == "ClientServerEntities" {
			for _, item := range section.Children {
				entityNames = append(entityNames, item.XMLName.Local)
			}
		}
	}

	for index, name := range entityNames {
		schema.entities[uint16(index+1)] = name

		var entity xmlNode
		if err = readXML(root, filepath.Join(defsPath, name+".def"), &entity); err != nil {
			return
		}

		var (
			allProperties []property
			allMethods    []method
		)

		if err = schema.readEntity(root, defsPath, entity, make(map[string]bool), &allProperties, &allMethods, aliases); err != nil {
			return
		}

		sort.SliceStable(allProperties, func(left, right int) bool {
			return typeSize(allProperties[left].Type, aliases) < typeSize(allProperties[right].Type, aliases)
		})

		var seenProperties map[string]bool = make(map[string]bool)
		for _, item := range allProperties {
			if clientVisible(item.Flag) && !seenProperties[item.Name] {
				schema.properties[name] = append(schema.properties[name], item.Name)
				schema.propertyTypes[name] = append(schema.propertyTypes[name], item.Type)
				seenProperties[item.Name] = true
			}
		}

		sort.SliceStable(allMethods, func(left, right int) bool {
			return allMethods[left].Size < allMethods[right].Size
		})

		var seenMethods map[string]bool = make(map[string]bool)
		for _, item := range allMethods {
			if !seenMethods[item.Name] {
				schema.methods[name] = append(schema.methods[name], item.Name)
				seenMethods[item.Name] = true
			}
		}
	}

	return
}

// readEntity applies the depth-first interface order used by BigWorld definitions.
func (schema *Schema) readEntity(root *os.Root, defsPath string, node xmlNode, visited map[string]bool, properties *[]property, methods *[]method, aliases map[string]int) (err error) {
	for _, child := range node.Children {
		if child.XMLName.Local != "Implements" {
			continue
		}

		for _, iface := range child.Children {
			var name string = strings.TrimSpace(iface.Text)
			if visited[name] {
				continue
			}

			visited[name] = true

			var interfaceNode xmlNode
			if err = readXML(root, filepath.Join(defsPath, "interfaces", name+".def"), &interfaceNode); err != nil {
				return
			}

			if err = schema.readEntity(root, defsPath, interfaceNode, visited, properties, methods, aliases); err != nil {
				return
			}
		}
	}

	for _, section := range node.Children {
		switch section.XMLName.Local {
		case "Properties":
			for _, entry := range section.Children {
				var item property = property{Name: entry.XMLName.Local}
				for _, field := range entry.Children {
					if field.XMLName.Local == "Type" {
						item.Type = strings.TrimSpace(field.Text)
					}
					if field.XMLName.Local == "Flags" {
						item.Flag = strings.TrimSpace(field.Text)
					}
				}

				*properties = append(*properties, item)
			}
		case "ClientMethods":
			for _, entry := range section.Children {
				var (
					item         method = method{Name: entry.XMLName.Local}
					argumentSize int
					headerSize   int = 1
				)

				for _, field := range entry.Children {
					if field.XMLName.Local == "Arg" {
						argumentSize += typeSize(aliasType(field), aliases)
					}

					if field.XMLName.Local == "Args" {
						for _, argument := range field.Children {
							argumentSize += typeSize(aliasType(argument), aliases)
						}
					}

					if field.XMLName.Local == "VariableLengthHeaderSize" {
						for _, detail := range field.Children {
							if parsedHeader, scanErr := strconv.Atoi(strings.TrimSpace(detail.Text)); scanErr == nil {
								headerSize = parsedHeader
								break
							}
						}
					}
				}

				item.Size = exposedMethodSortSize(argumentSize, headerSize)
				*methods = append(*methods, item)
			}
		}
	}

	return
}

// EntityName returns the entity mapped to a replay entity type ID.
func (schema *Schema) EntityName(entityTypeID uint16) string {
	return schema.entities[entityTypeID]
}

// PropertyName returns a property name when the schema defines that wire index.
func (schema *Schema) PropertyName(entityTypeID uint16, memberID uint32) string {
	var entityName string = schema.EntityName(entityTypeID)
	if entityName == "" || uint64(memberID) >= uint64(len(schema.properties[entityName])) {
		return ""
	}

	return schema.properties[entityName][memberID]
}

// MethodName returns a client method name when the schema defines that wire index.
func (schema *Schema) MethodName(entityTypeID uint16, memberID uint32) string {
	var entityName string = schema.EntityName(entityTypeID)
	if entityName == "" || uint64(memberID) >= uint64(len(schema.methods[entityName])) {
		return ""
	}

	return schema.methods[entityName][memberID]
}

// DecodeInitialState reads primitive inline properties before any unsupported variable-length values.
func (schema *Schema) DecodeInitialState(entityTypeID uint16, data []byte) (properties map[string]any, complete bool) {
	if len(data) == 0 {
		return
	}

	var (
		entityName    string   = schema.EntityName(entityTypeID)
		propertyNames []string = schema.properties[entityName]
		propertyTypes []string = schema.propertyTypes[entityName]
		propertyCount int      = int(data[0])
		offset        int      = 1
	)

	properties = make(map[string]any)
	for range propertyCount {
		if offset >= len(data) {
			return
		}

		var propertyID int = int(data[offset])
		offset++
		if propertyID >= len(propertyNames) || propertyID >= len(propertyTypes) {
			return
		}

		var (
			propertyName string = propertyNames[propertyID]
			typeName     string = strings.ToUpper(strings.TrimSpace(propertyTypes[propertyID]))
			primitive    string = typeName
		)

		if aliasType, ok := schema.aliasPrimitives[typeName]; ok {
			primitive = aliasType
		}

		var (
			value     any
			valueSize int
			decoded   bool
		)

		if value, valueSize, decoded = decodeInlinePrimitive(typeName, primitive, data[offset:]); decoded {
			properties[propertyName] = value
			offset += valueSize
			continue
		}

		valueSize = typeSize(typeName, schema.aliases)
		if valueSize >= 65535 || valueSize > len(data)-offset {
			return
		}

		offset += valueSize
	}

	complete = offset == len(data)
	return
}

// decodeInlinePrimitive decodes fixed-width scalar values from an EntityCreate state stream.
func decodeInlinePrimitive(typeName string, primitive string, data []byte) (value any, size int, ok bool) {
	if typeName == "BOOL" {
		if len(data) >= 1 {
			value = data[0] != 0
			size = 1
			ok = true
		}

		return
	}

	size = primitiveSizes[primitive]
	if size == 0 || len(data) < size {
		return
	}

	switch primitive {
	case "INT8":
		value = int8(data[0])
	case "UINT8":
		value = data[0]
	case "INT16":
		value = int16(binary.LittleEndian.Uint16(data))
	case "UINT16":
		value = binary.LittleEndian.Uint16(data)
	case "INT32":
		value = int32(binary.LittleEndian.Uint32(data))
	case "UINT32":
		value = binary.LittleEndian.Uint32(data)
	case "FLOAT", "FLOAT32":
		value = math.Float32frombits(binary.LittleEndian.Uint32(data))
	case "INT64":
		value = int64(binary.LittleEndian.Uint64(data))
	case "UINT64":
		value = binary.LittleEndian.Uint64(data)
	case "FLOAT64":
		value = math.Float64frombits(binary.LittleEndian.Uint64(data))
	case "VECTOR2":
		value = [2]float32{math.Float32frombits(binary.LittleEndian.Uint32(data)), math.Float32frombits(binary.LittleEndian.Uint32(data[4:]))}
	case "VECTOR3":
		value = [3]float32{math.Float32frombits(binary.LittleEndian.Uint32(data)), math.Float32frombits(binary.LittleEndian.Uint32(data[4:])), math.Float32frombits(binary.LittleEndian.Uint32(data[8:]))}
	default:
		return nil, 0, false
	}

	ok = true
	return
}

// Vehicle returns the ship catalog entry for a GameParams vehicle ID.
func (schema *Schema) Vehicle(vehicleTypeID uint64) (vehicle Vehicle, ok bool) {
	vehicle, ok = schema.vehicles[vehicleTypeID]
	return
}

// readXML loads one XML schema file.
func readXML(root *os.Root, path string, node *xmlNode) (err error) {
	var data []byte
	if data, err = root.ReadFile(path); err != nil {
		return
	}

	err = xml.Unmarshal(data, node)
	return
}

// clientVisible identifies property flags transmitted to replay clients.
func clientVisible(flag string) bool {
	switch flag {
	case "ALL_CLIENTS", "OTHER_CLIENTS", "OWN_CLIENT", "BASE_AND_CLIENT", "CELL_PUBLIC_AND_OWN", "ALL_CLIENTS_AND_CELL_PUBLIC":
		return true
	default:
		return false
	}
}

// typeSize returns BigWorld's ordering size for a primitive or known alias.
func typeSize(name string, aliases map[string]int) int {
	name = strings.ToUpper(strings.TrimSpace(strings.Trim(name, "<>")))
	if strings.Contains(name, "ARRAY") || strings.Contains(name, "STRING") || strings.Contains(name, "BLOB") {
		return 65535
	}

	if size, ok := primitiveSizes[name]; ok {
		return size
	}

	if size, ok := aliases[name]; ok {
		return size
	}

	return 65535
}

// exposedMethodSortSize matches BigWorld's capped argument-size ordering and header offset.
func exposedMethodSortSize(argumentSize int, headerSize int) (size int) {
	if argumentSize >= 65535 {
		size = 65535 + headerSize
	} else {
		size = argumentSize + headerSize
	}

	return
}

// aliasSizes resolves primitive aliases and fixed dictionaries for schema ordering.
func aliasSizes(definitions []xmlNode) (sizes map[string]int) {
	var nodes map[string]xmlNode = make(map[string]xmlNode, len(definitions))
	for _, definition := range definitions {
		nodes[definition.XMLName.Local] = definition
	}

	sizes = make(map[string]int, len(nodes))
	var (
		resolving map[string]bool = make(map[string]bool)
		resolve   func(string) int
	)

	resolve = func(name string) (size int) {
		if cached, ok := sizes[name]; ok {
			size = cached
			return
		}

		if resolving[name] {
			return 65535
		}

		definition, ok := nodes[name]
		if !ok {
			return typeSize(name, sizes)
		}

		resolving[name] = true
		var base string = strings.ToUpper(strings.TrimSpace(definition.Text))
		if size, ok = primitiveSizes[base]; ok {
			delete(resolving, name)
			sizes[name] = size
			return
		}

		switch {
		case strings.Contains(base, "ARRAY") || strings.Contains(base, "STRING") || strings.Contains(base, "BLOB"):
			size = 65535
		case base == "USER_TYPE":
			// BigWorld sorts exposed methods with USER_TYPE arguments as
			// variable-sized even when the wrapped primitive has a fixed wire size.
			size = 65535
		case base == "FIXED_DICT" || base == "TUPLE":
			for _, child := range definition.Children {
				if child.XMLName.Local == "AllowNone" && strings.EqualFold(strings.TrimSpace(child.Text), "true") {
					size = 65535
					break
				}

				if child.XMLName.Local != "Properties" && child.XMLName.Local != "Types" {
					continue
				}

				for _, field := range child.Children {
					for _, detail := range field.Children {
						if detail.XMLName.Local == "AllowNone" && strings.EqualFold(strings.TrimSpace(detail.Text), "true") {
							size = 65535
							break
						}
					}

					if size >= 65535 {
						break
					}

					var fieldType string
					for _, detail := range field.Children {
						if detail.XMLName.Local == "Type" {
							fieldType = strings.TrimSpace(detail.Text)
							break
						}
					}

					if fieldType == "" {
						fieldType = strings.TrimSpace(field.Text)
					}

					fieldSize := resolve(fieldType)
					if fieldSize >= 65535 {
						size = 65535
						break
					}

					size += fieldSize
				}
			}

			if size == 0 {
				size = 65535
			}
		default:
			size = resolve(base)
		}

		delete(resolving, name)
		sizes[name] = size
		return
	}

	for name := range nodes {
		resolve(name)
	}

	return
}

// primitiveAliases resolves aliases whose wire values remain fixed-width scalars.
func primitiveAliases(definitions []xmlNode) (types map[string]string) {
	var nodes map[string]xmlNode = make(map[string]xmlNode, len(definitions))
	for _, definition := range definitions {
		nodes[definition.XMLName.Local] = definition
	}

	types = make(map[string]string, len(nodes))

	var (
		resolving map[string]bool = make(map[string]bool)
		resolve   func(string) string
	)

	resolve = func(name string) (primitive string) {
		if cached, ok := types[name]; ok {
			primitive = cached
			return
		}

		if _, ok := primitiveSizes[name]; ok {
			primitive = name
			return
		}

		if resolving[name] {
			return
		}

		definition, ok := nodes[name]
		if !ok {
			return
		}

		resolving[name] = true
		var base string = strings.ToUpper(strings.TrimSpace(definition.Text))
		if _, ok := primitiveSizes[base]; ok {
			primitive = base
		} else if base == "USER_TYPE" {
			for _, child := range definition.Children {
				if child.XMLName.Local == "Type" {
					primitive = resolve(strings.ToUpper(strings.TrimSpace(child.Text)))
					break
				}
			}
		} else if _, ok := nodes[base]; ok {
			primitive = resolve(base)
		}

		delete(resolving, name)
		if primitive != "" {
			types[name] = primitive
		}

		return
	}

	for name := range nodes {
		resolve(name)
	}

	return
}

// aliasType flattens a definition type node to the primitive or alias name it contains.
func aliasType(node xmlNode) string {
	var value strings.Builder
	value.WriteString(strings.TrimSpace(node.Text))
	for _, child := range node.Children {
		value.WriteString(aliasType(child))
	}

	return value.String()
}
