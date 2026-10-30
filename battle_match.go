package replayparser

import "fmt"

// matchProperty follows Toolkit's BattleLogic match-state property updates.
func (state *battleAccumulator) matchProperty(event Event) (err error) {
	switch event.Name {
	case "state":
		err = state.battleLogic(event.Value, event.Clock)
	case "battleResult":
		var (
			fields map[string]any
			okay   bool
			raw    any
			exists bool
		)

		if fields, okay = event.Value.(map[string]any); !okay {
			err = fmt.Errorf("battleResult has type %T", event.Value)
			return
		}

		if raw, exists = fields["winnerTeamId"]; exists {
			var winner int64
			if winner, okay = numericValue(raw); !okay {
				err = fmt.Errorf("winnerTeamId has type %T", raw)
				return
			}

			state.battle.WinningTeam = &winner
			state.change(event.Clock, "winning_team", 0, &winner, winner)
		}

		if raw, exists = fields["finishReason"]; exists {
			var reason int64
			if reason, okay = numericValue(raw); !okay {
				err = fmt.Errorf("finishReason has type %T", raw)
				return
			}

			state.battle.FinishReason = &reason
			state.change(event.Clock, "finish_reason", 0, nil, reason)
		}
	case "timeLeft", "battleStage", "duration":
		var (
			value int64
			okay  bool
		)

		if value, okay = numericValue(event.Value); !okay {
			err = fmt.Errorf("%s has type %T", event.Name, event.Value)
			return
		}

		switch event.Name {
		case "timeLeft":
			state.battle.TimeLeft = &value
		case "battleStage":
			state.battle.BattleStage = &value
		case "duration":
			state.battle.Duration = &value
		}

		state.change(event.Clock, event.Name, 0, nil, value)
	}

	return
}
