![Tests](https://img.shields.io/github/actions/workflow/status/z46-dev/wows-replay-parser/ci.yml?branch=main&event=push&label=CI)
![Made with Golang](https://img.shields.io/badge/-Made_with_Golang-007d9c?logo=go&logoColor=white)

# wows-replay-parser

A Go library and CLI for World of Warships replays. The new parser follows the local WoWS Toolkit `wows_replays` crate for replay and packet behavior, and uses `wows-extractor` archives for build data.

## Current status

The implementation reads metadata, decrypts replay containers, walks version-aware raw packets, and decodes typed events using Toolkit-compatible entity definitions from a borrowed `wows-extractor` archive. Battle analysis, PNG rendering, and MP4 export are still being ported. The previous implementation is preserved locally in `references/archive/` and in Git history.

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

err = replay.WalkEvents(ctx, data, func(event replayparser.Event) error {
    fmt.Println(event.Clock, event.Type, event.EntityType, event.Name)
    return nil
})
```

The caller owns the archive and must keep it open while using `GameData`. Entity specs and GameParams regions are decoded once per `GameData` instance. `WalkEvents` retains entity state during one traversal. An undecodable packet yields an `invalid` event with its raw payload and a diagnostic; a method-size mapping mismatch stops traversal. `ReadMetadata(path)` avoids decrypting the packet stream when only the replay header is needed.

## CLI

```sh
go run ./cmd/wows-replay-parser inspect battle.wowsreplay
go run ./cmd/wows-replay-parser inspect battle.wowsreplay --json
go run ./cmd/wows-replay-parser packets battle.wowsreplay --gamedata wows-BUILD.zip
go run ./cmd/wows-replay-parser packets battle.wowsreplay --json
go run ./cmd/wows-replay-parser events battle.wowsreplay --gamedata wows-BUILD.zip
go run ./cmd/wows-replay-parser events battle.wowsreplay -g wows-BUILD.zip --type entity_method
```

The CLI uses the same library operations as Go callers. `events` emits one JSON event per line and supports `--type` and `--entity-id` filters. `--gamedata` verifies that the extractor archive matches the replay build. Its output commands use `go-arg`; terminal progress uses `golog`.

Run `go test ./...`, `go vet ./...`, and `go build ./...` locally or through the CI workflow.
