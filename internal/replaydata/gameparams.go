package replaydata

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"

	wowsextractor "github.com/z46-dev/wows-extractor"
)

// gameParamsJSON adapts extractor entries to the existing catalog readers.
func gameParamsJSON(params *wowsextractor.GameParams) (data []byte, err error) {
	var buffer bytes.Buffer
	buffer.WriteByte('{')
	for index, name := range params.Query("") {
		if index > 0 {
			buffer.WriteByte(',')
		}

		var (
			key   []byte
			value json.RawMessage
		)

		if key, err = json.Marshal(name); err != nil {
			return
		}

		if value, err = params.Get(name); err != nil {
			return
		}

		buffer.Write(key)
		buffer.WriteByte(':')
		buffer.Write(value)
	}

	buffer.WriteByte('}')
	data = buffer.Bytes()
	return
}

// Vehicle identifies a ship in the version-specific GameParams catalog.
type (
	PlaneIcon struct {
		ID   uint64 `json:"id"`
		Name string `json:"name"`
	}

	Vehicle struct {
		ID       uint64 `json:"id"`
		ShipID   string `json:"shipID"`
		ShipName string `json:"shipName"`
		Species  string `json:"species"`
	}

	gameParamVehicle struct {
		ID       json.RawMessage `json:"id"`
		Index    string          `json:"index"`
		Name     string          `json:"name"`
		TypeInfo struct {
			Species string `json:"species"`
		} `json:"typeinfo"`
	}

	gameParamTypeInfo struct {
		TypeInfo json.RawMessage `json:"typeinfo"`
	}
)

// ParsePlaneIcons maps aircraft GameParams IDs to the same minimap icon names used by the game.
func ParsePlaneIcons(reader io.Reader) (icons []PlaneIcon, err error) {
	type (
		parameter struct {
			ID       json.RawMessage `json:"id"`
			TypeInfo struct {
				Type    string `json:"type"`
				Species string `json:"species"`
			} `json:"typeinfo"`
			PlaneSubtypes []string `json:"planeSubtype"`
			BombName      string   `json:"bombName"`
			AmmoType      string   `json:"ammoType"`
		}
		aircraft struct {
			id       uint64
			species  string
			category string
			bomb     string
		}
	)
	var decoder *json.Decoder = json.NewDecoder(reader)
	var token json.Token
	if token, err = decoder.Token(); err != nil {
		return
	}
	if token != json.Delim('{') {
		err = fmt.Errorf("GameParams root must be an object")
		return
	}
	var ammoByName map[string]string = make(map[string]string)
	var aircraftParams []aircraft
	for decoder.More() {
		var name string
		if token, err = decoder.Token(); err != nil {
			return
		}
		name = token.(string)
		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return
		}
		if len(raw) == 0 || raw[0] != '{' {
			continue
		}
		var item parameter
		if err = json.Unmarshal(raw, &item); err != nil {
			return
		}
		if item.TypeInfo.Type == "" {
			continue
		}
		switch item.TypeInfo.Type {
		case "Projectile":
			ammoByName[name] = item.AmmoType
		case "Aircraft":
			var id uint64
			if err = json.Unmarshal(item.ID, &id); err != nil {
				err = fmt.Errorf("decode GameParams aircraft ID: %w", err)
				return
			}
			var category string = "controllable"
			for _, subtype := range item.PlaneSubtypes {
				if subtype == "airsupport" || subtype == "consumable" {
					category = subtype
					break
				}
			}
			aircraftParams = append(aircraftParams, aircraft{id: id, species: item.TypeInfo.Species, category: category, bomb: item.BombName})
		}
	}
	for _, plane := range aircraftParams {
		var ammo string = ammoByName[plane.bomb]
		var category string = plane.category
		if plane.species == "Airship" || plane.species == "Auxiliary" {
			category = "controllable"
		} else if category == "controllable" && ammo == "" {
			category = "consumable"
		}
		var base string = planeIconBase(plane.species, category == "consumable", ammo)
		var directory string = category
		if directory == "consumable" {
			directory = "consumables"
		}
		icons = append(icons, PlaneIcon{ID: plane.id, Name: directory + "/" + base})
	}
	if _, err = decoder.Token(); err != nil {
		return
	}
	return
}

