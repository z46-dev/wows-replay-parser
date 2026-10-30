package replayparser

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"encoding/binary"
	"io"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	wowsextractor "github.com/z46-dev/wows-extractor"
)

// appendTestFrame writes one Toolkit packet header and payload.
func appendTestFrame(stream []byte, id uint32, clock float32, payload []byte) (result []byte) {
	result = binary.LittleEndian.AppendUint32(stream, uint32(len(payload)))
	result = binary.LittleEndian.AppendUint32(result, id)
	result = binary.LittleEndian.AppendUint32(result, math.Float32bits(clock))
	result = append(result, payload...)
	return
}

// appendTestMethod writes an entity method's entity, exposed ID, length, and args.
func appendTestMethod(entityID, methodID uint32, args []byte) (payload []byte) {
	payload = binary.LittleEndian.AppendUint32(payload, entityID)
	payload = binary.LittleEndian.AppendUint32(payload, methodID)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(len(args)))
	payload = append(payload, args...)
	return
}

// TestAnalyzeFromReplay checks the complete archive-to-battle path in CI.
func TestAnalyzeFromReplay(t *testing.T) {
	var (
		archivePath string = filepath.Join(t.TempDir(), "build.zip")
		file        *os.File
		writer      *zip.Writer
		entry       io.Writer
		archive     *wowsextractor.Archive
		data        *GameData
		battle      *Battle
		err         error
	)

	if file, err = os.Create(archivePath); !assert.NoError(t, err) {
		return
	}

	writer = zip.NewWriter(file)
	if entry, err = writer.Create(".wows-extractor/manifest.json"); !assert.NoError(t, err) {
		return
	}

	if _, err = entry.Write([]byte(`{"formatVersion":2,"build":"13357625","clientVersion":"15.9.0.13357625"}`)); !assert.NoError(t, err) {
		return
	}

	if entry, err = writer.Create("content/GameParams.data"); !assert.NoError(t, err) {
		return
	}

	var params []byte
	if params, err = base64.StdEncoding.DecodeString("ARiTdQAHrFxaXFXIUFmRkhiE5CkSDSwDkLa9kL8tLzM1ILKkpyAOCOYMtkLU3MS8qWDMCr6shSmZUsA0WQtrmQoDPD2DsgFgimQtrGQsc/BgYGJgiGQtqmBrnHg="); !assert.NoError(t, err) {
		return
	}

	if _, err = entry.Write(params); !assert.NoError(t, err) {
		return
	}

	if err = writer.Close(); !assert.NoError(t, err) {
		return
	}

	if err = file.Close(); !assert.NoError(t, err) {
		return
	}

	if archive, err = wowsextractor.Load(archivePath); !assert.NoError(t, err) {
		return
	}

	defer archive.Close()
	if data, err = NewGameData(archive); !assert.NoError(t, err) {
		return
	}

	var (
		teamScoreType ArgType = ArgType{Kind: "FIXED_DICT", Properties: []ArgField{{Name: "teamId", Type: ArgType{Kind: "UINT8"}}, {Name: "score", Type: ArgType{Kind: "UINT16"}}}}
		teamsType     ArgType = ArgType{Kind: "ARRAY", Count: -1, Inner: &teamScoreType}
		missionsType  ArgType = ArgType{Kind: "FIXED_DICT", Properties: []ArgField{{Name: "teamsScore", Type: teamsType}}}
		stateType     ArgType = ArgType{Kind: "FIXED_DICT", Properties: []ArgField{{Name: "missions", Type: missionsType}}}
	)

	data.specs = []EntitySpec{
		{Name: "Avatar", ClientMethods: []MethodSpec{
			{Name: "onArenaStateReceived", Args: []ArgType{{Kind: "INT64"}, {Kind: "INT8"}, {Kind: "BLOB"}, {Kind: "BLOB"}, {Kind: "BLOB"}}},
			{Name: "receiveVehicleDeath", Args: []ArgType{{Kind: "INT32"}, {Kind: "INT32"}, {Kind: "UINT32"}}, FixedSize: 12},
			{Name: "onBattleEnd"},
		}},
		{Name: "BattleLogic", Properties: []PropertySpec{{Name: "state", Type: stateType}}},
	}

	data.specsLoaded = true

	var roster []byte
	if roster, err = base64.StdEncoding.DecodeString("gAJdcQBdcQEoSwBLe4ZxAksFWAMAAABBQkNxA4ZxBEsLTcgBhnEFSxGIhnEGSxhNQJyGcQdLGVgHAAAAQ2FwdGFpbnEIhnEJSyJNvAKGcQpLI00gA4ZxC0slSwGGcQxlYS4="); !assert.NoError(t, err) {
		return
	}

	var emptyRoster []byte
	if emptyRoster, err = base64.StdEncoding.DecodeString("gAJdcQAu"); !assert.NoError(t, err) {
		return
	}

	var stream, payload, args []byte
	payload = binary.LittleEndian.AppendUint32(payload, 1)
	payload = binary.LittleEndian.AppendUint16(payload, 1)
	payload = binary.LittleEndian.AppendUint32(payload, 0)
	stream = appendTestFrame(stream, 0x26, 1, payload)
	args = binary.LittleEndian.AppendUint64(args, 999)
	args = append(args, 0, 0)
	args = append(args, byte(len(roster)))
	args = append(args, roster...)
	args = append(args, byte(len(emptyRoster)))
	args = append(args, emptyRoster...)
	stream = appendTestFrame(stream, 0x08, 2, appendTestMethod(1, 0, args))

	payload = nil
	payload = binary.LittleEndian.AppendUint32(payload, 2)
	payload = binary.LittleEndian.AppendUint16(payload, 2)
	payload = append(payload, make([]byte, 4+4+12+12+4)...)
	payload = append(payload, 0)
	stream = appendTestFrame(stream, 0x05, 3, payload)

	var scoreValue []byte = []byte{2, 0, 44, 1, 1, 104, 1}
	payload = appendTestMethod(2, 0, scoreValue)
	stream = appendTestFrame(stream, 0x07, 4, payload)
	payload = nil
	payload = binary.LittleEndian.AppendUint32(payload, 700)
	payload = binary.LittleEndian.AppendUint32(payload, 0)

	for _, coordinate := range []float32{10, 20, 30, 0, 0, 0, 1, 2, 3} {
		payload = binary.LittleEndian.AppendUint32(payload, math.Float32bits(coordinate))
	}

	payload = append(payload, 1)
	stream = appendTestFrame(stream, 0x0a, 5, payload)
	args = nil
	args = binary.LittleEndian.AppendUint32(args, 700)
	args = binary.LittleEndian.AppendUint32(args, 701)
	args = binary.LittleEndian.AppendUint32(args, 17)
	stream = appendTestFrame(stream, 0x08, 6, appendTestMethod(1, 1, args))
	stream = appendTestFrame(stream, 0x08, 7, appendTestMethod(1, 2, nil))

	var replay *Replay = &Replay{Metadata: Metadata{ClientVersionFromExe: "15,9,0,13357625"}, packets: stream}
	if battle, err = replay.Analyze(context.Background(), data); !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, int64(999), battle.ArenaID)
	assert.True(t, battle.Ended)
	if !assert.Len(t, battle.Players, 1) || !assert.Len(t, battle.Ships, 1) {
		return
	}

	assert.Equal(t, "SHIP", battle.Players[0].ShipName)
	assert.Equal(t, "Captain", battle.Players[0].Name)
	assert.Equal(t, int64(360), battle.TeamScores[1].Score)
	assert.Equal(t, 1, battle.TeamScores[1].TeamIndex)
	assert.Equal(t, int64(1), *battle.TeamScores[1].TeamID)
	assert.Equal(t, Vector3{X: 10, Y: 20, Z: 30}, battle.Ships[0].World.Position)
	assert.False(t, *battle.Ships[0].Alive)
	assert.Equal(t, BattleKill{Clock: 6, VictimID: 700, KillerID: 701, Cause: 17}, battle.Kills[0])
}

