package replayparser

import (
	"bytes"
	"fmt"

	"github.com/nlpodyssey/gopickle/pickle"
	"github.com/nlpodyssey/gopickle/types"
)

// rosterObject preserves state for Toolkit's CamouflageInfo and PlayerMode pickle classes.
type rosterObject struct {
	module string
	name   string
	args   []any
	fields map[string]any
}

// rosterClass is a restricted pickle constructor for roster-only game objects.
type rosterClass struct {
	module string
	name   string
}

func (class rosterClass) Call(args ...any) (object any, err error) {
	object = &rosterObject{module: class.module, name: class.name, args: args, fields: make(map[string]any)}
	return
}

func (class rosterClass) PyNew(args ...any) (object any, err error) {
	object = &rosterObject{module: class.module, name: class.name, args: args, fields: make(map[string]any)}
	return
}

func (object *rosterObject) PyDictSet(key, value any) (err error) {
	var (
		name string
		okay bool
	)

	if name, okay = key.(string); !okay {
		err = fmt.Errorf("roster object key has type %T", key)
		return
	}

	object.fields[name] = value
	return
}

func (object *rosterObject) PySetAttr(name string, value any) (err error) {
	object.fields[name] = value
	return
}

// loadRosterRows decodes Toolkit's flat numeric-key player records.
func loadRosterRows(blob []byte) (rows []map[int64]any, err error) {
	var unpickler pickle.Unpickler = pickle.NewUnpickler(bytes.NewReader(blob))
	unpickler.FindClass = func(module, name string) (class any, classErr error) {
		class = rosterClass{module: module, name: name}
		return
	}

	var value any
	if value, err = unpickler.Load(); err != nil {
		err = fmt.Errorf("decode player roster: %w", err)
		return
	}

	var (
		list *types.List
		okay bool
	)

	if list, okay = value.(*types.List); !okay {
		err = fmt.Errorf("player roster is %T, expected list", value)
		return
	}

	for index, row := range *list {
		var fields map[int64]any
		if fields, err = rosterFields(row); err != nil {
			err = fmt.Errorf("player %d: %w", index, err)
			return
		}

		rows = append(rows, fields)
	}

	return
}

// loadRosterBlob maps complete Toolkit player records into battle players.
func loadRosterBlob(blob []byte, modern, bots bool) (players []BattlePlayer, err error) {
	var rows []map[int64]any
	if rows, err = loadRosterRows(blob); err != nil {
		return
	}

	for index, fields := range rows {
		var player BattlePlayer
		if player, err = playerFromFields(fields, modern, bots); err != nil {
			err = fmt.Errorf("player %d: %w", index, err)
			return
		}

		players = append(players, player)
	}

	return
}

// rosterFields turns the pickled list of (numeric key, value) pairs into a lookup.
func rosterFields(row any) (fields map[int64]any, err error) {
	var (
		pairs *types.List
		okay  bool
	)

	if pairs, okay = row.(*types.List); !okay {
		err = fmt.Errorf("roster record is %T", row)
		return
	}

	fields = make(map[int64]any, pairs.Len())
	for _, pair := range *pairs {
		var tuple *types.Tuple
		if tuple, okay = pair.(*types.Tuple); !okay || tuple.Len() != 2 {
			err = fmt.Errorf("roster field is %T", pair)
			return
		}

		var key int64
		if key, okay = numericValue(tuple.Get(0)); !okay {
			err = fmt.Errorf("roster key is %T", tuple.Get(0))
			return
		}

		fields[key] = tuple.Get(1)
	}

	return
}

// numericValue accepts the integer widths produced by gopickle and RPC decoding.
func numericValue(value any) (number int64, okay bool) {
	switch typed := value.(type) {
	case int:
		number, okay = int64(typed), true
	case int8:
		number, okay = int64(typed), true
	case int16:
		number, okay = int64(typed), true
	case int32:
		number, okay = int64(typed), true
	case int64:
		number, okay = typed, true
	case uint8:
		number, okay = int64(typed), true
	case uint16:
		number, okay = int64(typed), true
	case uint32:
		number, okay = int64(typed), true
	case uint64:
		if typed <= uint64(^uint64(0)>>1) {
			number, okay = int64(typed), true
		}
	}

	return
}

// requiredRosterNumber rejects a missing or malformed field used by battle analysis.
func requiredRosterNumber(fields map[int64]any, index int64, name string) (number int64, err error) {
	var okay bool
	if number, okay = numericValue(fields[index]); !okay {
		err = fmt.Errorf("%s field %d is missing or nonnumeric", name, index)
	}

	return
}

