package replayparser

import (
	"encoding/json"
	"fmt"

	wowsextractor "github.com/z46-dev/wows-extractor"
)

// resolveShipNames matches roster GameParam IDs to archived ship definitions.
func (state *battleAccumulator) resolveShipNames(data *GameData) (err error) {
	var needed map[uint32]bool = make(map[uint32]bool, len(state.players))
	for _, player := range state.players {
		if player.ShipParamID != 0 {
			needed[player.ShipParamID] = true
		}
	}

	for _, ship := range state.ships {
		if ship.ShipParamID != 0 {
			needed[ship.ShipParamID] = true
		}
	}

	if len(needed) == 0 {
		return
	}

	var params *wowsextractor.GameParams
	if params, err = data.GameParams("NA"); err != nil {
		err = fmt.Errorf("load ship GameParams: %w", err)
		return
	}

	var names map[uint32]string = make(map[uint32]string, len(needed))
	for _, name := range params.Query("") {
		var raw json.RawMessage
		if raw, err = params.Get(name); err != nil {
			return
		}

		var entry struct {
			ID       uint32 `json:"id"`
			Name     string `json:"name"`
			TypeInfo struct {
				Type string `json:"type"`
			} `json:"typeinfo"`
		}

		if err = json.Unmarshal(raw, &entry); err != nil {
			err = fmt.Errorf("GameParams %s: %w", name, err)
			return
		}

		if entry.TypeInfo.Type == "Ship" && needed[entry.ID] {
			names[entry.ID] = entry.Name
			if len(names) == len(needed) {
				break
			}
		}
	}

	for id := range needed {
		if _, exists := names[id]; !exists {
			err = fmt.Errorf("ship GameParam %d is absent", id)
			return
		}
	}

	for entityID, player := range state.players {
		if player.ShipParamID == 0 {
			continue
		}

		var exists bool
		if player.ShipName, exists = names[player.ShipParamID]; !exists {
			err = fmt.Errorf("ship GameParam %d for %s is absent", player.ShipParamID, player.Name)
			return
		}

		state.players[entityID] = player
		state.ship(entityID).ShipName = player.ShipName
	}

	for _, ship := range state.ships {
		if ship.ShipParamID != 0 {
			ship.ShipName = names[ship.ShipParamID]
		}
	}

	for index := range state.battle.Timeline {
		if state.battle.Timeline[index].Kind == "player" || state.battle.Timeline[index].Kind == "player_update" {
			var player BattlePlayer = state.battle.Timeline[index].Value.(BattlePlayer)
			player.ShipName = names[player.ShipParamID]
			state.battle.Timeline[index].Value = player
		}
	}

	return
}
