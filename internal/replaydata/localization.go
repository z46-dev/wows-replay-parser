package replaydata

import (
	"encoding/binary"
	"fmt"
	"io"
)

// ParseMO reads GNU gettext translations from a compiled .mo catalog.
func ParseMO(reader io.Reader) (translations map[string]string, err error) {
	var data []byte
	if data, err = io.ReadAll(reader); err != nil {
		return
	}

	if len(data) < 28 {
		err = fmt.Errorf("gettext catalog header is truncated")
		return
	}

	var order binary.ByteOrder
	switch binary.LittleEndian.Uint32(data[:4]) {
	case 0x950412de:
		order = binary.LittleEndian
	default:
		if binary.BigEndian.Uint32(data[:4]) != 0x950412de {
			err = fmt.Errorf("invalid gettext catalog magic")
			return
		}

		order = binary.BigEndian
	}

	var (
		count      uint32 = order.Uint32(data[8:12])
		originals  uint32 = order.Uint32(data[12:16])
		translated uint32 = order.Uint32(data[16:20])
	)

	if uint64(originals)+uint64(count)*8 > uint64(len(data)) || uint64(translated)+uint64(count)*8 > uint64(len(data)) {
		err = fmt.Errorf("gettext catalog string table is out of bounds")
		return
	}

	translations = make(map[string]string, count)
	for index := range count {
		var (
			originalEntry    uint32 = originals + index*8
			translatedEntry  uint32 = translated + index*8
			originalLength   uint32 = order.Uint32(data[originalEntry : originalEntry+4])
			originalOffset   uint32 = order.Uint32(data[originalEntry+4 : originalEntry+8])
			translatedLength uint32 = order.Uint32(data[translatedEntry : translatedEntry+4])
			translatedOffset uint32 = order.Uint32(data[translatedEntry+4 : translatedEntry+8])
		)

		if uint64(originalOffset)+uint64(originalLength) > uint64(len(data)) || uint64(translatedOffset)+uint64(translatedLength) > uint64(len(data)) {
			err = fmt.Errorf("gettext catalog string is out of bounds")
			return
		}

		var original string = string(data[originalOffset : originalOffset+originalLength])
		var translation string = string(data[translatedOffset : translatedOffset+translatedLength])
		translations[original] = translation
	}

	return
}
