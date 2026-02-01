package pieces

import (
	"crypto/sha1"
	"sync"
)

// PieceState represents the download state of a piece
type PieceState int

const (
	PieceNeeded PieceState = iota
	PieceRequested
	PieceDownloaded
	PieceVerified
)

// Piece represents a single piece in a torrent
type Piece struct {
	Index  int
	Hash   [20]byte
	Length int
	State  PieceState
	Data   []byte
	mu     sync.Mutex
}

// Manager manages all pieces in a torrent
type Manager struct {
	pieces   []*Piece
	pieceLen int
	totalLen int64
	mu       sync.RWMutex
}

// NewManager creates a new piece manager
// hashes should be a slice of 20-byte SHA1 hashes, one per piece
func NewManager(pieceLength int, totalLength int64, hashes [][20]byte) *Manager {
	m := &Manager{
		pieces:   make([]*Piece, len(hashes)),
		pieceLen: pieceLength,
		totalLen: totalLength,
	}

	for i, hash := range hashes {
		m.pieces[i] = &Piece{
			Index:  i,
			Hash:   hash,
			Length: pieceLength,
			State:  PieceNeeded,
			Data:   make([]byte, 0, pieceLength),
		}
	}

	// Set the last piece to potentially be shorter
	if len(m.pieces) > 0 {
		lastPieceLen := int(totalLength % int64(pieceLength))
		if lastPieceLen == 0 {
			lastPieceLen = pieceLength
		}
		m.pieces[len(m.pieces)-1].Length = lastPieceLen
	}

	return m
}

// PickPiece selects a piece to download based on what the peer has
// Uses simplified rarest-first: just picks the first needed piece the peer has
// Returns the piece index and true if a piece was found, false otherwise
func (m *Manager) PickPiece(peerBitfield []byte) (int, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for i, piece := range m.pieces {
		// Check if piece is already verified
		if piece.State == PieceVerified {
			continue
		}

		// Check if peer has this piece
		byteIndex := i / 8
		bitIndex := 7 - (i % 8) // MSB first
		if byteIndex >= len(peerBitfield) {
			continue
		}

		if (peerBitfield[byteIndex] & (1 << uint(bitIndex))) != 0 {
			// Peer has this piece and we need it
			return i, true
		}
	}

	return 0, false
}

// MarkRequested marks a piece as requested
func (m *Manager) MarkRequested(index int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if index >= 0 && index < len(m.pieces) {
		m.pieces[index].mu.Lock()
		if m.pieces[index].State == PieceNeeded {
			m.pieces[index].State = PieceRequested
		}
		m.pieces[index].mu.Unlock()
	}
}

// ReceiveBlock accumulates a block of data for a piece
// When all blocks are received, automatically verifies the piece
func (m *Manager) ReceiveBlock(index int, begin int, data []byte) error {
	m.mu.RLock()
	if index < 0 || index >= len(m.pieces) {
		m.mu.RUnlock()
		return ErrInvalidPieceIndex
	}
	piece := m.pieces[index]
	m.mu.RUnlock()

	piece.mu.Lock()
	defer piece.mu.Unlock()

	// Ensure Data buffer is large enough
	if begin+len(data) > piece.Length {
		return ErrBlockOutOfRange
	}

	// Expand Data if necessary
	if len(piece.Data) < begin+len(data) {
		newData := make([]byte, begin+len(data))
		copy(newData, piece.Data)
		piece.Data = newData
	}

	// Copy data into the piece
	copy(piece.Data[begin:], data)

	// Check if piece is complete
	if len(piece.Data) == piece.Length {
		piece.State = PieceDownloaded

		// Auto-verify
		if m.VerifyPieceInternal(piece) {
			piece.State = PieceVerified
		} else {
			// Verification failed, reset for retry
			piece.Data = make([]byte, 0, piece.Length)
			piece.State = PieceNeeded
		}
	}

	return nil
}

// VerifyPiece verifies a piece against its hash
func (m *Manager) VerifyPiece(index int) bool {
	m.mu.RLock()
	if index < 0 || index >= len(m.pieces) {
		m.mu.RUnlock()
		return false
	}
	piece := m.pieces[index]
	m.mu.RUnlock()

	piece.mu.Lock()
	defer piece.mu.Unlock()

	return m.VerifyPieceInternal(piece)
}

// VerifyPieceInternal verifies a piece (must be called with piece.mu held)
func (m *Manager) VerifyPieceInternal(piece *Piece) bool {
	if len(piece.Data) != piece.Length {
		return false
	}

	hash := sha1.Sum(piece.Data)
	return hash == piece.Hash
}

// PieceLength returns the length of a specific piece
func (m *Manager) PieceLength(index int) int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if index < 0 || index >= len(m.pieces) {
		return 0
	}
	return m.pieces[index].Length
}

// IsComplete returns true if all pieces are verified
func (m *Manager) IsComplete() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.pieces) == 0 {
		return false
	}

	for _, piece := range m.pieces {
		piece.mu.Lock()
		if piece.State != PieceVerified {
			piece.mu.Unlock()
			return false
		}
		piece.mu.Unlock()
	}

	return true
}

// Downloaded returns the count of verified pieces
func (m *Manager) Downloaded() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	count := 0
	for _, piece := range m.pieces {
		piece.mu.Lock()
		if piece.State == PieceVerified {
			count++
		}
		piece.mu.Unlock()
	}

	return count
}

// NumPieces returns the total number of pieces
func (m *Manager) NumPieces() int {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return len(m.pieces)
}

// Bitfield returns our bitfield for peers (verified pieces only)
// Bit i represents piece i, MSB first in each byte
func (m *Manager) Bitfield() []byte {
	m.mu.RLock()
	defer m.mu.RUnlock()

	numBytes := (len(m.pieces) + 7) / 8
	bitfield := make([]byte, numBytes)

	for i, piece := range m.pieces {
		piece.mu.Lock()
		if piece.State == PieceVerified {
			byteIndex := i / 8
			bitIndex := 7 - (i % 8) // MSB first
			bitfield[byteIndex] |= (1 << uint(bitIndex))
		}
		piece.mu.Unlock()
	}

	return bitfield
}
