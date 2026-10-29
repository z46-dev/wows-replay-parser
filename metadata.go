package replayparser

import "encoding/json"

type (
	// VehicleInfo identifies a participant and their ship in the replay header.
	VehicleInfo struct {
		ShipID   uint64 `json:"shipId"`
		Relation uint32 `json:"relation"`
		ID       uint64 `json:"id"`
		Name     string `json:"name"`
	}

	// Metadata contains the replay's plaintext match setup and preserves its raw JSON.
	Metadata struct {
		MatchGroup           *string             `json:"matchGroup,omitempty"`
		GameMode             uint32              `json:"gameMode"`
		GameType             *string             `json:"gameType,omitempty"`
		ClientVersionFromExe string              `json:"clientVersionFromExe"`
		ScenarioUICategoryID *uint32             `json:"scenarioUiCategoryId,omitempty"`
		MapDisplayName       string              `json:"mapDisplayName"`
		MapID                uint32              `json:"mapId"`
		ClientVersionFromXML string              `json:"clientVersionFromXml"`
		WeatherParams        map[string][]string `json:"weatherParams,omitempty"`
		Duration             uint32              `json:"duration"`
		GameLogic            *string             `json:"gameLogic,omitempty"`
		Name                 string              `json:"name"`
		Scenario             string              `json:"scenario"`
		PlayerID             uint64              `json:"playerID"`
		Vehicles             []VehicleInfo       `json:"vehicles"`
		PlayersPerTeam       uint32              `json:"playersPerTeam"`
		DateTime             string              `json:"dateTime"`
		MapName              string              `json:"mapName"`
		PlayerName           string              `json:"playerName"`
		ScenarioConfigID     uint32              `json:"scenarioConfigId"`
		TeamsCount           uint32              `json:"teamsCount"`
		Logic                *string             `json:"logic,omitempty"`
		PlayerVehicle        string              `json:"playerVehicle"`
		BattleDuration       *uint32             `json:"battleDuration,omitempty"`
		Raw                  json.RawMessage     `json:"-"`
	}
)
