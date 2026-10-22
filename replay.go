// Package replayparser reads World of Warships replay containers and packet frames.
package replayparser

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/z46-dev/wows-replay-parser/internal/replayformat"
	"golang.org/x/crypto/blowfish"
)

const (
	maxContainerBytes = 256 << 20
	maxMetadataBytes  = 16 << 20
	maxInflatedBytes  = 256 << 20
	maxPacketCount    = 4_000_000
)

var replayKey = []byte{0x29, 0xb7, 0xc9, 0x09, 0x38, 0x3f, 0x84, 0x88, 0xfa, 0x98, 0xec, 0x4e, 0x13, 0x19, 0x79, 0xfb}

type (
	// Metadata contains stable replay-header fields; Raw retains the complete header JSON.
	Metadata struct {
		ClientVersionFromExe string          `json:"clientVersionFromExe"`
		ClientVersionFromXML string          `json:"clientVersionFromXml"`
		DateTime             string          `json:"dateTime"`
		GameType             string          `json:"gameType"`
		MatchGroup           string          `json:"matchGroup"`
		Scenario             string          `json:"scenario"`
		MapDisplayName       string          `json:"mapDisplayName"`
		MapName              string          `json:"mapName"`
		MapID                int             `json:"mapId"`
		PlayerName           string          `json:"playerName"`
		PlayerVehicle        string          `json:"playerVehicle"`
		PlayersPerTeam       int             `json:"playersPerTeam"`
		TeamsCount           int             `json:"teamsCount"`
		GameMode             int             `json:"gameMode"`
		Duration             int             `json:"duration"`
		BattleDuration       int             `json:"battleDuration"`
		Raw                  json.RawMessage `json:"-"`
	}

	// Packet is one length-framed replay packet. Payload aliases Replay's packet buffer.
	Packet struct {
		Type    uint32
		Clock   float32
		Payload []byte
	}

	// Replay contains parsed metadata and raw packet frames for one supported client build.
	Replay struct {
		Metadata      Metadata
		ClientVersion string
		PacketLayout  string
		Packets       []Packet
	}
)

// Parse validates and decodes a complete replay container into raw packet frames.
func Parse(data []byte) (replay *Replay, err error) {
	if len(data) > maxContainerBytes {
		err = fmt.Errorf("replay is larger than %d bytes", maxContainerBytes)
		return
	}

	var (
		magic, blockCount, plainSize uint32
		metadataRaw                  []byte
	)

	var reader *bytes.Reader = bytes.NewReader(data)
	if err = binary.Read(reader, binary.LittleEndian, &magic); err != nil {
		return
	}

	if magic != replayformat.Magic {
		err = fmt.Errorf("invalid replay magic: 0x%08x", magic)
		return
	}

	if err = binary.Read(reader, binary.LittleEndian, &blockCount); err != nil {
		return
	}

	if blockCount == 0 || blockCount > 64 {
		err = fmt.Errorf("invalid replay block count: %d", blockCount)
		return
	}

	for index := uint32(0); index < blockCount; index++ {
		var blockSize uint32
		if err = binary.Read(reader, binary.LittleEndian, &blockSize); err != nil {
			return
		}

		if uint64(blockSize) > uint64(reader.Len()) {
			err = fmt.Errorf("metadata block %d exceeds remaining input", index)
			return
		}

		if index == 0 {
			if blockSize > maxMetadataBytes {
				err = fmt.Errorf("replay metadata exceeds %d bytes", maxMetadataBytes)
				return
			}

			metadataRaw = make([]byte, int(blockSize))
			if _, err = io.ReadFull(reader, metadataRaw); err != nil {
				return
			}
		} else if _, err = io.CopyN(io.Discard, reader, int64(blockSize)); err != nil {
			return
		}
	}

	if err = binary.Read(reader, binary.LittleEndian, &plainSize); err != nil {
		return
	}

	var compressedSize uint32
	if err = binary.Read(reader, binary.LittleEndian, &compressedSize); err != nil {
		return
	}

	if uint64(compressedSize) != uint64(reader.Len()) {
		err = fmt.Errorf("compressed size %d does not match remaining input %d", compressedSize, reader.Len())
		return
	}

	if plainSize > maxInflatedBytes {
		err = fmt.Errorf("declared packet stream exceeds %d bytes", maxInflatedBytes)
		return
	}

	var metadata Metadata
	if err = json.Unmarshal(metadataRaw, &metadata); err != nil {
		err = fmt.Errorf("invalid replay metadata: %w", err)
		return
	}

	metadata.Raw = append(json.RawMessage(nil), metadataRaw...)
	var (
		build replayformat.ClientBuild
		ok    bool
	)

	if build, ok = replayformat.LookupClientBuild(metadata.ClientVersionFromExe); !ok {
		err = fmt.Errorf("unsupported client build %q", metadata.ClientVersionFromExe)
		return
	}

	if blockCount != build.BlockCount {
		err = fmt.Errorf("client build %s expects %d metadata blocks, got %d", build.ClientVersion, build.BlockCount, blockCount)
		return
	}

	var packetData []byte
	if packetData, err = decryptAndInflate(reader, int64(plainSize)); err != nil {
		return
	}

	var packets []Packet
	if packets, err = parsePackets(packetData); err != nil {
		return
	}

	replay = &Replay{Metadata: metadata, ClientVersion: build.ClientVersion, PacketLayout: build.PacketLayout, Packets: packets}
	return
}