// TestRosterPickle checks the real Python protocol-2 shape used by arena blobs.
func TestRosterPickle(t *testing.T) {
	var (
		blob []byte
		err  error
	)

	if blob, err = base64.StdEncoding.DecodeString("gAJdcQBdcQEoSwBLe4ZxAksFWAMAAABBQkNxA4ZxBEsLTcgBhnEFSxGIhnEGSxhNQJyGcQdLGVgHAAAAQ2FwdGFpbnEIhnEJSyJNvAKGcQpLI00gA4ZxC0slSwGGcQxlYS4="); !assert.NoError(t, err) {
		return
	}

	var players []BattlePlayer
	if players, err = loadRosterBlob(blob, true, false); !assert.NoError(t, err) {
		return
	}

	if !assert.Len(t, players, 1) {
		return
	}

	assert.Equal(t, "Captain", players[0].Name)
	assert.Equal(t, uint32(700), players[0].EntityID)
	assert.Equal(t, uint32(800), players[0].ShipParamID)
}

// TestRosterLayouts checks Toolkit's 15.8 and 15.9 player and bot index shifts.
func TestRosterLayouts(t *testing.T) {
	var (
		humanFields map[int64]any = map[int64]any{
			0: int64(123), 5: "ABC", 11: int64(456), 17: true,
			24: int64(40000), 25: "Captain", 33: int64(700), 34: int64(800),
			35: int64(900), 36: int64(1), 37: int64(0),
		}
		player BattlePlayer
		err    error
	)

	if player, err = playerFromFields(humanFields, false, false); !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, uint32(700), player.EntityID)
	assert.Equal(t, uint32(800), player.ShipParamID)
	assert.Equal(t, int64(1), player.TeamID)
	assert.Equal(t, "ABC", player.Clan)
	assert.Equal(t, true, *player.Connected)

	if player, err = playerFromFields(humanFields, true, false); !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, uint32(800), player.EntityID)
	assert.Equal(t, uint32(900), player.ShipParamID)
	assert.Equal(t, int64(0), player.TeamID)

	var botFields map[int64]any = map[int64]any{
		0: int64(0), 5: "", 10: int64(777), 18: int64(22000),
		19: "Bot", 23: int64(10), 24: int64(20), 25: int64(30),
		26: int64(1), 27: int64(0),
	}

	if player, err = playerFromFields(botFields, false, true); !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, uint32(10), player.EntityID)
	assert.Equal(t, int64(1), player.TeamID)
	if player, err = playerFromFields(botFields, true, true); !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, uint32(20), player.EntityID)
	assert.Equal(t, int64(0), player.TeamID)
}