// planeIconBase mirrors the game's species/ammunition minimap icon mapping.
func planeIconBase(species string, consumable bool, ammo string) (base string) {
	ammo = strings.ToLower(strings.NewReplacer(" ", "_", "-", "_").Replace(ammo))
	switch ammo {
	case "sea_mine":
		ammo = "mine"
	case "seamine":
		ammo = "mine"
	case "depthcharge":
		ammo = "depth_charge"
	case "torpedodeepwater":
		ammo = "torpedo_deepwater"
	}
	if consumable {
		if species == "Dive" && ammo != "" {
			return "bomber_" + ammo
		}
		if species == "Dive" {
			return "bomber"
		}
		return strings.ToLower(species)
	}
	switch species {
	case "Fighter":
		if ammo != "" {
			return "fighter_" + ammo
		}
		return "fighter"
	case "Dive":
		if ammo != "" {
			return "bomber_" + ammo
		}
		return "bomber"
	case "Bomber":
		if ammo == "torpedo_deepwater" {
			return ammo
		}
		return "torpedo_regular"
	case "Skip":
		if ammo != "" {
			return "skip_" + ammo
		}
		return "skip"
	case "Airship", "Auxiliary":
		return "auxiliary"
	default:
		if ammo != "" {
			return "fighter_" + ammo
		}
		return "fighter"
	}
}

// ParseVehicles streams GameParams JSON and extracts only ship identifiers and names.
func ParseVehicles(reader io.Reader, localizedNames map[string]string) (vehicles []Vehicle, err error) {
	var decoder *json.Decoder = json.NewDecoder(reader)

	var openingToken json.Token
	if openingToken, err = decoder.Token(); err != nil {
		return
	}

	if openingToken != json.Delim('{') {
		err = fmt.Errorf("GameParams root must be an object")
		return
	}

	var seenIDs map[uint64]bool = make(map[uint64]bool)
	for decoder.More() {
		if _, err = decoder.Token(); err != nil {
			return
		}

		var raw json.RawMessage
		if err = decoder.Decode(&raw); err != nil {
			return
		}

		if len(raw) == 0 || raw[0] != '{' {
			continue
		}

		var typeInfo gameParamTypeInfo
		if err = json.Unmarshal(raw, &typeInfo); err != nil {
			return
		}

		var vehicleType struct {
			Type string `json:"type"`
		}

		if err = json.Unmarshal(typeInfo.TypeInfo, &vehicleType); err != nil || vehicleType.Type != "Ship" {
			continue
		}

		var item gameParamVehicle
		if err = json.Unmarshal(raw, &item); err != nil {
			return
		}

		var vehicleID uint64
		if err = json.Unmarshal(item.ID, &vehicleID); err != nil {
			err = fmt.Errorf("decode GameParams ship ID: %w", err)
			return
		}

		if vehicleID == 0 || item.Index == "" || item.Name == "" {
			err = fmt.Errorf("GameParams ship entry is missing its id, index, or name")
			return
		}

		if seenIDs[vehicleID] {
			err = fmt.Errorf("GameParams contains duplicate ship ID %d", vehicleID)
			return
		}

		var shipName string = localizedNames["IDS_"+item.Index+"_FULL"]
		if shipName == "" {
			shipName = localizedNames["IDS_"+item.Index]
			if shipName == "" {
				shipName = strings.TrimPrefix(item.Name, item.Index+"_")
				shipName = strings.ReplaceAll(shipName, "_", " ")
			}
		}

		vehicles = append(vehicles, Vehicle{ID: vehicleID, ShipID: item.Index, ShipName: shipName, Species: item.TypeInfo.Species})
		seenIDs[vehicleID] = true
	}

	if _, err = decoder.Token(); err != nil {
		return
	}

	if len(vehicles) == 0 {
		err = fmt.Errorf("GameParams contains no ship entries")
		return
	}

	sort.Slice(vehicles, func(left, right int) bool {
		return vehicles[left].ID < vehicles[right].ID
	})

	return
}
