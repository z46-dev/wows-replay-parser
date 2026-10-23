package replaydata

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type (
	Manifest struct {
		FormatVersion int               `json:"formatVersion"`
		ClientVersion string            `json:"clientVersion"`
		Files         map[string]string `json:"files"`
	}

	Schema struct {
		ClientVersion string
		entities      map[uint16]string
		properties    map[string][]string
		methods       map[string][]string
		vehicles      map[uint64]Vehicle
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
	var data []byte
	if data, err = os.ReadFile(filepath.Join(directory, "manifest.json")); err != nil {
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
		var fileData []byte
		if fileData, err = os.ReadFile(filepath.Join(directory, filepath.FromSlash(name))); err != nil {
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
		aliases     map[string]int = make(map[string]int)
		defsPath    string         = filepath.Join(directory, "entity_defs")
	)

	if err = readXML(filepath.Join(directory, "entities.xml"), &entitiesXML); err != nil {
		return
	}

	if err = readXML(filepath.Join(defsPath, "alias.xml"), &aliasesXML); err != nil {
		return
	}

	for _, alias := range aliasesXML.Children {
		if len(alias.Children) == 0 {
			continue
		}

		aliases[alias.XMLName.Local] = typeSize(aliasType(alias.Children[0]), aliases)
	}

	schema = &Schema{ClientVersion: manifest.ClientVersion, entities: make(map[uint16]string), properties: make(map[string][]string), methods: make(map[string][]string)}
	schema.vehicles = make(map[uint64]Vehicle)
	if vehicleData, readErr := os.ReadFile(filepath.Join(directory, "vehicles.json")); readErr == nil {
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
		if err = readXML(filepath.Join(defsPath, name+".def"), &entity); err != nil {
			return
		}

		var (
			allProperties []property
			allMethods    []method
		)

		if err = schema.readEntity(defsPath, entity, make(map[string]bool), &allProperties, &allMethods, aliases); err != nil {
			return
		}

		sort.SliceStable(allProperties, func(left, right int) bool {
			return typeSize(allProperties[left].Type, aliases) < typeSize(allProperties[right].Type, aliases)
		})

		var seenProperties map[string]bool = make(map[string]bool)
		for _, item := range allProperties {
			if clientVisible(item.Flag) && !seenProperties[item.Name] {
				schema.properties[name] = append(schema.properties[name], item.Name)
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
func (schema *Schema) readEntity(defsPath string, node xmlNode, visited map[string]bool, properties *[]property, methods *[]method, aliases map[string]int) (err error) {
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
			if err = readXML(filepath.Join(defsPath, "interfaces", name+".def"), &interfaceNode); err != nil {
				return
			}

			if err = schema.readEntity(defsPath, interfaceNode, visited, properties, methods, aliases); err != nil {
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
				var item method = method{Name: entry.XMLName.Local, Size: 0}
				for _, field := range entry.Children {
					if field.XMLName.Local == "Arg" {
						item.Size += typeSize(aliasType(field), aliases)
					}

					if field.XMLName.Local == "Args" {
						for _, argument := range field.Children {
							item.Size += typeSize(aliasType(argument), aliases)
						}
					}

					if field.XMLName.Local == "VariableLengthHeaderSize" {
						var header int
						fmt.Sscan(strings.TrimSpace(field.Text), &header)
						item.Size = 65535 + header
					}
				}

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

// Vehicle returns the ship catalog entry for a GameParams vehicle ID.
func (schema *Schema) Vehicle(vehicleTypeID uint64) (vehicle Vehicle, ok bool) {
	vehicle, ok = schema.vehicles[vehicleTypeID]
	return
}

// readXML loads one XML schema file.
func readXML(path string, node *xmlNode) (err error) {
	var data []byte
	if data, err = os.ReadFile(path); err != nil {
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

// aliasType flattens a definition type node to the primitive or alias name it contains.
func aliasType(node xmlNode) string {
	var value strings.Builder
	value.WriteString(strings.TrimSpace(node.Text))
	for _, child := range node.Children {
		value.WriteString(aliasType(child))
	}

	return value.String()
}
