# wows-replay-parser

A Go library and CLI for World of Warships replays. The new parser follows the local WoWS Toolkit `wows_replays` crate for replay and packet behavior, and uses `wows-extractor` archives for build data.

## Current status

The new implementation reads metadata, decrypts complete replay containers, walks version-aware raw packets, and borrows an already-opened extractor archive through `NewGameData`. Typed packet decoding, battle analysis, PNG rendering, and MP4 export are still being ported. The previous implementation is preserved locally in `references/archive/` and in Git history.

## Library

```go
archive, err := wowsextractor.Load("wows-BUILD.zip")
if err != nil {
    return err
}
defer archive.Close()

data, err := replayparser.NewGameData(archive)
if err != nil {
    return err
}

replay, err := replayparser.Open("battle.wowsreplay")
if err != nil {
    return err
}
if err = data.ValidateVersion(replay); err != nil {
    return err
}

err = replay.WalkPackets(ctx, func(packet replayparser.Packet) error {
    fmt.Println(packet.Clock, packet.Type)
    return nil
})
```

The caller owns the archive and must keep it open while using `GameData`. GameParams regions are decoded once per `GameData` instance. `ReadMetadata(path)` avoids decrypting the packet stream when only the replay header is needed.

## CLI

```sh
go run ./cmd/wows-replay-parser inspect battle.wowsreplay
go run ./cmd/wows-replay-parser inspect battle.wowsreplay --json
go run ./cmd/wows-replay-parser packets battle.wowsreplay --gamedata wows-BUILD.zip
go run ./cmd/wows-replay-parser packets battle.wowsreplay --json
```

The CLI uses the same library operations as Go callers. `--gamedata` verifies that the extractor archive matches the replay build. Its output commands use `go-arg`; terminal progress uses `golog`.

Run `go test ./...`, `go vet ./...`, and `go build ./...` locally or through the CI workflow.
