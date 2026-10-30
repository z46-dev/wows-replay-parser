package replayparser

import (
	"encoding/xml"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// PropertySpec is one client-visible entity property.
type PropertySpec struct {
	Name  string
	Type  ArgType
	Flags string
}

// MethodSpec is one client method in its exposed-method order.
type MethodSpec struct {
	Name       string
	Args       []ArgType
	HeaderSize int
	FixedSize  int
}

// EntitySpec is one entity definition in the order of scripts/entities.xml.
type EntitySpec struct {
	Name               string
	BaseMethods        []MethodSpec
	CellMethods        []MethodSpec
	ClientMethods      []MethodSpec
	Properties         []PropertySpec
	InternalProperties []PropertySpec
	BaseProperties     []PropertySpec
}

type xmlNode struct {
	XMLName  xml.Name
	Text     string    `xml:",chardata"`
	Children []xmlNode `xml:",any"`
}

type entityDefinition struct {
	baseMethods   []MethodSpec
	cellMethods   []MethodSpec
	clientMethods []MethodSpec
	properties    []PropertySpec
	implements    []string
}

// child finds one direct XML child by tag name.
func (node *xmlNode) child(name string) (child *xmlNode) {
	for index := range node.Children {
		if node.Children[index].XMLName.Local == name {
			child = &node.Children[index]
			break
		}
	}

	return
}

// parseXML reads one game script without discarding child order.
func parseXML(data []byte) (node xmlNode, err error) {
	if err = xml.Unmarshal(data, &node); err != nil {
		err = fmt.Errorf("invalid game script XML: %w", err)
	}

	return
}

// loadEntitySpecs follows Toolkit's alias, interface, method, and property ordering.
func loadEntitySpecs(data *GameData) (specs []EntitySpec, err error) {
	var (
		raw      []byte
		aliases  xmlNode
		entities xmlNode
		root     *xmlNode
		resolved map[string]ArgType          = make(map[string]ArgType)
		cache    map[string]entityDefinition = make(map[string]entityDefinition)
	)

	if raw, err = data.ReadFile("scripts/entity_defs/alias.xml"); err != nil {
		return
	}

	if aliases, err = parseXML(raw); err != nil {
		return
	}

	for index := range aliases.Children {
		var alias xmlNode = aliases.Children[index]
		if resolved[alias.XMLName.Local], err = parseRPCType(&alias, resolved); err != nil {
			err = fmt.Errorf("alias %s: %w", alias.XMLName.Local, err)
			return
		}
	}

	if raw, err = data.ReadFile("scripts/entities.xml"); err != nil {
		return
	}

	if entities, err = parseXML(raw); err != nil {
		return
	}

	if root = entities.child("ClientServerEntities"); root == nil {
		err = fmt.Errorf("scripts/entities.xml has no ClientServerEntities")
		return
	}

	for index := range root.Children {
		var (
			name           string = root.Children[index].XMLName.Local
			own, inherited entityDefinition
		)

		if own, err = readDefinition(data, "scripts/entity_defs/"+name+".def", resolved, cache); err != nil {
			return
		}

		for _, parent := range own.implements {
			var parentDef entityDefinition
			if parentDef, err = readDefinition(data, "scripts/entity_defs/interfaces/"+strings.TrimSpace(parent)+".def", resolved, cache); err != nil {
				return
			}

			for _, grandparent := range parentDef.implements {
				var grandparentDef entityDefinition
				if grandparentDef, err = readDefinition(data, "scripts/entity_defs/interfaces/"+strings.TrimSpace(grandparent)+".def", resolved, cache); err != nil {
					return
				}

				inherited = appendDefinition(inherited, grandparentDef)
			}

			inherited = appendDefinition(inherited, parentDef)
		}

		var (
			combined entityDefinition = appendDefinition(inherited, own)
			spec     EntitySpec       = EntitySpec{Name: name, BaseMethods: combined.baseMethods, CellMethods: combined.cellMethods}
			seen     map[string]bool  = make(map[string]bool)
		)

		for _, method := range combined.clientMethods {
			if !seen[method.Name] {
				spec.ClientMethods = append(spec.ClientMethods, method)
				seen[method.Name] = true
			}
		}

		sort.SliceStable(spec.ClientMethods, func(left, right int) bool {
			return spec.ClientMethods[left].sortSize() < spec.ClientMethods[right].sortSize()
		})

		for _, property := range combined.properties {
			if property.Flags == "BASE_AND_CLIENT" {
				spec.BaseProperties = append(spec.BaseProperties, property)
			}

			switch property.Flags {
			case "ALL_CLIENTS", "OTHER_CLIENTS", "OWN_CLIENT", "CELL_PUBLIC_AND_OWN":
				spec.InternalProperties = append(spec.InternalProperties, property)
				spec.Properties = append(spec.Properties, property)
			case "BASE_AND_CLIENT":
				spec.Properties = append(spec.Properties, property)
			}
		}

		sort.SliceStable(spec.Properties, func(left, right int) bool {
			return spec.Properties[left].Type.sortSize() < spec.Properties[right].Type.sortSize()
		})

		specs = append(specs, spec)
	}

	return
}

// appendDefinition merges inherited members before an entity's own members.
func appendDefinition(target, source entityDefinition) (combined entityDefinition) {
	combined.baseMethods = append(append([]MethodSpec(nil), target.baseMethods...), source.baseMethods...)
	combined.cellMethods = append(append([]MethodSpec(nil), target.cellMethods...), source.cellMethods...)
	combined.clientMethods = append(append([]MethodSpec(nil), target.clientMethods...), source.clientMethods...)
	combined.properties = append(append([]PropertySpec(nil), target.properties...), source.properties...)

	return
}

// readDefinition parses and caches one entity or interface definition.
func readDefinition(data *GameData, path string, aliases map[string]ArgType, cache map[string]entityDefinition) (definition entityDefinition, err error) {
	if cached, exists := cache[path]; exists {
		definition = cached
		return
	}

	var raw []byte
	if raw, err = data.ReadFile(path); err != nil {
		err = fmt.Errorf("%s: %w", path, err)
		return
	}

	var node xmlNode
	if node, err = parseXML(raw); err != nil {
		err = fmt.Errorf("%s: %w", path, err)
		return
	}

	for _, section := range node.Children {
		switch section.XMLName.Local {
		case "Implements":
			for _, child := range section.Children {
				definition.implements = append(definition.implements, strings.TrimSpace(child.Text))
			}
		case "Properties":
			for index := range section.Children {
				var property PropertySpec
				if property, err = parseProperty(&section.Children[index], aliases); err != nil {
					err = fmt.Errorf("%s: %w", path, err)
					return
				}

				definition.properties = append(definition.properties, property)
			}
		case "BaseMethods", "CellMethods", "ClientMethods":
			for index := range section.Children {
				var method MethodSpec
				if method, err = parseMethod(&section.Children[index], aliases); err != nil {
					err = fmt.Errorf("%s: %w", path, err)
					return
				}

				switch section.XMLName.Local {
				case "BaseMethods":
					definition.baseMethods = append(definition.baseMethods, method)
				case "CellMethods":
					definition.cellMethods = append(definition.cellMethods, method)
				case "ClientMethods":
					definition.clientMethods = append(definition.clientMethods, method)
				}
			}
		}
	}

	cache[path] = definition
	return
}

// parseProperty reads a named property and validates its client visibility flag.
func parseProperty(node *xmlNode, aliases map[string]ArgType) (property PropertySpec, err error) {
	var typeNode, flagsNode *xmlNode = node.child("Type"), node.child("Flags")
	if typeNode == nil || flagsNode == nil {
		err = fmt.Errorf("property %s lacks Type or Flags", node.XMLName.Local)
		return
	}

	property.Name = node.XMLName.Local
	property.Flags = strings.TrimSpace(flagsNode.Text)

	switch property.Flags {
	case "ALL_CLIENTS", "CELL_PUBLIC_AND_OWN", "OWN_CLIENT", "BASE_AND_CLIENT", "BASE", "CELL_PRIVATE", "CELL_PUBLIC", "OTHER_CLIENTS":
	default:
		err = fmt.Errorf("property %s has unknown flag %q", property.Name, property.Flags)
		return
	}

	property.Type, err = parseRPCType(typeNode, aliases)
	return
}

// parseMethod reads one exposed method and its arguments.
func parseMethod(node *xmlNode, aliases map[string]ArgType) (method MethodSpec, err error) {
	method.Name = node.XMLName.Local
	method.HeaderSize = 1

	for index := range node.Children {
		var child *xmlNode = &node.Children[index]

		switch child.XMLName.Local {
		case "Arg":
			var argument ArgType
			if argument, err = parseRPCType(child, aliases); err != nil {
				return
			}

			method.Args = append(method.Args, argument)
		case "Args":
			for argumentIndex := range child.Children {
				var argument ArgType
				if argument, err = parseRPCType(&child.Children[argumentIndex], aliases); err != nil {
					return
				}

				method.Args = append(method.Args, argument)
			}
		case "VariableLengthHeaderSize":
			if method.HeaderSize, err = strconv.Atoi(strings.TrimSpace(child.Text)); err != nil {
				method.HeaderSize = 1
				err = nil
			}
		}
	}

	for _, argument := range method.Args {
		if method.FixedSize += argument.sortSize(); method.FixedSize >= variableSortSize {
			method.FixedSize = variableSortSize
			break
		}
	}

	return
}

// sortSize applies Toolkit's exposed-method sort key.
func (method MethodSpec) sortSize() (size int) {
	size = min(variableSortSize, method.FixedSize) + method.HeaderSize
	return
}

// parseRPCType resolves one primitive, composite, or named game alias.
func parseRPCType(node *xmlNode, aliases map[string]ArgType) (kind ArgType, err error) {
	var name string = strings.ToUpper(strings.TrimSpace(node.Text))

	switch name {
	case "UINT8", "UINT16", "UINT32", "UINT64", "INT8", "INT16", "INT32", "INT64", "FLOAT", "FLOAT32", "FLOAT64", "VECTOR2", "VECTOR3", "STRING", "UNICODE_STRING", "BLOB":
		kind.Kind = name
	case "MAILBOX", "PYTHON":
		kind.Kind = "BLOB"
	case "USER_TYPE":
		if inner := node.child("Type"); inner != nil {
			kind.Kind = "USER_TYPE"

			var parsed ArgType
			if parsed, err = parseRPCType(inner, aliases); err != nil {
				return
			}

			kind.Inner = &parsed
		} else {
			kind.Kind = "BLOB"
		}
	case "ARRAY", "TUPLE":
		var inner, sizeNode *xmlNode = node.child("of"), node.child("size")

		if inner == nil || name == "TUPLE" && sizeNode == nil {
			err = fmt.Errorf("%s lacks element type or size", name)
			return
		}

		kind.Kind = name
		kind.Count = -1
		if sizeNode != nil {
			if kind.Count, err = strconv.Atoi(strings.TrimSpace(sizeNode.Text)); err != nil || kind.Count < 0 {
				err = fmt.Errorf("invalid %s size %q", name, sizeNode.Text)
				return
			}
		}

		var parsed ArgType
		if parsed, err = parseRPCType(inner, aliases); err != nil {
			return
		}

		kind.Inner = &parsed
	case "FIXED_DICT":
		kind.Kind = name
		kind.AllowNone = node.child("AllowNone") != nil

		if fields := node.child("Properties"); fields != nil {
			for index := range fields.Children {
				var field ArgField = ArgField{Name: fields.Children[index].XMLName.Local}
				if inner := fields.Children[index].child("Type"); inner != nil {
					if field.Type, err = parseRPCType(inner, aliases); err != nil {
						return
					}
				} else {
					err = fmt.Errorf("FIXED_DICT field %s has no Type", field.Name)
					return
				}

				kind.Properties = append(kind.Properties, field)
			}
		}
	default:
		var (
			resolved ArgType
			exists   bool
		)

		if resolved, exists = aliases[name]; !exists {
			err = fmt.Errorf("unknown RPC type %q", name)
			return
		}

		kind = ArgType{Kind: "NAMED", Name: name, Inner: &resolved}
	}

	return
}
