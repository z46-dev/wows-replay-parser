package events

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/z46-dev/wows-replay-parser"
)

// decodeBattleResults verifies and retains the packet's length-prefixed JSON payload.
func decodeBattleResults(packet replayparser.Packet) (event Event, err error) {
	if len(packet.Payload) < 4 {
		err = fmt.Errorf("battle-results packet has %d-byte payload, expected length prefix", len(packet.Payload))
		return
	}

	var data []byte = packet.Payload[4:]
	if uint64(binary.LittleEndian.Uint32(packet.Payload[0:4])) != uint64(len(data)) {
		err = fmt.Errorf("battle-results JSON length does not match payload")
		return
	}

	if !json.Valid(data) {
		err = fmt.Errorf("battle-results packet is not valid JSON")
		return
	}

	var summary *BattleSummary
	if summary, err = decodeBattleSummary(data); err != nil {
		return
	}

	event = Event{
		Clock: packet.Clock,
		Type:  BattleResultsEvent,
		Results: &BattleResults{
			Raw:     append(json.RawMessage(nil), data...),
			Summary: summary,
		},
	}

	return
}

// decodeBattleSummary extracts stable result fields while retaining the complete raw result object.
func decodeBattleSummary(data []byte) (summary *BattleSummary, err error) {
	var result struct {
		ArenaUniqueID     json.RawMessage              `json:"arenaUniqueID"`
		AccountDBID       json.RawMessage              `json:"accountDBID"`
		CommonList        []json.RawMessage            `json:"commonList"`
		PlayersPublicInfo map[string][]json.RawMessage `json:"playersPublicInfo"`
	}

	if err = json.Unmarshal(data, &result); err != nil {
		err = fmt.Errorf("battle-results JSON object is invalid: %w", err)
		return
	}

	summary = &BattleSummary{
		ArenaUniqueID: uintValue(result.ArenaUniqueID),
		AccountDBID:   uintValue(result.AccountDBID),
		Common:        decodeBattleCommon(result.CommonList),
		Players:       make(map[uint64]BattlePlayerResult, len(result.PlayersPublicInfo)),
	}

	for accountID, values := range result.PlayersPublicInfo {
		var player BattlePlayerResult = decodeBattlePlayer(values)
		if player.AccountDBID == 0 {
			player.AccountDBID = uintValue(json.RawMessage(accountID))
		}

		if player.AccountDBID != 0 {
			summary.Players[player.AccountDBID] = player
		}
	}

	return
}

// decodeBattleCommon maps the documented stable common-list indices.
func decodeBattleCommon(values []json.RawMessage) (common BattleCommon) {
	common = BattleCommon{
		ArenaID:       uintValue(valueAt(values, 0)),
		ClusterID:     uintValue(valueAt(values, 1)),
		StartUnixTime: uintValue(valueAt(values, 2)),
		WinnerTeamID:  uintValue(valueAt(values, 3)),
		WinTypeID:     uintValue(valueAt(values, 4)),
		Duration:      uintValue(valueAt(values, 8)),
		MapTypeID:     uintValue(valueAt(values, 9)),
		ScenarioName:  stringValue(valueAt(values, 10)),
		GameMode:      uintValue(valueAt(values, 12)),
	}

	return
}

// decodeBattlePlayer maps stable public-player result indices shared by supported 15.x builds.
func decodeBattlePlayer(values []json.RawMessage) (player BattlePlayerResult) {
	player = BattlePlayerResult{
		AccountDBID:     uintValue(valueAt(values, 0)),
		Name:            stringValue(valueAt(values, 1)),
		ClanTag:         stringValue(valueAt(values, 3)),
		TeamID:          uintValue(valueAt(values, 6)),
		VehicleTypeID:   uintValue(valueAt(values, 7)),
		MaxHealth:       floatValue(valueAt(values, 15)),
		RemainingHealth: floatValue(valueAt(values, 20)),
		IsAlive:         boolValue(valueAt(values, 21)),
		LifetimeSeconds: floatValue(valueAt(values, 22)),
		Distance:        floatValue(valueAt(values, 23)),
		CapturePoints:   floatValue(valueAt(values, 24)),
		DefencePoints:   floatValue(valueAt(values, 25)),
		ShipsKilled:     floatValue(valueAt(values, 32)),
		KillerAccountID: uintValue(valueAt(values, 408)),
		KillerVehicleID: uintValue(valueAt(values, 409)),
		SpottingDamage:  floatValue(valueAt(values, 412)),
		Damage:          floatValue(valueAt(values, 426)),
	}

	return
}

// valueAt returns an empty raw value when an optional results list is shorter than expected.
func valueAt(values []json.RawMessage, index int) (value json.RawMessage) {
	if index >= 0 && index < len(values) {
		value = values[index]
	}

	return
}

// uintValue reads an optional JSON numeric value without treating malformed optional data as fatal.
func uintValue(value json.RawMessage) (number uint64) {
	var parsed float64
	if json.Unmarshal(value, &parsed) == nil && parsed >= 0 {
		number = uint64(parsed)
	}

	return
}

// floatValue reads an optional JSON numeric value without treating malformed optional data as fatal.
func floatValue(value json.RawMessage) (number float64) {
	if err := json.Unmarshal(value, &number); err != nil {
		number = 0
	}

	return
}

// boolValue reads an optional JSON boolean value without treating malformed optional data as fatal.
func boolValue(value json.RawMessage) (flag bool) {
	if err := json.Unmarshal(value, &flag); err != nil {
		flag = false
	}

	return
}

// stringValue reads an optional JSON string value without treating malformed optional data as fatal.
func stringValue(value json.RawMessage) (text string) {
	if err := json.Unmarshal(value, &text); err != nil {
		text = ""
	}

	return
}
