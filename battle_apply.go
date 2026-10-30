package replayparser

import (
	"fmt"
)

// ship returns one mutable observed ship without asserting that it has spawned.
func (state *battleAccumulator) ship(entityID uint32) (ship *BattleShip) {
	if ship = state.ships[entityID]; ship == nil {
		ship = &BattleShip{EntityID: entityID}
		state.ships[entityID] = ship
	}

	return
}

// change records an ordered battle fact for later rendering or inference.
func (state *battleAccumulator) change(clock float32, kind string, entityID uint32, teamID *int64, value any) {
	state.battle.Timeline = append(state.battle.Timeline, BattleChange{
		Clock:    clock,
		Kind:     kind,
		EntityID: entityID,
		TeamID:   teamID,
		Value:    value,
	})
}

// apply consumes only packets that carry the battle facts exposed by Battle.
func (state *battleAccumulator) apply(event Event) (err error) {
	if event.Type == "invalid" {
		err = fmt.Errorf("battle packet %d (%s): %s", event.ID, event.Name, event.Diagnostic)
		return
	}

	if (event.Diagnostic != "" || len(event.Leftover) > 0) && battleSource(event) {
		err = fmt.Errorf("battle source %s/%s has undecoded bytes: %s", event.Type, event.Name, event.Diagnostic)
		return
	}

	// The observed 16-byte 0xffffffff packet is opaque in Toolkit too.
	if event.Diagnostic != "" || len(event.Leftover) > 0 || event.Type == "unknown" && !(event.ID == ^uint32(0) && event.Clock == 0 && len(event.Raw) == 16) {
		var diagnostic string = event.Diagnostic
		if event.Type == "unknown" && diagnostic == "" {
			diagnostic = "unmapped packet ID"
		}

		state.battle.Issues = append(state.battle.Issues, BattleIssue{PacketID: event.ID, Clock: event.Clock, Type: event.Type, Name: event.Name, Diagnostic: diagnostic, RemainingBytes: len(event.Leftover)})
	}

	switch event.Type {
	case "entity_method":
		switch event.Name {
		case "onArenaStateReceived":
			err = state.arenaRoster(event)
		case "onNewPlayerSpawnedInBattle":
			err = state.spawnRoster(event)
		case "onGameRoomStateChanged":
			err = state.roomState(event)
		case "updateMinimapVisionInfo":
			err = state.minimapUpdates(event)
		case "receiveVehicleDeath":
			err = state.vehicleDeath(event)
		case "onBattleEnd":
			state.battle.Ended = true
			if len(event.Args) > 0 {
				var (
					winner int64
					okay   bool
				)

				if winner, okay = numericValue(event.Args[0]); !okay {
					err = fmt.Errorf("battle end winning team has type %T", event.Args[0])
					return
				}

				state.battle.WinningTeam = &winner
			}

			state.change(event.Clock, "battle_end", 0, state.battle.WinningTeam, nil)
		}
	case "entity_create", "cell_player_create", "base_player_create":
		err = state.entityCreated(event)
	case "entity_property":
		err = state.entityProperty(event)
	case "nested_property_update":
		err = state.nestedProperty(event)
	case "battle_results":
		var (
			result string
			okay   bool
		)

		if result, okay = event.Value.(string); !okay {
			err = fmt.Errorf("battle results has type %T", event.Value)
			return
		}

		state.battle.BattleResults = result
	case "position":
		var (
			value PositionValue
			okay  bool
		)

		if value, okay = event.Value.(PositionValue); !okay {
			err = fmt.Errorf("position value has type %T", event.Value)
			return
		}

		state.worldPosition(event.EntityID, value.Position, value.Rotation, event.Clock)
	case "player_orientation":
		var (
			value OrientationValue
			okay  bool
		)

		if value, okay = event.Value.(OrientationValue); !okay {
			err = fmt.Errorf("orientation value has type %T", event.Value)
			return
		}

		if value.ParentID == 0 {
			state.worldPosition(event.EntityID, value.Position, value.Rotation, event.Clock)
		}
	}

	return
}

// battleSource identifies packets whose complete payload is required for battle facts.
func battleSource(event Event) (source bool) {
	switch event.Type {
	case "position", "player_orientation", "battle_results":
		source = true
	case "entity_create", "cell_player_create", "base_player_create":
		source = event.EntityType == "Vehicle" || event.EntityType == "BattleLogic"
	case "entity_property":
		if event.EntityType == "BattleLogic" {
			source = true
		}

		if event.EntityType == "Vehicle" {
			switch event.Name {
			case "health", "maxHealth", "teamId", "isAlive", "visibilityFlags", "isInvisible":
				source = true
			}
		}
	case "nested_property_update":
		source = event.EntityType == "BattleLogic" && event.Name == "state"
	case "entity_method":
		switch event.Name {
		case "onArenaStateReceived", "onNewPlayerSpawnedInBattle", "onGameRoomStateChanged", "updateMinimapVisionInfo", "receiveVehicleDeath", "onBattleEnd":
			source = true
		}
	}

	return
}

