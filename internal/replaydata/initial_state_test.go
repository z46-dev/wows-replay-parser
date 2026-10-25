package replaydata

import (
	"encoding/binary"
	"encoding/xml"
	"testing"
)

func TestDecodeBattleLogicInitialScores(t *testing.T) {
	const aliasesXML string = `<root>
		<BATTLE_LOGIC_STATE>FIXED_DICT<Properties><missions><Type>MISSIONS_STATE</Type></missions></Properties></BATTLE_LOGIC_STATE>
		<MISSIONS_STATE>FIXED_DICT<Properties><teamsScore><Type>ARRAY<of>TEAM_SCORE</of></Type></teamsScore></Properties><AllowNone>true</AllowNone></MISSIONS_STATE>
		<TEAM_SCORE>FIXED_DICT<Properties><teamId><Type>UINT8</Type></teamId><score><Type>UINT16</Type></score></Properties></TEAM_SCORE>
	</root>`
	var definitions xmlNode
	if err := xml.Unmarshal([]byte(aliasesXML), &definitions); err != nil {
		t.Fatalf("xml.Unmarshal() error = %v", err)
	}

	var state []byte = []byte{2, 0}
	var duration [2]byte
	binary.LittleEndian.PutUint16(duration[:], 1200)
	state = append(state, duration[:]...)
	state = append(state, 1, 4, 2, 0, 44, 1, 1, 44, 1)
	var schema *Schema = &Schema{
		entities:      map[uint16]string{7: "BattleLogic"},
		properties:    map[string][]string{"BattleLogic": {"duration", "state"}},
		propertyTypes: map[string][]string{"BattleLogic": {"UINT16", "BATTLE_LOGIC_STATE"}},
		aliasNodes:    aliasDefinitions(definitions.Children),
	}

	var scores []InitialTeamScore
	var ok bool
	if scores, ok = schema.DecodeBattleLogicInitialScores(7, state); !ok {
		t.Fatal("DecodeBattleLogicInitialScores() did not recognize the state")
	}
	if len(scores) != 2 || scores[0] != (InitialTeamScore{TeamIndex: 0, Score: 300}) || scores[1] != (InitialTeamScore{TeamIndex: 1, Score: 300}) {
		t.Fatalf("decoded initial scores = %+v, want 300 for both teams", scores)
	}
	if _, ok = schema.DecodeBattleLogicInitialScores(7, state[:len(state)-1]); ok {
		t.Fatal("DecodeBattleLogicInitialScores() accepted a truncated state")
	}
	state[5] = 0
	if _, ok = schema.DecodeBattleLogicInitialScores(7, state); ok {
		t.Fatal("DecodeBattleLogicInitialScores() accepted an absent missions state")
	}
}
