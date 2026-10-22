# wows-replay-parser

A Go library for reading World of Warships replay files.

## Current support

The root package parses the replay container and returns metadata and raw packet frames. It currently supports client builds 15.8.0.13187581 and 15.9.0.13357625. Gameplay-event decoding, statistics extraction, and video generation are not implemented yet.

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

The initial CLI is in `cmd/wows-replay-parser`. Run it with `go run ./cmd/wows-replay-parser path/to/replay.wowsreplay`; it prints the replay version, player, map, packet count, and packet-type counts.