// roomState applies Toolkit's partial player-state updates.
func (state *battleAccumulator) roomState(event Event) (err error) {
	if len(event.Args) < 1 {
		err = fmt.Errorf("room state has no roster blob")
		return
	}

	var (
		blob []byte
		okay bool
	)

	if blob, okay = event.Args[0].([]byte); !okay {
		err = fmt.Errorf("room state blob has type %T", event.Args[0])
		return
	}

	var rows []map[int64]any
	if rows, err = loadRosterRows(blob); err != nil {
		return
	}

	for _, fields := range rows {
		var playerID int64
		if playerID, err = requiredRosterNumber(fields, 11, "room-state player ID"); err != nil {
			return
		}

		for entityID, player := range state.players {
			if player.PlayerID != playerID {
				continue
			}

			if err = state.updateRoomPlayer(entityID, player, fields, event.Clock); err != nil {
				return
			}

			break
		}
	}

	return
}

// arenaRoster seeds all participants, including ships the camera never observes.
func (state *battleAccumulator) arenaRoster(event Event) (err error) {
	if len(event.Args) < 5 {
		err = fmt.Errorf("arena roster has %d args, expected at least 5", len(event.Args))
		return
	}

	var okay bool
	if state.battle.ArenaID, okay = numericValue(event.Args[0]); !okay {
		err = fmt.Errorf("arena ID has type %T", event.Args[0])
		return
	}

	var humanBlob, botBlob []byte
	if humanBlob, okay = event.Args[3].([]byte); !okay {
		err = fmt.Errorf("arena human roster has type %T", event.Args[3])
		return
	}

	if botBlob, okay = event.Args[4].([]byte); !okay {
		err = fmt.Errorf("arena bot roster has type %T", event.Args[4])
		return
	}

	var humans, bots []BattlePlayer
	if humans, err = loadRosterBlob(humanBlob, state.modern, false); err != nil {
		return
	}

	if bots, err = loadRosterBlob(botBlob, state.modern, true); err != nil {
		return
	}

	if len(humans)+len(bots) == 0 {
		err = fmt.Errorf("arena roster is empty")
		return
	}

	state.arenaSeen = true
	for _, player := range humans {
		state.addPlayer(player, event.Clock)
	}

	for _, player := range bots {
		state.addPlayer(player, event.Clock)
	}

	return
}

// spawnRoster adds participants that join after the arena roster is sent.
func (state *battleAccumulator) spawnRoster(event Event) (err error) {
	if len(event.Args) < 2 {
		err = fmt.Errorf("spawn roster has %d args, expected 2", len(event.Args))
		return
	}

	for index := range 2 {
		var (
			blob []byte
			okay bool
		)

		if blob, okay = event.Args[index].([]byte); !okay {
			err = fmt.Errorf("spawn roster arg %d has type %T", index, event.Args[index])
			return
		}

		var players []BattlePlayer
		if players, err = loadRosterBlob(blob, state.modern, index == 1); err != nil {
			return
		}

		for _, player := range players {
			state.addPlayer(player, event.Clock)
		}
	}

	return
}

// addPlayer links a roster record to its ship and seeds full starting health.
func (state *battleAccumulator) addPlayer(player BattlePlayer, clock float32) {
	for _, vehicle := range state.battle.Metadata.Vehicles {
		if player.AccountID >= 0 && vehicle.ID == uint64(player.AccountID) {
			var relation uint32 = vehicle.Relation
			player.Relation = &relation
			break
		}
	}

	state.players[player.EntityID] = player
	var ship *BattleShip = state.ship(player.EntityID)
	if placement, exists := state.world[player.EntityID]; exists {
		ship.World = &placement
	}

	ship.ShipParamID = player.ShipParamID
	ship.TeamID = &player.TeamID

	var health float64 = float64(player.MaxHealth)
	if ship.MaxHealth == nil {
		ship.MaxHealth = &health
	}

	if ship.Health == nil {
		ship.Health = &health
	}

	if ship.Alive == nil {
		var alive bool = true
		if player.Alive != nil {
			alive = *player.Alive
		}

		ship.Alive = &alive
	}

	state.change(clock, "player", player.EntityID, &player.TeamID, player)
}

