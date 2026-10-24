package replaydata

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// Vehicle identifies a ship in the version-specific GameParams catalog.
type (
	Vehicle struct {
		ID       uint64 `json:"id"`
		ShipID   string `json:"shipID"`
		ShipName string `json:"shipName"`
		Species  string `json:"species"`
	}

	gameParamVehicle struct {
		ID      json.RawMessage `json:"id"`
		Index   string          `json:"index"`
		Name    string          `json:"name"`
		Species string          `json:"species"`
	}

	gameParamTypeInfo struct {
		TypeInfo json.RawMessage `json:"typeinfo"`
	}
)

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

		vehicles = append(vehicles, Vehicle{ID: vehicleID, ShipID: item.Index, ShipName: shipName, Species: item.Species})
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
