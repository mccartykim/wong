package peer

import (
	"encoding/binary"
	"net"
	"testing"
)

// TestNewPeer tests peer creation.
func TestNewPeer(t *testing.T) {
	ip := net.IPv4(127, 0, 0, 1)
	p := NewPeer(ip, 6881)

	if p == nil {
		t.Fatal("NewPeer returned nil")
	}

	if p.Port != 6881 {
		t.Errorf("expected port 6881, got %d", p.Port)
	}

	if p.AmChoking != true {
		t.Error("peer should start in choking state")
	}

	if p.PeerChoking != true {
		t.Error("peer should start with remote peer choking")
	}
}

// TestSetNumPieces tests setting the number of pieces.
func TestSetNumPieces(t *testing.T) {
	ip := net.IPv4(127, 0, 0, 1)
	p := NewPeer(ip, 6881)

	numPieces := 100
	p.SetNumPieces(numPieces)

	if p.NumPieces != numPieces {
		t.Errorf("expected %d pieces, got %d", numPieces, p.NumPieces)
	}

	// Calculate expected bitfield size
	expectedSize := (numPieces + 7) / 8
	if len(p.Bitfield) != expectedSize {
		t.Errorf("expected bitfield size %d, got %d", expectedSize, len(p.Bitfield))
	}
}

// TestHasPiece tests checking if a peer has a piece.
func TestHasPiece(t *testing.T) {
	ip := net.IPv4(127, 0, 0, 1)
	p := NewPeer(ip, 6881)
	p.SetNumPieces(100)

	// Manually set a bit
	pieceIndex := uint32(5)
	byteIdx := pieceIndex / 8
	bitIdx := 7 - (pieceIndex % 8)
	p.Bitfield[byteIdx] |= 1 << bitIdx

	if !p.HasPiece(pieceIndex) {
		t.Errorf("expected peer to have piece %d", pieceIndex)
	}

	if p.HasPiece(10) {
		t.Error("expected peer to not have piece 10")
	}
}

// TestHasPieceOutOfRange tests checking a piece index out of range.
func TestHasPieceOutOfRange(t *testing.T) {
	ip := net.IPv4(127, 0, 0, 1)
	p := NewPeer(ip, 6881)
	p.SetNumPieces(10)

	if p.HasPiece(100) {
		t.Error("expected HasPiece to return false for out-of-range index")
	}
}

// TestMessageEncoding tests message encoding.
func TestMessageEncoding(t *testing.T) {
	// Create a request message manually
	data := make([]byte, 12)
	binary.BigEndian.PutUint32(data[0:4], 5)   // index
	binary.BigEndian.PutUint32(data[4:8], 0)   // begin
	binary.BigEndian.PutUint32(data[8:12], 16384) // length

	if binary.BigEndian.Uint32(data[0:4]) != 5 {
		t.Error("index encoding failed")
	}

	if binary.BigEndian.Uint32(data[4:8]) != 0 {
		t.Error("begin encoding failed")
	}

	if binary.BigEndian.Uint32(data[8:12]) != 16384 {
		t.Error("length encoding failed")
	}
}