// decryptAndInflate applies the replay's Blowfish/XOR layer and checks the declared output size.
func decryptAndInflate(encrypted io.Reader, expectedSize int64) (plain []byte, err error) {
	var (
		cipher    *blowfish.Cipher
		cipherErr error
	)

	if cipher, cipherErr = blowfish.NewCipher(replayKey); cipherErr != nil {
		err = cipherErr
		return
	}

	var (
		ciphertext, previous, block [blowfish.BlockSize]byte
		decrypted                   bytes.Buffer
	)

	for {
		if _, err = io.ReadFull(encrypted, ciphertext[:]); err == io.EOF {
			err = nil
			break
		}

		if err != nil {
			err = fmt.Errorf("encrypted stream is not block aligned: %w", err)
			return
		}

		cipher.Decrypt(block[:], ciphertext[:])
		for index := range block {
			block[index] ^= previous[index]
		}

		if _, err = decrypted.Write(block[:]); err != nil {
			return
		}

		previous = block
	}

	var (
		zreader io.ReadCloser
		zerr    error
	)

	if zreader, zerr = zlib.NewReader(bytes.NewReader(decrypted.Bytes())); zerr != nil {
		err = fmt.Errorf("open compressed packet stream: %w", zerr)
		return
	}

	plain, err = io.ReadAll(io.LimitReader(zreader, maxInflatedBytes+1))
	var closeErr error
	closeErr = zreader.Close()
	if err != nil {
		return
	}

	if closeErr != nil {
		err = closeErr
		return
	}

	if int64(len(plain)) != expectedSize {
		err = fmt.Errorf("decompressed size %d does not match declared size %d", len(plain), expectedSize)
	}

	return
}

// parsePackets validates all frames and retains their payload slices without copying.
func parsePackets(data []byte) (packets []Packet, err error) {
	for offset := 0; offset < len(data); {
		if len(packets) == maxPacketCount {
			err = fmt.Errorf("packet stream exceeds %d packets", maxPacketCount)
			return
		}

		if len(data)-offset < 12 {
			err = fmt.Errorf("truncated packet header at offset %d", offset)
			return
		}

		var (
			payloadSize uint32 = binary.LittleEndian.Uint32(data[offset : offset+4])
			end         uint64 = uint64(offset) + 12 + uint64(payloadSize)
		)

		if end > uint64(len(data)) {
			err = fmt.Errorf("packet at offset %d exceeds packet stream", offset)
			return
		}

		packets = append(packets, Packet{
			Type:    binary.LittleEndian.Uint32(data[offset+4 : offset+8]),
			Clock:   math.Float32frombits(binary.LittleEndian.Uint32(data[offset+8 : offset+12])),
			Payload: data[offset+12 : int(end)],
		})

		offset = int(end)
	}

	return
}
