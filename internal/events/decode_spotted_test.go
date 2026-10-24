package events

import "testing"

// TestDecodeSpottedEntitiesReadsPackedIDs validates the count-prefixed 24-bit entity IDs.
func TestDecodeSpottedEntitiesReadsPackedIDs(t *testing.T) {
	var (
		event Event
		err   error
	)

	if event, err = decodeSpottedEntities(12.5, 900, []byte{2, 0x79, 0x1d, 0x08, 0, 0x81, 0x1d, 0x08, 0}); err != nil {
		t.Fatalf("decodeSpottedEntities() error = %v", err)
	}

	if event.Type != ShipVisibilityEvent || event.EntityID != 900 || len(event.Visibility.EntityIDs) != 2 {
		t.Fatalf("decodeSpottedEntities() = %#v, want two spotted IDs", event)
	}

	if event.Visibility.EntityIDs[0] != 531833 || event.Visibility.EntityIDs[1] != 531841 {
		t.Fatalf("decoded IDs = %v, want [531833 531841]", event.Visibility.EntityIDs)
	}
}

// TestDecodeSpottedEntitiesRejectsMalformedPayload prevents partial visibility updates.
func TestDecodeSpottedEntitiesRejectsMalformedPayload(t *testing.T) {
	if _, err := decodeSpottedEntities(0, 1, []byte{2, 1, 2, 3}); err == nil {
		t.Fatal("decodeSpottedEntities() error = nil, want malformed count error")
	}
}
