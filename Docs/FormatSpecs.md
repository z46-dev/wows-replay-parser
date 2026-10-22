# `.wowsreplay` format notes

These notes describe the files inspected so far; they are observations, not a complete official specification.

## Container and packet stream

The sampled files use little-endian integers and have this layout:

| Field | Size | Meaning |
|---|---:|---|
| Magic | 4 bytes | `0x11343212` |
| Block count | 4 bytes | Number of length-prefixed blocks; `3` in the samples |
| Blocks | Repeated | Each is a 4-byte length followed by that many bytes |
| Decompressed size | 4 bytes | Expected size of the plaintext packet stream |
| Compressed size | 4 bytes | Size of the encrypted stream that follows |
| Packet stream | As declared | Blowfish-encrypted, zlib-compressed bytes |

The first block is a JSON match header. In the inspected files, the other two blocks are an empty block and a timestamp-like ASCII string. The value `3` is the block count, not evidence of a replay protocol version.

To read the packet stream, decrypt each 8-byte block with Blowfish ECB using key `29 b7 c9 09 38 3f 84 88 fa 98 ec 4e 13 19 79 fb`. XOR each decrypted block with the previous plaintext block, starting with eight zero bytes, then zlib-decompress the result. The plaintext stream is a sequence of packets with a 12-byte header:

| Field | Size | Encoding |
|---|---:|---|
| Payload length | 4 bytes | `u32` little-endian |
| Packet type | 4 bytes | `u32` little-endian |
| Clock | 4 bytes | `f32` little-endian |
| Payload | Declared length | Opaque until its packet type is decoded |

## 15.8.0 and 15.9.0 comparison

The comparison used five 15.8.0 replays (catalog IDs `237008`, `236904`, `236902`, `236887`, `236883`) and seven local 15.9.0 replays. The full client builds are `15.8.0.13187581` and `15.9.0.13357625`.

| Area | Observed result |
|---|---|
| Container | Both use magic `0x11343212`, block count `3`, and the same block/size layout. |
| JSON header | All 12 files have the same 28 top-level keys; values vary by battle. |
| Encryption and compression | The same decrypt-and-inflate procedure succeeds for both sets. Declared sizes match the decoded data. |
| Packet framing | The same 12-byte packet-header walk consumes every decoded stream exactly, with no trailing bytes or invalid lengths. |
| Version packet | Packet type `22` has a 12-byte payload in both sets. Its payload contains `15,8,0,0` or `15,9,0,0`, respectively. |
| Other packet IDs | The 15.9.0 sample set adds no packet IDs absent from the 15.8.0 set. IDs `46` and `47` occur only in 15.8.0 samples (`46` in one file, `47` in four); their meanings are not identified. |

This supports sharing the container reader, decryptor, decompressor, and packet-frame walker for these two builds. It does **not** establish that every packet payload has the same schema or meaning. The packet-ID difference may be version-related or reflect which battles were sampled; more matched replays and decoded packet definitions are needed to decide.

## Parser support

The root Go package parses the container, decrypts and inflates the packet stream, and returns typed match-setup metadata plus raw packet frames (`type`, `clock`, and payload). The `events` subpackage currently decodes packet type `10` as entity-position updates and validates the length-prefixed JSON in the type `34` battle-results message. Across 14 inspected replays, the JSON header has the same 28 top-level keys and contains no post-battle result or statistics fields; those must be recovered from packet payloads. The initial CLI at `cmd/wows-replay-parser` prints metadata and counts raw packet IDs, and can export the decoded event timeline as JSON; it does not render video. Build descriptors live in `internal/replayformat/v15_8.go` and `v15_9.go`; shared container and frame logic stays in the root package. Add a descriptor only after inspecting replays for that exact client build. Put actual wire-layout changes in shared decoding code selected by an explicit protocol layout/version rule, rather than copying the parser into each build file.

The package currently accepts the exact observed builds `15.8.0.13187581` and `15.9.0.13357625`, each with three metadata blocks and the modern packet-ID layout. The modern layout assignment follows WoWs Toolkit's documented 12.6.0 transition; the 15.8/15.9 sample comparison itself verified framing, not payload schemas. Container and inflated packet data are capped at 256 MiB, and frame count is capped at four million. One replay from each build is checked in under `testdata/replays/` and exercised by `go test ./...`.

## How this comparison was made

1. Read each file's magic, block count, and block lengths; parse the first block as JSON and record the client build.
2. Read the declared decompressed/compressed sizes, decrypt and decompress the remaining bytes, and confirm both sizes.
3. Walk packet headers using payload length, type, and clock; require each packet to fit and the walk to end exactly at EOF.
4. Compare JSON key sets, packet framing, and packet-type sets across the 15.8.0 and 15.9.0 samples.

The inspection used Python's `struct` and `zlib` modules plus PyCryptodome's Blowfish implementation. No comparison script is checked in yet. Samples are local under the gitignored `references/replays/` directory.

## References

- [WoWs Toolkit replay container and decrypt/inflate implementation](https://github.com/landaire/wows-toolkit/blob/main/crates/wows-replays/src/wowsreplay.rs)
- [WoWs Toolkit BigWorld packet notes](https://github.com/landaire/wows-toolkit/blob/main/docs/BIGWORLD_PROTOCOL.md)
