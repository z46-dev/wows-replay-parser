package replaydata

import (
	"encoding/binary"
	"testing"
)

func TestDecodeBattleLogicTeamScoreUpdate(t *testing.T) {
	var schema Schema = Schema{
		entities: map[uint16]string{7: "BattleLogic"},
		properties: map[string][]string{
			"BattleLogic": {"duration", "state", "teams"},
		},
		aliasFields: map[string][]property{
			"BATTLE_LOGIC_STATE": {{Name: "other"}, {Name: "missions"}},
			"MISSIONS_STATE":     {{Name: "hold"}, {Name: "teamsScore"}},
			"TEAM_SCORE":         {{Name: "teamId"}, {Name: "score"}},
		},
	}

	var payload []byte = appendScoreBits(1, 0, 1, 1, 1, 1, 1, 1, 1, 0, 1)
	var score [2]byte
	binary.LittleEndian.PutUint16(score[:], 713)
	payload = append(payload, score[:]...)
	if teamIndex, decodedScore, ok := schema.DecodeBattleLogicTeamScoreUpdate(7, payload); !ok || teamIndex != 1 || decodedScore != 713 {
		t.Fatalf("decoded team score = (%d, %d, %t), want (1, 713, true)", teamIndex, decodedScore, ok)
	}

	if _, _, ok := schema.DecodeBattleLogicTeamScoreUpdate(8, payload); ok {
		t.Fatal("decoded team score for a non-BattleLogic entity")
	}
	if _, _, ok := schema.DecodeBattleLogicTeamScoreUpdate(7, payload[:1]); ok {
		t.Fatal("decoded a truncated team score update")
	}
}

func appendScoreBits(values ...int) (payload []byte) {
	var bitOffset int
	for _, value := range values {
		if bitOffset%8 == 0 {
			payload = append(payload, 0)
		}
		if value != 0 {
			payload[bitOffset/8] |= byte(1 << uint(7-bitOffset%8))
		}
		bitOffset++
	}
	return
}
