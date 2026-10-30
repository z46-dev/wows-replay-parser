package replayparser

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

// BattlePlayer is one participant from Toolkit's arena roster.
type BattlePlayer struct {
	AccountID   int64   `json:"accountId"`
	PlayerID    int64   `json:"playerId"`
	EntityID    uint32  `json:"entityId"`
	ShipParamID uint32  `json:"shipParamId"`
	ShipName    string  `json:"shipName"`
	Name        string  `json:"name"`
	Clan        string  `json:"clan,omitempty"`
	TeamID      int64   `json:"teamId"`
	MaxHealth   int64   `json:"maxHealth"`
	Bot         bool    `json:"bot"`
	Connected   *bool   `json:"connected,omitempty"`
	Alive       *bool   `json:"alive,omitempty"`
	Relation    *uint32 `json:"relation,omitempty"`
}

// WorldPlacement is a ship's last reported world position and rotation.
type WorldPlacement struct {
	Position Vector3   `json:"position"`
	Rotation Rotation3 `json:"rotation"`
	Clock    float32   `json:"clock"`
}

// MinimapPlacement is a ship's last visible map position.
type MinimapPlacement struct {
	X               float32 `json:"x"`
	Y               float32 `json:"y"`
	Heading         float32 `json:"heading"`
	Visible         bool    `json:"visible"`
	VisibilityFlags *uint32 `json:"visibilityFlags,omitempty"`
	IsInvisible     bool    `json:"isInvisible"`
	Clock           float32 `json:"clock"`
}

// MinimapPing preserves a one-shot hydrophone position without marking a ship visible.
type MinimapPing struct {
	X       float32 `json:"x"`
	Y       float32 `json:"y"`
	Heading float32 `json:"heading"`
	Unknown bool    `json:"unknown"`
}

// BattleShip is the final observed state of one ship entity.
type BattleShip struct {
	EntityID        uint32            `json:"entityId"`
	ShipParamID     uint32            `json:"shipParamId,omitempty"`
	ShipName        string            `json:"shipName,omitempty"`
	TeamID          *int64            `json:"teamId,omitempty"`
	Health          *float64          `json:"health,omitempty"`
	MaxHealth       *float64          `json:"maxHealth,omitempty"`
	Alive           *bool             `json:"alive,omitempty"`
	VisibilityFlags *uint32           `json:"visibilityFlags,omitempty"`
	IsInvisible     *bool             `json:"isInvisible,omitempty"`
	World           *WorldPlacement   `json:"world,omitempty"`
	Minimap         *MinimapPlacement `json:"minimap,omitempty"`
}

// BattleTeamScore is one team's most recently reported score.
type BattleTeamScore struct {
	TeamIndex int    `json:"teamIndex"`
	TeamID    *int64 `json:"teamId,omitempty"`
	Score     int64  `json:"score"`
}

// BattleKill records Toolkit's receiveVehicleDeath event.
type BattleKill struct {
	Clock    float32 `json:"clock"`
	VictimID uint32  `json:"victimId"`
	KillerID uint32  `json:"killerId"`
	Cause    uint32  `json:"cause"`
}

// BattleChange is one ordered timeline change to a battle fact.
type BattleChange struct {
	Clock    float32 `json:"clock"`
	Kind     string  `json:"kind"`
	EntityID uint32  `json:"entityId,omitempty"`
	TeamID   *int64  `json:"teamId,omitempty"`
	Value    any     `json:"value,omitempty"`
}

// BattleIssue records packet data that the typed decoder did not fully identify.
type BattleIssue struct {
	PacketID       uint32  `json:"packetId"`
	Clock          float32 `json:"clock"`
	Type           string  `json:"type"`
	Name           string  `json:"name,omitempty"`
	Diagnostic     string  `json:"diagnostic"`
	RemainingBytes int     `json:"remainingBytes,omitempty"`
}

// Battle is the arena roster, final ship state, and ordered fact timeline.
type Battle struct {
	Metadata      Metadata          `json:"metadata"`
	ArenaID       int64             `json:"arenaId"`
	Players       []BattlePlayer    `json:"players"`
	Ships         []BattleShip      `json:"ships"`
	TeamScores    []BattleTeamScore `json:"teamScores"`
	Kills         []BattleKill      `json:"kills"`
	Timeline      []BattleChange    `json:"timeline"`
	Ended         bool              `json:"ended"`
	WinningTeam   *int64            `json:"winningTeam,omitempty"`
	FinishReason  *int64            `json:"finishReason,omitempty"`
	TimeLeft      *int64            `json:"timeLeft,omitempty"`
	BattleStage   *int64            `json:"battleStage,omitempty"`
	Duration      *int64            `json:"duration,omitempty"`
	BattleResults string            `json:"battleResults,omitempty"`
	Issues        []BattleIssue     `json:"issues,omitempty"`
}

type battleAccumulator struct {
	battle    Battle
	players   map[uint32]BattlePlayer
	ships     map[uint32]*BattleShip
	world     map[uint32]WorldPlacement
	scores    map[int]BattleTeamScore
	arenaSeen bool
	modern    bool
}

// Analyze builds battle facts from the same typed event walk used by the CLI.
func (replay *Replay) Analyze(ctx context.Context, data *GameData) (battle *Battle, err error) {
	if replay == nil || data == nil || ctx == nil {
		err = fmt.Errorf("replay, game data, and context are required")
		return
	}

	var version string = strings.ReplaceAll(replay.Metadata.ClientVersionFromExe, ",", ".")
	if !strings.HasPrefix(version, "15.8.") && !strings.HasPrefix(version, "15.9.") {
		err = fmt.Errorf("battle analysis has no verified roster layout for version %s", version)
		return
	}

	var state battleAccumulator = battleAccumulator{
		battle:  Battle{Metadata: replay.Metadata},
		players: make(map[uint32]BattlePlayer),
		ships:   make(map[uint32]*BattleShip),
		world:   make(map[uint32]WorldPlacement),
		scores:  make(map[int]BattleTeamScore),
		modern:  strings.HasPrefix(version, "15.9."),
	}

	if err = replay.WalkEvents(ctx, data, state.apply); err != nil {
		return
	}

	if !state.arenaSeen {
		err = fmt.Errorf("replay has no decoded arena roster")
		return
	}

	if err = state.resolveShipNames(data); err != nil {
		return
	}

	var changes []BattleChange = state.battle.Timeline[:0]
	for _, change := range state.battle.Timeline {
		if change.Kind == "world_position" && state.ships[change.EntityID] == nil {
			continue
		}

		changes = append(changes, change)
	}

	state.battle.Timeline = changes
	for _, player := range state.players {
		state.battle.Players = append(state.battle.Players, player)
	}

	for _, ship := range state.ships {
		state.battle.Ships = append(state.battle.Ships, *ship)
	}

	for _, score := range state.scores {
		state.battle.TeamScores = append(state.battle.TeamScores, score)
	}

	sort.Slice(state.battle.Players, func(i, j int) bool { return state.battle.Players[i].EntityID < state.battle.Players[j].EntityID })
	sort.Slice(state.battle.Ships, func(i, j int) bool { return state.battle.Ships[i].EntityID < state.battle.Ships[j].EntityID })
	sort.Slice(state.battle.TeamScores, func(i, j int) bool {
		return state.battle.TeamScores[i].TeamIndex < state.battle.TeamScores[j].TeamIndex
	})

	battle = &state.battle
	return
}
