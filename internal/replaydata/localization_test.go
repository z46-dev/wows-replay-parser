package replaydata

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"testing"
)

// TestParseMO extracts the English display name from a GNU gettext catalog.
func TestParseMO(t *testing.T) {
	var (
		messageID  string = "IDS_PJSB018"
		translated string = "Yamato"
		data       []byte = make([]byte, 44+len(messageID)+1+len(translated)+1)
	)

	binary.LittleEndian.PutUint32(data[0:4], 0x950412de)
	binary.LittleEndian.PutUint32(data[8:12], 1)
	binary.LittleEndian.PutUint32(data[12:16], 28)
	binary.LittleEndian.PutUint32(data[16:20], 36)
	binary.LittleEndian.PutUint32(data[28:32], uint32(len(messageID)))
	binary.LittleEndian.PutUint32(data[32:36], 44)
	binary.LittleEndian.PutUint32(data[36:40], uint32(len(translated)))
	binary.LittleEndian.PutUint32(data[40:44], uint32(45+len(messageID)))
	copy(data[44:], messageID)
	copy(data[45+len(messageID):], translated)

	var (
		translations map[string]string
		err          error
	)

	if translations, err = ParseMO(bytes.NewReader(data)); err != nil {
		t.Fatalf("ParseMO returned an error: %v", err)
	}

	if !reflect.DeepEqual(translations, map[string]string{"IDS_PJSB018": "Yamato"}) {
		t.Fatalf("ParseMO returned %#v", translations)
	}
}
