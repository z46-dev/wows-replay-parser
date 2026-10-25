package replaydata

import (
	"encoding/binary"
	"math/bits"
)

type scoreBitReader struct {
	data      []byte
	bitOffset int
}

// DecodeBattleLogicTeamScoreUpdate recognizes authoritative BattleLogic team-score updates.
func (schema *Schema) DecodeBattleLogicTeamScoreUpdate(entityTypeID uint16, payload []byte) (teamIndex int64, score uint16, ok bool) {
	if schema.EntityName(entityTypeID) != "BattleLogic" {
		return
	}

	var propertyIndex int = -1
	for index, propertyName := range schema.properties["BattleLogic"] {
		if propertyName == "state" {
			propertyIndex = index
			break
		}
	}
	if propertyIndex < 0 {
		return
	}

	var reader scoreBitReader = scoreBitReader{data: payload}
	if reader.read(1) != 1 || reader.read(bitWidth(len(schema.properties["BattleLogic"]))) != propertyIndex {
		return
	}

	var (
		missionsIndex int
		teamsIndex    int
		scoreIndex    int
	)
	if missionsIndex, ok = fieldIndex(schema.aliasFields["BATTLE_LOGIC_STATE"], "missions"); !ok {
		return
	}
	if teamsIndex, ok = fieldIndex(schema.aliasFields["MISSIONS_STATE"], "teamsScore"); !ok {
		return
	}
	if scoreIndex, ok = fieldIndex(schema.aliasFields["TEAM_SCORE"], "score"); !ok {
		return
	}

	if reader.read(1) != 1 || reader.read(bitWidth(len(schema.aliasFields["BATTLE_LOGIC_STATE"]))) != missionsIndex {
		ok = false
		return
	}
	if reader.read(1) != 1 || reader.read(bitWidth(len(schema.aliasFields["MISSIONS_STATE"]))) != teamsIndex {
		ok = false
		return
	}
	if reader.read(1) != 1 {
		ok = false
		return
	}
	// shortcut: the HUD supports standard two-team battles; use decoded initial state for modes with a different team count.
	teamIndex = int64(reader.read(1))
	if reader.read(1) != 0 || reader.read(bitWidth(len(schema.aliasFields["TEAM_SCORE"]))) != scoreIndex {
		ok = false
		return
	}

	reader.align()
	if reader.remaining() < 16 {
		ok = false
		return
	}

	score = binary.LittleEndian.Uint16(payload[reader.bitOffset/8:])
	ok = true
	return
}

func fieldIndex(fields []property, name string) (index int, ok bool) {
	for index, field := range fields {
		if field.Name == name {
			ok = true
			return index, ok
		}
	}
	return
}

func bitWidth(count int) (width int) {
	if count > 1 {
		width = bits.Len(uint(count - 1))
	}
	return
}

func (reader *scoreBitReader) read(count int) (value int) {
	if count < 0 || reader.remaining() < count {
		reader.bitOffset = len(reader.data) * 8
		return
	}
	for range count {
		var (
			byteIndex int  = reader.bitOffset / 8
			bitIndex  uint = uint(7 - reader.bitOffset%8)
		)
		value = (value << 1) | int((reader.data[byteIndex]>>bitIndex)&1)
		reader.bitOffset++
	}
	return
}

func (reader *scoreBitReader) align() {
	reader.bitOffset = (reader.bitOffset + 7) &^ 7
}

func (reader *scoreBitReader) remaining() (count int) {
	count = len(reader.data)*8 - reader.bitOffset
	return
}
