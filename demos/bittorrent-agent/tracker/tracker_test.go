package tracker

import (
	"testing"
)

// TestGeneratePeerID tests peer ID generation.
func TestGeneratePeerID(t *testing.T) {
	id := GeneratePeerID()

	// Should be 20 bytes
	if len(id) != 20 {
		t.Errorf("peer ID length should be 20, got %d", len(id))
	}

	// Should start with -GO0001-
	expected := "-GO0001-"
	if string(id[:8]) != expected {
		t.Errorf("peer ID should start with %s, got %s", expected, string(id[:8]))
	}
}

// TestParseCompactPeers tests parsing of compact peer format.
func TestParseCompactPeers(t *testing.T) {
	// Create mock peer data: 127.0.0.1:6881 (4 bytes IP + 2 bytes port)
	data := "\x7f\x00\x00\x01\x1a\xe1" // 127.0.0.1:6881
	peers, err := parseCompactPeers(data)

	if err != nil {
		t.Fatalf("parseCompactPeers failed: %v", err)
	}

	if len(peers) != 1 {
		t.Errorf("expected 1 peer, got %d", len(peers))
	}

	if peers[0].IP.String() != "127.0.0.1" {
		t.Errorf("expected IP 127.0.0.1, got %s", peers[0].IP.String())
	}

	if peers[0].Port != 6881 {
		t.Errorf("expected port 6881, got %d", peers[0].Port)
	}
}

// TestURLEncode tests URL encoding of binary data.
func TestURLEncode(t *testing.T) {
	data := []byte{0x01, 0x02, 0x03}
	encoded := urlEncode(data)
	expected := "%01%02%03"

	if encoded != expected {
		t.Errorf("expected %s, got %s", expected, encoded)
	}
}

// TestParseCompactPeersInvalid tests parsing of invalid compact peer data.
func TestParseCompactPeersInvalid(t *testing.T) {
	// Invalid length (not a multiple of 6)
	data := "\x7f\x00\x00"
	_, err := parseCompactPeers(data)

	if err == nil {
		t.Error("expected error for invalid length")
	}
}
