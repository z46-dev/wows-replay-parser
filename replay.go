// Package replayparser reads World of Warships replay containers and packets.
package replayparser

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"golang.org/x/crypto/blowfish"
)

const (
	replayMagic      uint32 = 0x11343212
	maxReplayBytes          = 256 << 20
	maxMetadataBytes        = 16 << 20
	maxPacketBytes          = 256 << 20
)

var replayKey = [16]byte{0x29, 0xb7, 0xc9, 0x09, 0x38, 0x3f, 0x84, 0x88, 0xfa, 0x98, 0xec, 0x4e, 0x13, 0x19, 0x79, 0xfb}

type (
	// Replay holds metadata and the decrypted packet stream.
	Replay struct {
		Metadata Metadata
		packets  []byte
	}

	// replayDecryptor implements Toolkit's Blowfish decrypt plus previous-plaintext XOR chain.
	replayDecryptor struct {
		input    io.Reader
		cipher   *blowfish.Cipher
		previous [blowfish.BlockSize]byte
		pending  [blowfish.BlockSize]byte
		offset   int
	}
)

// Open reads and parses one complete replay file.
func Open(path string) (replay *Replay, err error) {
	var info os.FileInfo
	if info, err = os.Stat(path); err != nil {
		return
	}

	if info.Size() > maxReplayBytes {
		err = fmt.Errorf("replay exceeds %d bytes", maxReplayBytes)
		return
	}

	var data []byte
	if data, err = os.ReadFile(path); err != nil {
		return
	}

	replay, err = Parse(data)
	return
}

// ReadMetadata reads only the plaintext replay header from a file.
func ReadMetadata(path string) (metadata Metadata, err error) {
	var file *os.File
	if file, err = os.Open(path); err != nil {
		return
	}

	defer file.Close()

	var (
		header      [12]byte
		count, size uint32
	)

	if _, err = io.ReadFull(file, header[:]); err != nil {
		return
	}

	if binary.LittleEndian.Uint32(header[:4]) != replayMagic {
		err = fmt.Errorf("invalid replay magic")
		return
	}

	if count = binary.LittleEndian.Uint32(header[4:8]); count == 0 || count > 64 {
		err = fmt.Errorf("invalid metadata block count %d", count)
		return
	}

	if size = binary.LittleEndian.Uint32(header[8:]); size > maxMetadataBytes {
		err = fmt.Errorf("replay metadata exceeds %d bytes", maxMetadataBytes)
		return
	}

	var raw []byte = make([]byte, size)
	if _, err = io.ReadFull(file, raw); err != nil {
		return
	}

	metadata, err = parseMetadata(raw)
	return
}

// Parse reads Toolkit's replay wrapper and inflates its packet stream.
func Parse(data []byte) (replay *Replay, err error) {
	if len(data) > maxReplayBytes {
		err = fmt.Errorf("replay exceeds %d bytes", maxReplayBytes)
		return
	}

	var (
		reader     *bytes.Reader = bytes.NewReader(data)
		magic      uint32
		blockCount uint32
	)

	if err = binary.Read(reader, binary.LittleEndian, &magic); err != nil {
		return
	}

	if magic != replayMagic {
		err = fmt.Errorf("invalid replay magic: 0x%08x", magic)
		return
	}

	if err = binary.Read(reader, binary.LittleEndian, &blockCount); err != nil {
		return
	}

	if blockCount == 0 || blockCount > 64 {
		err = fmt.Errorf("invalid metadata block count %d", blockCount)
		return
	}

	var metadata Metadata
	for index := range blockCount {
		var size uint32
		if err = binary.Read(reader, binary.LittleEndian, &size); err != nil {
			return
		}

		if uint64(size) > uint64(reader.Len()) {
			err = fmt.Errorf("metadata block %d exceeds replay", index)
			return
		}

		if index == 0 {
			if size > maxMetadataBytes {
				err = fmt.Errorf("replay metadata exceeds %d bytes", maxMetadataBytes)
				return
			}

			var raw []byte = make([]byte, size)
			if _, err = io.ReadFull(reader, raw); err != nil {
				return
			}

			if metadata, err = parseMetadata(raw); err != nil {
				return
			}
		} else if _, err = io.CopyN(io.Discard, reader, int64(size)); err != nil {
			return
		}
	}

	var (
		plainSize      uint32
		compressedSize uint32
	)

	if err = binary.Read(reader, binary.LittleEndian, &plainSize); err != nil {
		return
	}

	if err = binary.Read(reader, binary.LittleEndian, &compressedSize); err != nil {
		return
	}

	if plainSize > maxPacketBytes || uint64(compressedSize) != uint64(reader.Len()) {
		err = fmt.Errorf("invalid replay packet stream sizes: plain=%d compressed=%d remaining=%d", plainSize, compressedSize, reader.Len())
		return
	}

	var packetData []byte
	if packetData, err = inflatePackets(reader, int64(plainSize)); err != nil {
		return
	}

	replay = &Replay{Metadata: metadata, packets: packetData}
	return
}

// parseMetadata keeps the original JSON alongside the common typed fields.
func parseMetadata(raw []byte) (metadata Metadata, err error) {
	if err = json.Unmarshal(raw, &metadata); err != nil {
		err = fmt.Errorf("invalid replay metadata: %w", err)
		return
	}

	metadata.Raw = append(json.RawMessage(nil), raw...)
	return
}

// inflatePackets decrypts only as zlib asks for input and checks the declared size.
func inflatePackets(encrypted io.Reader, expected int64) (packets []byte, err error) {
	var cipher *blowfish.Cipher
	if cipher, err = blowfish.NewCipher(replayKey[:]); err != nil {
		return
	}

	var (
		input   *replayDecryptor = &replayDecryptor{input: encrypted, cipher: cipher, offset: blowfish.BlockSize}
		decoder io.ReadCloser
	)

	if decoder, err = zlib.NewReader(input); err != nil {
		err = fmt.Errorf("open compressed packets: %w", err)
		return
	}

	defer decoder.Close()

	if packets, err = io.ReadAll(io.LimitReader(decoder, int64(maxPacketBytes)+1)); err != nil {
		return
	}

	if int64(len(packets)) != expected {
		err = fmt.Errorf("packet stream size %d differs from declared %d", len(packets), expected)
	}

	return
}

// Read produces plaintext bytes from complete encrypted Blowfish blocks.
func (reader *replayDecryptor) Read(output []byte) (n int, err error) {
	for len(output) > 0 {
		if reader.offset == blowfish.BlockSize {
			var encrypted [blowfish.BlockSize]byte
			if _, err = io.ReadFull(reader.input, encrypted[:]); err != nil {
				if err == io.EOF && n > 0 {
					err = nil
				}

				return
			}

			reader.cipher.Decrypt(reader.pending[:], encrypted[:])
			for index := range reader.pending {
				reader.pending[index] ^= reader.previous[index]
			}

			reader.previous = reader.pending
			reader.offset = 0
		}

		var copied int = copy(output, reader.pending[reader.offset:])

		n += copied
		reader.offset += copied
		output = output[copied:]
	}

	return
}
