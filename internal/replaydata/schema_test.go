package replaydata

import (
	"encoding/binary"
	"encoding/json"
	"encoding/xml"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestAliasSizesResolveWireTypes checks the alias sizes used to order entity schema members.
func TestAliasSizesResolveWireTypes(t *testing.T) {
	var definitions xmlNode
	if err := xml.Unmarshal([]byte(`<root><TEAM_ID>INT8</TEAM_ID><WRAPPED_TEAM>USER_TYPE<Type>TEAM_ID</Type></WRAPPED_TEAM><POSITION>FIXED_DICT<Properties><x><Type>FLOAT32</Type></x><y><Type>VECTOR3</Type></y></Properties></POSITION><OPTIONAL_POSITION>FIXED_DICT<AllowNone>true</AllowNone><Properties><x><Type>FLOAT32</Type></x></Properties></OPTIONAL_POSITION><POINTS>ARRAY<of>TEAM_ID</of></POINTS></root>`), &definitions); err != nil {
		t.Fatalf("xml.Unmarshal() error = %v", err)
	}

	var sizes map[string]int = aliasSizes(definitions.Children)
	for name, expected := range map[string]int{"TEAM_ID": 1, "WRAPPED_TEAM": 65535, "POSITION": 16, "OPTIONAL_POSITION": 65535, "POINTS": 65535} {
		if sizes[name] != expected {
			t.Errorf("alias size %s = %d, want %d", name, sizes[name], expected)
		}
	}
}

// TestExposedMethodSortSizeCapsVariableArguments matches BigWorld method ordering.
func TestExposedMethodSortSizeCapsVariableArguments(t *testing.T) {
	for _, test := range []struct {
		argumentSize int
		headerSize   int
		want         int
	}{
		{argumentSize: 12, headerSize: 1, want: 13},
		{argumentSize: 65535, headerSize: 1, want: 65536},
		{argumentSize: 131070, headerSize: 4, want: 65539},
	} {
		if got := exposedMethodSortSize(test.argumentSize, test.headerSize); got != test.want {
			t.Errorf("exposedMethodSortSize(%d, %d) = %d, want %d", test.argumentSize, test.headerSize, got, test.want)
		}
	}
}

// TestDecodeInitialStateReadsVehicleScalars checks team, alive, and health values from inline state.
func TestDecodeInitialStateReadsVehicleScalars(t *testing.T) {
	var (
		schema *Schema = &Schema{
			entities:        map[uint16]string{1: "Vehicle"},
			properties:      map[string][]string{"Vehicle": {"teamId", "isAlive", "health", "maxHealth"}},
			propertyTypes:   map[string][]string{"Vehicle": {"TEAM_ID", "BOOL", "FLOAT32", "FLOAT32"}},
			aliases:         map[string]int{"TEAM_ID": 1},
			aliasPrimitives: map[string]string{"TEAM_ID": "INT8"},
		}
		state      []byte = []byte{4, 0, 2, 1, 1, 2, 0, 0, 122, 68, 3, 0, 0, 250, 68}
		properties map[string]any
		complete   bool
	)

	if properties, complete = schema.DecodeInitialState(1, state); !complete {
		t.Fatal("DecodeInitialState() complete = false, want true")
	}

	if properties["teamId"] != int8(2) || properties["isAlive"] != true || properties["health"] != float32(1000) || properties["maxHealth"] != math.Float32frombits(binary.LittleEndian.Uint32([]byte{0, 0, 250, 68})) {
		t.Fatalf("DecodeInitialState() = %#v, want team 2, alive, and decoded health values", properties)
	}
}

// TestLoadRejectsManifestPathsOutsidePackage prevents a manifest from reading outside its package.
func TestLoadRejectsManifestPathsOutsidePackage(t *testing.T) {
	var (
		directory string = t.TempDir()
		manifest  []byte
		err       error
	)

	if manifest, err = json.Marshal(Manifest{
		FormatVersion: 1,
		ClientVersion: "15.9",
		Files:         map[string]string{"../outside.json": "unused"},
	}); err != nil {
		t.Fatal(err)
	}

	if err = os.WriteFile(filepath.Join(directory, "manifest.json"), manifest, 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err = Load(directory, "15.9", false); err == nil {
		t.Fatal("Load() accepted a manifest path outside the package")
	}
}
