# wows-replay-parser

A Go library for reading World of Warships replay files.

## Current support

The root package parses the replay container and returns typed match-setup metadata plus raw packet frames. The `events` subpackage currently decodes entity-position updates and validates the raw battle-results message for client builds 15.8.0.13187581 and 15.9.0.13357625. The metadata header does not contain post-battle results in the inspected samples; typed result fields and additional gameplay events remain to be decoded.

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

The initial CLI is in `cmd/wows-replay-parser`. Run it with `go run ./cmd/wows-replay-parser path/to/replay.wowsreplay`; it prints replay metadata and packet-type counts. Use `--events-json` to export decoded events as JSON, optionally filtered with `--event-type position` and `--entity-id ID`.
