# wows-replay-parser

A Go library for reading World of Warships replay files.

## Current support

The root package parses the replay container and returns typed match-setup metadata plus raw packet frames. For client builds 15.8.0.13187581 and 15.9.0.13357625, the `events` subpackage decodes replay setup, entity lifecycle, movement, player controls, camera state, map metadata, post-battle results, and verified vehicle health changes. The metadata header does not contain post-battle results in the inspected samples; the event stream supplies them instead.

```go
data, err := os.ReadFile("battle.wowsreplay")
if err != nil {
	return err
}

replay, err := replayparser.Parse(data)
if err != nil {
	return err
}
fmt.Printf("%s: %d packets\n", replay.ClientVersion, len(replay.Packets))
```

Version descriptors are kept in `internal/replayformat/v*.go`; shared parsing remains in the root package. Run `go test ./...` to parse the checked-in 15.8 and 15.9 fixture replays.

The CLI is in `cmd/wows-replay-parser`. Run it with `go run ./cmd/wows-replay-parser path/to/replay.wowsreplay`; it prints replay metadata and packet-type counts. Use `--events-json` to export decoded events, optionally filtered with `--event-type entity_create`, `--event-type entity_property`, `--event-type entity_method`, `--event-type weapon_lock`, `--entity-id ID`, or `--my-ship` for player-ship movement updates. Property and method events expose validated envelopes and base64-encoded raw payloads; decoding their remaining payloads requires build-specific entity definitions.

For example, export every observed health decrease for the replaying ship with `go run ./cmd/wows-replay-parser --events-json --event-type ship_damage --my-ship path/to/replay.wowsreplay`. Each event includes the current health, previous health, and negative delta. Use `--event-type ship_repair` for positive deltas, or `--event-type ship_health` for every health update.

Filter with `--event-type battle_results` to export verified post-battle summaries, including match outcome and duration plus public player statistics such as remaining health, kills, damage, spotting damage, and killer IDs. The original result JSON remains available alongside the typed summary.