// TestBattleFacts checks state, score-path, minimap, and death transitions.
func TestBattleFacts(t *testing.T) {
	var (
		state battleAccumulator = battleAccumulator{
			players: make(map[uint32]BattlePlayer),
			ships:   make(map[uint32]*BattleShip),
			world:   make(map[uint32]WorldPlacement),
			scores:  make(map[int]BattleTeamScore),
			modern:  true,
		}
		err error
	)

	err = state.apply(Event{Type: "entity_create", EntityType: "BattleLogic", Clock: 10, Value: EntityCreateValue{Properties: map[string]any{"state": map[string]any{"missions": map[string]any{"teamsScore": []any{map[string]any{"teamId": uint8(4), "score": int64(300)}, map[string]any{"teamId": uint8(8), "score": int64(300)}}}}}}})
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, int64(300), state.scores[0].Score)
	err = state.apply(Event{Type: "nested_property_update", EntityType: "BattleLogic", Name: "state", Clock: 12, Value: NestedUpdateValue{Levels: []any{"missions", "teamsScore", 1}, Action: "set_key", Key: "score", Value: uint16(360)}})
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, int64(360), state.scores[1].Score)
	assert.Equal(t, int64(8), *state.scores[1].TeamID)
	err = state.apply(Event{Type: "entity_property", EntityType: "BattleLogic", Name: "battleResult", Clock: 12.5, Value: map[string]any{"winnerTeamId": int8(1), "finishReason": uint8(13)}})
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, int64(1), *state.battle.WinningTeam)
	assert.Equal(t, int64(13), *state.battle.FinishReason)

	err = state.apply(Event{Type: "position", EntityID: 7, Clock: 12.8, Value: PositionValue{Position: Vector3{X: 2}}})
	if !assert.NoError(t, err) {
		return
	}

	state.addPlayer(BattlePlayer{Name: "Captain", EntityID: 7, TeamID: 1, MaxHealth: 100}, 13)
	assert.Equal(t, float32(2), state.ships[7].World.Position.X)

	err = state.updateRoomPlayer(7, state.players[7], map[int64]any{17: false, 24: int64(120), 37: int64(0)}, 13.5)
	if !assert.NoError(t, err) {
		return
	}

	assert.False(t, *state.players[7].Connected)
	assert.Equal(t, int64(0), state.players[7].TeamID)
	assert.Equal(t, float64(120), *state.ships[7].MaxHealth)

	var packed uint32 = 512 | 768<<11 | 128<<22
	err = state.apply(Event{Type: "entity_method", Name: "updateMinimapVisionInfo", Clock: 14, Args: []any{[]any{map[string]any{"vehicleID": uint32(7), "packedData": packed}}}})
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, float32(-0.5), state.ships[7].Minimap.X)
	assert.Equal(t, float32(0), state.ships[7].Minimap.Y)
	assert.True(t, state.ships[7].Minimap.Visible)

	err = state.apply(Event{Type: "entity_method", Name: "updateMinimapVisionInfo", Clock: 15, Args: []any{[]any{map[string]any{"vehicleID": uint32(7), "packedData": uint32(0)}}}})
	if !assert.NoError(t, err) {
		return
	}

	assert.False(t, state.ships[7].Minimap.Visible)
	assert.Equal(t, float32(-0.5), state.ships[7].Minimap.X)

	err = state.apply(Event{Type: "entity_method", Name: "updateMinimapVisionInfo", Clock: 15.5, Args: []any{[]any{map[string]any{"vehicleID": uint32(7), "packedData": packed | 1<<31}}}})
	if !assert.NoError(t, err) {
		return
	}

	assert.Equal(t, "minimap_ping", state.battle.Timeline[len(state.battle.Timeline)-2].Kind)
	assert.False(t, state.ships[7].Minimap.Visible)

	err = state.apply(Event{Type: "entity_method", Name: "receiveVehicleDeath", Clock: 16, Args: []any{int64(7), int64(8), uint32(17)}})
	assert.NoError(t, err)
	assert.False(t, *state.ships[7].Alive)
	assert.Equal(t, BattleKill{Clock: 16, VictimID: 7, KillerID: 8, Cause: 17}, state.battle.Kills[0])

	err = state.apply(Event{ID: ^uint32(0), Type: "unknown", Raw: make([]byte, 16)})
	assert.NoError(t, err)
	assert.Empty(t, state.battle.Issues)

	err = state.apply(Event{ID: 99, Type: "unknown", Raw: []byte{1}})
	assert.NoError(t, err)
	assert.Equal(t, "unmapped packet ID", state.battle.Issues[0].Diagnostic)

	err = state.apply(Event{ID: 8, Type: "entity_method", Name: "receiveGunSyncRotations", Diagnostic: "decode failed"})
	assert.NoError(t, err)
	assert.Equal(t, "decode failed", state.battle.Issues[1].Diagnostic)

	err = state.apply(Event{Type: "entity_method", Name: "onArenaStateReceived", Diagnostic: "trailing bytes"})
	assert.Error(t, err)
}
