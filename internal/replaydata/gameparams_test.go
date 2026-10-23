package replaydata

import (
	"reflect"
	"strings"
	"testing"
)

// TestParseVehicles selects ships, derives readable names, and ignores other GameParams records.
func TestParseVehicles(t *testing.T) {
	var (
		vehicles []Vehicle
		err      error
	)

	if vehicles, err = ParseVehicles(strings.NewReader(`{
		"PVSC708_Comandante_Aguirre": {"id": 3552490832, "index": "PVSC708", "name": "PVSC708_Comandante_Aguirre", "typeinfo": {"type": "Ship"}},
		"PAAB001_Douglas_TBD": {"id": 42, "index": "PAAB001", "name": "PAAB001_Douglas_TBD", "typeinfo": {"type": "Aircraft"}},
		"non-object": []
	}`), map[string]string{"IDS_PVSC708": "C. Aguirre", "IDS_PVSC708_FULL": "Comandante Aguirre"}); err != nil {
		t.Fatalf("ParseVehicles returned an error: %v", err)
	}

	var expected []Vehicle = []Vehicle{{ID: 3552490832, ShipID: "PVSC708", ShipName: "Comandante Aguirre"}}
	if !reflect.DeepEqual(vehicles, expected) {
		t.Fatalf("ParseVehicles returned %#v, expected %#v", vehicles, expected)
	}
}