// entityCreated seeds ship properties and the battle logic's initial scores.
func (state *battleAccumulator) entityCreated(event Event) (err error) {
	var properties map[string]any
	switch value := event.Value.(type) {
	case EntityCreateValue:
		properties = value.Properties
		if event.EntityType == "Vehicle" {
			state.ship(event.EntityID)
			state.worldPosition(event.EntityID, value.Position, value.Rotation, event.Clock)
		}
	case BootstrapValue:
		properties = value.Properties
	default:
		err = fmt.Errorf("entity create value has type %T", event.Value)
		return
	}

	if event.EntityType == "BattleLogic" {
		for _, name := range []string{"state", "battleResult", "timeLeft", "battleStage", "duration"} {
			if raw, exists := properties[name]; exists {
				if err = state.matchProperty(Event{Name: name, Value: raw, Clock: event.Clock}); err != nil {
					return
				}
			}
		}

		return
	}

	if event.EntityType != "Vehicle" {
		return
	}

	var ship *BattleShip = state.ship(event.EntityID)
	for _, name := range []string{"teamId", "health", "maxHealth", "isAlive", "visibilityFlags", "isInvisible"} {
		if value, exists := properties[name]; exists {
			if err = state.shipProperty(ship, name, value, event.Clock); err != nil {
				return
			}
		}
	}

	return
}

// entityProperty updates live Vehicle properties and BattleLogic score state.
func (state *battleAccumulator) entityProperty(event Event) (err error) {
	if event.EntityType == "BattleLogic" {
		err = state.matchProperty(event)
		return
	}

	if event.EntityType != "Vehicle" {
		return
	}

	switch event.Name {
	case "health", "maxHealth", "isAlive", "teamId", "visibilityFlags", "isInvisible":
		err = state.shipProperty(state.ship(event.EntityID), event.Name, event.Value, event.Clock)
	}

	return
}

// shipProperty applies one named vehicle fact without inventing missing values.
func (state *battleAccumulator) shipProperty(ship *BattleShip, name string, value any, clock float32) (err error) {
	switch name {
	case "health", "maxHealth":
		var (
			amount float64
			okay   bool
		)

		if amount, okay = numberValue(value); !okay {
			err = fmt.Errorf("Vehicle.%s has type %T", name, value)
			return
		}

		if name == "health" {
			ship.Health = &amount
		} else {
			ship.MaxHealth = &amount
		}

		state.change(clock, name, ship.EntityID, ship.TeamID, amount)
	case "teamId":
		var (
			team int64
			okay bool
		)

		if team, okay = numericValue(value); !okay {
			err = fmt.Errorf("Vehicle.teamId has type %T", value)
			return
		}

		ship.TeamID = &team
		state.change(clock, "team", ship.EntityID, &team, team)
	case "isAlive":
		var (
			alive bool
			okay  bool
		)

		if alive, okay = boolValue(value); !okay {
			err = fmt.Errorf("Vehicle.isAlive has type %T", value)
			return
		}

		ship.Alive = &alive
		state.change(clock, "alive", ship.EntityID, ship.TeamID, alive)
	case "visibilityFlags":
		var (
			flags int64
			okay  bool
		)

		if flags, okay = numericValue(value); !okay || flags < 0 || flags > int64(^uint32(0)) {
			err = fmt.Errorf("Vehicle.visibilityFlags %v is invalid", value)
			return
		}

		var encoded uint32 = uint32(flags)
		ship.VisibilityFlags = &encoded
		state.change(clock, "visibility_flags", ship.EntityID, ship.TeamID, encoded)
	case "isInvisible":
		var (
			invisible bool
			okay      bool
		)

		if invisible, okay = boolValue(value); !okay {
			err = fmt.Errorf("Vehicle.isInvisible has type %T", value)
			return
		}

		ship.IsInvisible = &invisible
		state.change(clock, "invisible", ship.EntityID, ship.TeamID, invisible)
	}

	return
}

// worldPosition updates the last ship transform and adds it to the timeline.
func (state *battleAccumulator) worldPosition(entityID uint32, position Vector3, rotation Rotation3, clock float32) {
	var placement WorldPlacement = WorldPlacement{Position: position, Rotation: rotation, Clock: clock}
	state.world[entityID] = placement

	var (
		teamID *int64
		ship   *BattleShip
		exists bool
	)

	if ship, exists = state.ships[entityID]; exists {
		ship.World = &placement
		teamID = ship.TeamID
	}

	state.change(clock, "world_position", entityID, teamID, placement)
}

// numberValue reads health from Toolkit's float or integer RPC values.
func numberValue(value any) (number float64, okay bool) {
	switch typed := value.(type) {
	case float32:
		number, okay = float64(typed), true
	case float64:
		number, okay = typed, true
	default:
		var integer int64
		if integer, okay = numericValue(value); okay {
			number = float64(integer)
		}
	}

	return
}

// boolValue handles the boolean and integer forms in Vehicle properties.
func boolValue(value any) (boolean bool, okay bool) {
	if boolean, okay = value.(bool); okay {
		return
	}

	var number int64
	if number, okay = numericValue(value); okay {
		boolean = number != 0
	}

	return
}