// playerFromFields uses Toolkit's 15.8 and 15.9 numeric roster layouts.
func playerFromFields(fields map[int64]any, modern, bot bool) (player BattlePlayer, err error) {
	var nameIndex, playerIndex, entityIndex, paramIndex, teamIndex, healthIndex, accountIndex int64
	if bot {
		nameIndex, playerIndex, healthIndex, accountIndex = 19, 10, 18, 0
		entityIndex, paramIndex, teamIndex = 23, 24, 26
		if modern {
			entityIndex, paramIndex, teamIndex = 24, 25, 27
		}
	} else {
		nameIndex, playerIndex, healthIndex, accountIndex = 25, 11, 24, 0
		entityIndex, paramIndex, teamIndex = 33, 34, 36
		if modern {
			entityIndex, paramIndex, teamIndex = 34, 35, 37
		}
	}

	var name any = fields[nameIndex]
	switch typed := name.(type) {
	case string:
		player.Name = typed
	case []byte:
		player.Name = string(typed)
	default:
		err = fmt.Errorf("player name field %d has type %T", nameIndex, name)
		return
	}

	if clan, exists := fields[5]; exists {
		switch typed := clan.(type) {
		case string:
			player.Clan = typed
		case []byte:
			player.Clan = string(typed)
		}
	}

	var raw int64
	if raw, err = requiredRosterNumber(fields, playerIndex, "player ID"); err != nil {
		return
	}

	player.PlayerID = raw
	if raw, err = requiredRosterNumber(fields, entityIndex, "ship entity ID"); err != nil {
		return
	}

	if raw < 0 || raw > int64(^uint32(0)) {
		err = fmt.Errorf("ship entity ID %d is outside uint32", raw)
		return
	}

	player.EntityID = uint32(raw)
	if raw, err = requiredRosterNumber(fields, paramIndex, "ship parameter ID"); err != nil {
		return
	}

	if raw < 0 || raw > int64(^uint32(0)) {
		err = fmt.Errorf("ship parameter ID %d is outside uint32", raw)
		return
	}

	player.ShipParamID = uint32(raw)
	if player.TeamID, err = requiredRosterNumber(fields, teamIndex, "team ID"); err != nil {
		return
	}

	if player.MaxHealth, err = requiredRosterNumber(fields, healthIndex, "max health"); err != nil {
		return
	}

	if player.AccountID, err = requiredRosterNumber(fields, accountIndex, "account ID"); err != nil {
		return
	}

	player.Bot = bot
	var aliveIndex int64 = 14
	if bot {
		aliveIndex = 12
	}

	if raw, exists := fields[aliveIndex]; exists {
		if alive, okay := boolValue(raw); okay {
			player.Alive = &alive
		}
	}

	if !bot {
		if raw, exists := fields[17]; exists {
			var (
				connected bool
				okay      bool
			)

			if connected, okay = boolValue(raw); !okay {
				err = fmt.Errorf("connection field 17 has type %T", raw)
				return
			}

			player.Connected = &connected
		}
	}

	return
}

// updateRoomPlayer applies the roster fields Toolkit accepts in room-state deltas.
func (state *battleAccumulator) updateRoomPlayer(entityID uint32, player BattlePlayer, fields map[int64]any, clock float32) (err error) {
	var (
		updated bool
		moved   bool
		raw     any
		exists  bool
	)

	if raw, exists = fields[0]; exists {
		if value, okay := numericValue(raw); okay {
			player.AccountID, updated = value, true
		}
	}

	if raw, exists = fields[5]; exists {
		if value, okay := raw.(string); okay {
			player.Clan, updated = value, true
		}
	}

	if raw, exists = fields[25]; exists {
		if value, okay := raw.(string); okay {
			player.Name, updated = value, true
		}
	}

	if raw, exists = fields[24]; exists {
		if value, okay := numericValue(raw); okay {
			player.MaxHealth, updated = value, true
			if err = state.shipProperty(state.ship(entityID), "maxHealth", value, clock); err != nil {
				return
			}
		}
	}

	var entityIndex, paramIndex, teamIndex int64 = 33, 34, 36
	if state.modern {
		entityIndex, paramIndex, teamIndex = 34, 35, 37
	}

	if raw, exists = fields[paramIndex]; exists {
		if value, okay := numericValue(raw); okay && value >= 0 && value <= int64(^uint32(0)) {
			player.ShipParamID, updated = uint32(value), true
		}
	}

	if raw, exists = fields[teamIndex]; exists {
		if value, okay := numericValue(raw); okay {
			player.TeamID, updated = value, true
			if err = state.shipProperty(state.ship(entityID), "teamId", value, clock); err != nil {
				return
			}
		}
	}

	if raw, exists = fields[17]; exists {
		var (
			connected bool
			okay      bool
		)

		if connected, okay = boolValue(raw); !okay {
			err = fmt.Errorf("room-state connection has type %T", raw)
			return
		}

		if player.Connected == nil || *player.Connected != connected {
			player.Connected = &connected
			state.change(clock, "connection", entityID, &player.TeamID, connected)
			updated = true
		}
	}

	if raw, exists = fields[14]; exists {
		if alive, okay := boolValue(raw); okay {
			player.Alive, updated = &alive, true
			if err = state.shipProperty(state.ship(entityID), "isAlive", alive, clock); err != nil {
				return
			}
		}
	}

	if raw, exists = fields[entityIndex]; exists {
		if value, okay := numericValue(raw); okay && value >= 0 && value <= int64(^uint32(0)) && uint32(value) != entityID {
			delete(state.players, entityID)
			player.EntityID = uint32(value)
			entityID = player.EntityID
			updated, moved = true, true
		}
	}

	if updated {
		if moved {
			state.addPlayer(player, clock)
			return
		}

		state.players[entityID] = player
		var ship *BattleShip = state.ship(entityID)
		ship.ShipParamID = player.ShipParamID
		ship.TeamID = &player.TeamID
		state.change(clock, "player_update", entityID, &player.TeamID, player)
	}

	return
}
