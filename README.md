# wows-replay-parser

A Go library for reading World of Warships replay files.

## Current support

The root package parses the replay container and returns typed match-setup metadata plus raw packet frames. For client builds 15.8.0.13187581 and 15.9.0.13357625, the `events` subpackage decodes replay setup, entity lifecycle, movement, player controls, camera state, map metadata, and post-battle results. Supplying an extracted matching replay-data package resolves entity type, property, and client-method names. The metadata header does not contain post-battle results in the inspected samples; the event stream supplies them instead.

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

The CLI is in `cmd/wows-replay-parser`. Run it with `go run ./cmd/wows-replay-parser path/to/replay.wowsreplay`; it prints replay metadata. Use `--events-json` to export decoded events, optionally filtered with `--event-type entity_create`, `--event-type entity_property`, `--event-type entity_method`, `--event-type ship_visibility`, `--event-type weapon_lock`, `--entity-id ID`, or `--my-ship` for player-ship movement updates. Add `--gamedata references/out/15.9.0.13357625` to resolve entity member names; the package version must match the replay exactly unless `--allow-version-mixing` is explicitly supplied. Mixed-version results may have incorrect member names and emit a warning. Raw payload bytes remain available on each property and method event.

Extract replay schemas and a compact ship-name catalog with `go run ./cmd/wows-replay-data-extract --install "C:\Program Files (x86)\Steam\steamapps\common\World of Warships" --version 15.9.0.13357625 --wowsunpack "C:\path\to\wowsunpack.exe"`. The extractor uses `wowsunpack` to decode the client's `GameParams.data` and combines its ship IDs with English display names from the build's gettext catalog. It stores only those names alongside `entities.xml`, entity definitions, interfaces, `alias.xml`, and the SHA-256 manifest in `references/out/<version>`. `wowsunpack` is available from [WoWs Toolkit releases](https://github.com/landaire/wows-toolkit/releases). Use `--output PATH` to choose another package location; an existing package is preserved unless `--force` is supplied. The version is explicit because the executable metadata does not identify the replay schema build reliably.

Filter with `--event-type battle_results` to export verified post-battle summaries, including match outcome and duration plus public player statistics such as remaining health, kills, damage, spotting damage, and killer IDs. The original result JSON remains available alongside the typed summary. Entity-create and player-bootstrap events retain their initial state bytes; JSON encodes those byte slices as base64, so event exports include the data required for future entity-state decoding and can be larger.

Export one headless PNG frame from the parser CLI with `go run ./cmd/wows-replay-parser path/to/replay.wowsreplay --export-frame references/out/frame.png --gamedata references/out/15.9.0.13357625 --spaces "C:\\Program Files (x86)\\Steam\\steamapps\\common\\World of Warships\\res_unpack\\spaces" --at 0`. The renderer uses the game's `minimap.png` and `minimap_water.png` plus ship silhouettes from the adjacent `gui/ships_silhouettes` directory; `--font PATH` optionally selects a local TTF/OTF font. Map markers use decoded team, current health, and alive state; never-spotted enemies are hidden, while previously spotted enemies use dim last-known positions. Capture-point rings use replay map-entity positions. Sidebar health, damage, kills, and survival currently come from final battle results; live event state is not yet correlated to each named player row.

Export a headless MP4 with `go run ./cmd/wows-replay-parser path/to/replay.wowsreplay --export-video references/out/replay.mp4 --gamedata references/out/15.9.0.13357625 --spaces "C:\\Program Files (x86)\\Steam\\steamapps\\common\\World of Warships\\res_unpack\\spaces"`. FFmpeg must be installed or selected with `--ffmpeg PATH`. Tune playback using `--video-speed 20`, `--video-fps 30`, quality using `--video-crf 20` (supported range 16-35; lower is higher quality), and output size using `--video-width 1328 --video-height 800` (even dimensions; width 960-3840 and height 540-2160). The map shows visibility-aware ships, capture points, aircraft, and brief team-colored torpedo sync markers. The CLI streams frames directly to FFmpeg, refuses to overwrite existing output, writes to a temporary file before publishing, and limits exports to 30,000 frames and 32 GiB of estimated raw RGBA frame data.
