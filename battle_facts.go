package replayparser

import (
	"fmt"
)

// battleLogic reads Toolkit's initial team scores from state.missions.teamsScore.
func (state *battleAccumulator) battleLogic(value any, clock float32) (err error) {
	if value == nil {
		return
	}

	var (
		root map[string]any
		okay bool
	)

	if root, okay = value.(map[string]any); !okay {
		err = fmt.Errorf("BattleLogic.state has type %T", value)
		return
	}

	var missions map[string]any
	if missions, okay = root["missions"].(map[string]any); !okay {
		return
	}

	var entries []any
	if entries, okay = missions["teamsScore"].([]any); !okay {
		return
	}

	for index, entry := range entries {
		var fields map[string]any
		if fields, okay = entry.(map[string]any); !okay {
			continue
		}

		var (
			teamID *int64
			raw    any
			exists bool
		)

		if raw, exists = fields["teamId"]; exists {
			var parsed int64
			if parsed, okay = numericValue(raw); !okay {
				err = fmt.Errorf("team score %d teamId has type %T", index, raw)
				return

			}
			teamID = &parsed
		}

		var score int64
		if raw, exists = fields["score"]; exists {
			if score, okay = numericValue(raw); !okay {
				err = fmt.Errorf("team score %d score has type %T", index, raw)
				return
			}
		}

		state.setScore(index, teamID, score, clock)
	}

	return
}

// nestedProperty applies Toolkit's BattleLogic score update path.
func (state *battleAccumulator) nestedProperty(event Event) (err error) {
	if event.EntityType != "BattleLogic" || event.Name != "state" {
		return
	}

	var (
		update NestedUpdateValue
		okay   bool
	)

	if update, okay = event.Value.(NestedUpdateValue); !okay {
		err = fmt.Errorf("nested score update has type %T", event.Value)
		return
	}

	if len(update.Levels) == 3 && update.Levels[0] == "missions" && update.Levels[1] == "teamsScore" && update.Action == "set_key" && update.Key == "score" {
		var index int
		if index, okay = update.Levels[2].(int); !okay {
			err = fmt.Errorf("team score index has type %T", update.Levels[2])
			return
		}

		var score int64
		if score, okay = numericValue(update.Value); !okay {
			err = fmt.Errorf("updated team score has type %T", update.Value)
			return
		}

		var current BattleTeamScore = state.scores[index]
		state.setScore(index, current.TeamID, score, event.Clock)
	}

	return
}

// setScore updates a team score and records its exact event clock.
func (state *battleAccumulator) setScore(index int, teamID *int64, score int64, clock float32) {
	state.scores[index] = BattleTeamScore{TeamIndex: index, TeamID: teamID, Score: score}
	state.change(clock, "team_score", 0, teamID, BattleTeamScore{TeamIndex: index, TeamID: teamID, Score: score})
}

// minimapUpdates unpacks Toolkit's 11/11/8/1/1-bit minimap records.
func (state *battleAccumulator) minimapUpdates(event Event) (err error) {
	if len(event.Args) < 1 {
		err = fmt.Errorf("minimap update has no array argument")
		return
	}

	var (
		updates []any
		okay    bool
	)

	if updates, okay = event.Args[0].([]any); !okay {
		err = fmt.Errorf("minimap update arg has type %T", event.Args[0])
		return
	}

	for index, raw := range updates {
		var fields map[string]any
		if fields, okay = raw.(map[string]any); !okay {
			err = fmt.Errorf("minimap entry %d has type %T", index, raw)
			return
		}

		var shipID, packed int64
		if shipID, okay = numericValue(fields["vehicleID"]); !okay || shipID < 0 || shipID > int64(^uint32(0)) {
			err = fmt.Errorf("minimap vehicleID %v is invalid", fields["vehicleID"])
			return
		}

		if packed, okay = numericValue(fields["packedData"]); !okay || packed < 0 || packed > int64(^uint32(0)) {
			err = fmt.Errorf("minimap packedData %v is invalid", fields["packedData"])
			return
		}

		var (
			bits         uint32           = uint32(packed)
			x, y         uint32           = bits & 0x7ff, bits >> 11 & 0x7ff
			disappearing bool             = bits>>31 != 0
			sentinel     bool             = x == 0 && y == 0
			visible      bool             = !sentinel && !disappearing
			ship         *BattleShip      = state.ship(uint32(shipID))
			placement    MinimapPlacement = MinimapPlacement{
				X:               float32(x)/512 - 1.5,
				Y:               float32(y)/512 - 1.5,
				Heading:         float32(bits>>22&0xff)/256*360 - 180,
				Visible:         visible,
				VisibilityFlags: ship.VisibilityFlags,
				Clock:           event.Clock,
			}
		)

		if ship.IsInvisible != nil {
			placement.IsInvisible = *ship.IsInvisible
		}

		if disappearing && !sentinel {
			state.change(event.Clock, "minimap_ping", ship.EntityID, ship.TeamID, MinimapPing{X: placement.X, Y: placement.Y, Heading: placement.Heading, Unknown: bits>>30&1 != 0})
		}

		if !visible && ship.Minimap != nil {
			placement.X, placement.Y, placement.Heading = ship.Minimap.X, ship.Minimap.Y, ship.Minimap.Heading
		}

		ship.Minimap = &placement
		state.change(event.Clock, "minimap_position", ship.EntityID, ship.TeamID, placement)
	}

	return
}

// vehicleDeath records a kill without assuming the last health packet was zero.
func (state *battleAccumulator) vehicleDeath(event Event) (err error) {
	if len(event.Args) < 3 {
		err = fmt.Errorf("vehicle death has %d args, expected 3", len(event.Args))
		return
	}

	var (
		victim, killer, cause int64
		okay                  bool
	)

	if victim, okay = numericValue(event.Args[0]); !okay || victim < 0 || victim > int64(^uint32(0)) {
		err = fmt.Errorf("death victim %v is invalid", event.Args[0])
		return
	}

	if killer, okay = numericValue(event.Args[1]); !okay || killer < 0 || killer > int64(^uint32(0)) {
		err = fmt.Errorf("death killer %v is invalid", event.Args[1])
		return
	}

	if cause, okay = numericValue(event.Args[2]); !okay || cause < 0 || cause > int64(^uint32(0)) {
		err = fmt.Errorf("death cause %v is invalid", event.Args[2])
		return
	}

	var record BattleKill = BattleKill{Clock: event.Clock, VictimID: uint32(victim), KillerID: uint32(killer), Cause: uint32(cause)}
	state.battle.Kills = append(state.battle.Kills, record)

	var (
		ship  *BattleShip = state.ship(record.VictimID)
		alive bool        = false
	)

	ship.Alive = &alive
	state.change(event.Clock, "ship_destroyed", record.VictimID, ship.TeamID, record)
	return
}
