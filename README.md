# wows-replay-parser

A Go library for reading World of Warships replay files.

## Current support

The root package parses the replay container and returns typed match-setup metadata plus raw packet frames. For client builds 15.8.0.13187581 and 15.9.0.13357625, the `events` subpackage decodes replay setup, entity lifecycle, movement, player controls, camera state, map metadata, and post-battle results. Supplying a matching `wows-extractor` archive resolves entity type, property, and client-method names. The metadata header does not contain post-battle results in the inspected samples; the event stream supplies them instead.

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

The CLI is in `cmd/wows-replay-parser`. Run it with `go run ./cmd/wows-replay-parser path/to/replay.wowsreplay`; it prints replay metadata. Use `--events-json` to export decoded events, optionally filtered with `--event-type entity_create`, `--event-type entity_property`, `--event-type entity_method`, `--event-type ship_visibility`, `--event-type weapon_lock`, `--entity-id ID`, or `--my-ship` for player-ship movement updates. Add `--gamedata wows-BUILD.zip` to resolve entity member names; the archive's version must match the replay unless `--allow-version-mixing` is explicitly supplied. Mixed-version results may have incorrect member names and emit a warning. Raw payload bytes remain available on each property and method event.

Create one game archive with [wows-extractor](https://github.com/z46-dev/wows-extractor). Its `--include` flags replace the tool's defaults, so include schemas, GameParams, English text, minimap icons, and maps explicitly:

```powershell
wows-extractor extract -o wows-BUILD.zip "C:\Program Files (x86)\Steam\steamapps\common\World of Warships" --include scripts/ --include content/GameParams.data --include bin/res/texts/en/LC_MESSAGES/global.mo --include gui/fla/minimap/ship_icons/ --include gui/battle_hud/markers_minimap/plane/ --include gui/battle_hud/icon_frag/frags.png --include spaces/
```

The archive's build number must match the replay. `--client-version` can record the exact full version for stricter matching. `spaces/` includes other map files because the extractor selects by path prefix; for a single known map, include its `minimap.png`, `minimap_water.png`, and `space.settings` individually instead.

Filter with `--event-type battle_results` to export verified post-battle summaries, including match outcome and duration plus public player statistics such as remaining health, kills, damage, spotting damage, and killer IDs. The original result JSON remains available alongside the typed summary. Entity-create and player-bootstrap events retain their initial state bytes; JSON encodes those byte slices as base64, so event exports include the data required for future entity-state decoding and can be larger.

Export one headless PNG frame with `go run ./cmd/wows-replay-parser path/to/replay.wowsreplay --export-frame frame.png --gamedata wows-BUILD.zip --at 0`. The renderer reads the game's minimap PNGs, map bounds, and class and aircraft icons directly from the archive; `--font PATH` optionally selects a local TTF/OTF font. Map markers use decoded team, current health, and alive state; never-spotted enemies are hidden, while previously spotted enemies use dim last-known positions. Capture-point rings use replay map-entity positions.

Export a headless MP4 with `go run ./cmd/wows-replay-parser path/to/replay.wowsreplay --export-video replay.mp4 --gamedata wows-BUILD.zip`. FFmpeg must be installed or selected with `--ffmpeg PATH`. Tune playback using `--video-speed 20`, `--video-fps 30`, quality using `--video-crf 20` (supported range 16-35; lower is higher quality), and output size using `--video-width 1328 --video-height 800` (even dimensions; width 960-3840 and height 540-2160). The map shows visibility-aware ships, capture points, aircraft, and brief team-colored torpedo sync markers. The CLI streams frames directly to FFmpeg, refuses to overwrite existing output, writes to a temporary file before publishing, and limits exports to 30,000 frames and 32 GiB of estimated raw RGBA frame data.
