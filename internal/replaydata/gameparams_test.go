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
		"PVSC708_Comandante_Aguirre": {"id": 3552490832, "index": "PVSC708", "name": "PVSC708_Comandante_Aguirre", "typeinfo": {"type": "Ship", "species": "Cruiser"}},
		"PAAB001_Douglas_TBD": {"id": 42, "index": "PAAB001", "name": "PAAB001_Douglas_TBD", "typeinfo": {"type": "Aircraft"}},
		"non-object": []
	}`), map[string]string{"IDS_PVSC708": "C. Aguirre", "IDS_PVSC708_FULL": "Comandante Aguirre"}); err != nil {
		t.Fatalf("ParseVehicles returned an error: %v", err)
	}

	var expected []Vehicle = []Vehicle{{ID: 3552490832, ShipID: "PVSC708", ShipName: "Comandante Aguirre", Species: "Cruiser"}}
	if !reflect.DeepEqual(vehicles, expected) {
		t.Fatalf("ParseVehicles returned %#v, expected %#v", vehicles, expected)
	}
}

// TestParsePlaneIcons follows the game's aircraft species and ammunition icon convention.
func TestParsePlaneIcons(t *testing.T) {
	const data string = `{
		"fighter_param":{"id":1,"typeinfo":{"type":"Aircraft","species":"Fighter"},"planeSubtype":["controllable"],"bombName":"projectile"},
		"projectile":{"typeinfo":{"type":"Projectile"},"ammoType":"HE"},
		"asw_param":{"id":2,"typeinfo":{"type":"Aircraft","species":"Dive"},"planeSubtype":["airsupport"],"bombName":"depth"},
		"depth":{"typeinfo":{"type":"Projectile"},"ammoType":"DepthCharge"},
		"mine_plane":{"id":3,"typeinfo":{"type":"Aircraft","species":"Dive"},"planeSubtype":["airsupport"],"bombName":"mine"},
		"mine":{"typeinfo":{"type":"Projectile"},"ammoType":"seaMine"}
	}`
	var icons []PlaneIcon
	var err error
	if icons, err = ParsePlaneIcons(strings.NewReader(data)); err != nil {
		t.Fatalf("ParsePlaneIcons returned an error: %v", err)
	}
	if len(icons) != 3 || icons[0] != (PlaneIcon{ID: 1, Name: "controllable/fighter_he"}) || icons[1] != (PlaneIcon{ID: 2, Name: "airsupport/bomber_depth_charge"}) || icons[2] != (PlaneIcon{ID: 3, Name: "airsupport/bomber_mine"}) {
		t.Fatalf("ParsePlaneIcons returned %#v", icons)
	}
}
