package pieces

import (
	"crypto/sha1"
	"sync"
	"testing"
)

// Test 1: NewManager creates correct number of pieces
func TestNewManager(t *testing.T) {
	hashes := [][20]byte{
		sha1.Sum([]byte("piece1")),
		sha1.Sum([]byte("piece2")),
		sha1.Sum([]byte("piece3")),
	}
	pieceLen := 256 * 1024 // 256KB
	totalLen := int64(3 * 256 * 1024)

	m := NewManager(pieceLen, totalLen, hashes)

	if m.NumPieces() != 3 {
		t.Fatalf("expected 3 pieces, got %d", m.NumPieces())
	}

	for i := 0; i < 3; i++ {
		m.mu.RLock()
		piece := m.pieces[i]
		m.mu.RUnlock()

		if piece.Index != i {
			t.Errorf("piece %d has wrong index: %d", i, piece.Index)
		}
		if piece.State != PieceNeeded {
			t.Errorf("piece %d should start in PieceNeeded state, got %v", i, piece.State)
		}
	}
}

// Test 2: PieceLength returns correct size, including shorter last piece
func TestPieceLength(t *testing.T) {
	hashes := [][20]byte{
		sha1.Sum([]byte("piece1")),
		sha1.Sum([]byte("piece2")),
		sha1.Sum([]byte("piece3")),
	}
	pieceLen := 256 * 1024 // 256KB
	totalLen := int64(2*256*1024 + 100*1024) // 612KB

	m := NewManager(pieceLen, totalLen, hashes)

	// First two pieces should be full size
	if m.PieceLength(0) != pieceLen {
		t.Errorf("piece 0 length: expected %d, got %d", pieceLen, m.PieceLength(0))
	}
	if m.PieceLength(1) != pieceLen {
		t.Errorf("piece 1 length: expected %d, got %d", pieceLen, m.PieceLength(1))
	}

	// Last piece should be shorter
	expectedLastLen := 100 * 1024
	if m.PieceLength(2) != expectedLastLen {
		t.Errorf("piece 2 length: expected %d, got %d", expectedLastLen, m.PieceLength(2))
	}
}

// Test 3: ReceiveBlock accumulates data correctly
func TestReceiveBlock(t *testing.T) {
	// Create data of 34 bytes
	data := []byte("hello world test data for a piece")
	hash := sha1.Sum(data)

	m := NewManager(34, 34, [][20]byte{hash})

	// Receive blocks out of order
	block1 := data[0:10]
	block2 := data[10:20]
	block3 := data[20:]

	err := m.ReceiveBlock(0, 20, block3)
	if err != nil {
		t.Fatalf("ReceiveBlock failed: %v", err)
	}

	err = m.ReceiveBlock(0, 0, block1)
	if err != nil {
		t.Fatalf("ReceiveBlock failed: %v", err)
	}

	err = m.ReceiveBlock(0, 10, block2)
	if err != nil {
		t.Fatalf("ReceiveBlock failed: %v", err)
	}

	m.mu.RLock()
	piece := m.pieces[0]
	m.mu.RUnlock()

	piece.mu.Lock()
	if len(piece.Data) != len(data) {
		t.Errorf("expected data length %d, got %d", len(data), len(piece.Data))
	}
	for i, b := range piece.Data {
		if b != data[i] {
			t.Errorf("data mismatch at position %d: expected %d, got %d", i, data[i], b)
		}
	}
	piece.mu.Unlock()
}

// Test 4: VerifyPiece succeeds with correct hash, fails with wrong data
func TestVerifyPiece(t *testing.T) {
	// Create data that's exactly 32 bytes
	data := make([]byte, 32)
	for i := 0; i < 32; i++ {
		data[i] = byte(i % 256)
	}
	correctHash := sha1.Sum(data)

	// Create wrong data
	wrongData := make([]byte, 32)
	for i := 0; i < 32; i++ {
		wrongData[i] = byte((i + 100) % 256)
	}
	wrongHash := sha1.Sum(wrongData)

	// Test with correct hash
	m := NewManager(32, 32, [][20]byte{correctHash})
	err := m.ReceiveBlock(0, 0, data)
	if err != nil {
		t.Fatalf("ReceiveBlock failed: %v", err)
	}

	// Piece should auto-verify
	m.mu.RLock()
	piece := m.pieces[0]
	m.mu.RUnlock()

	piece.mu.Lock()
	if piece.State != PieceVerified {
		t.Errorf("piece should be verified after receiving all blocks, state: %v", piece.State)
	}
	piece.mu.Unlock()

	// Test with wrong hash
	m2 := NewManager(32, 32, [][20]byte{wrongHash})
	err = m2.ReceiveBlock(0, 0, data) // Send the original data but with wrong hash expectation
	if err != nil {
		t.Fatalf("ReceiveBlock failed: %v", err)
	}

	m2.mu.RLock()
	piece2 := m2.pieces[0]
	m2.mu.RUnlock()

	piece2.mu.Lock()
	if piece2.State != PieceNeeded {
		t.Errorf("piece should be back in PieceNeeded after verification failure, state: %v", piece2.State)
	}
	piece2.mu.Unlock()
}

// Test 5: PickPiece returns a needed piece the peer has
func TestPickPiece(t *testing.T) {
	data1 := []byte("piece one data")
	data2 := []byte("piece two data")
	data3 := []byte("piece three data")

	hashes := [][20]byte{
		sha1.Sum(data1),
		sha1.Sum(data2),
		sha1.Sum(data3),
	}

	m := NewManager(100, 300, hashes)

	// Peer bitfield: has pieces 0 and 2, not 1
	// Bitfield format: bits are MSB first
	// Piece 0 = bit 0 of byte 0 (MSB)
	// Piece 1 = bit 1 of byte 0
	// Piece 2 = bit 2 of byte 0
	// Piece 0 and 2: 10100000 = 0xA0
	peerBitfield := []byte{0xA0}

	// Pick should return piece 0 first
	idx, found := m.PickPiece(peerBitfield)
	if !found {
		t.Fatal("expected to find a piece, but didn't")
	}
	if idx != 0 {
		t.Errorf("expected piece 0, got piece %d", idx)
	}

	// Mark piece 0 as verified
	m.mu.Lock()
	m.pieces[0].mu.Lock()
	m.pieces[0].State = PieceVerified
	m.pieces[0].mu.Unlock()
	m.mu.Unlock()

	// Pick again, should return piece 2 now
	idx, found = m.PickPiece(peerBitfield)
	if !found {
		t.Fatal("expected to find a piece, but didn't")
	}
	if idx != 2 {
		t.Errorf("expected piece 2, got piece %d", idx)
	}
}

// Test 6: IsComplete returns true when all verified
func TestIsComplete(t *testing.T) {
	data1 := []byte("0123456789abcdef") // 16 bytes
	data2 := []byte("ghijklmnopqrstuv") // 16 bytes

	hashes := [][20]byte{
		sha1.Sum(data1),
		sha1.Sum(data2),
	}

	m := NewManager(16, 32, hashes)

	// Initially not complete
	if m.IsComplete() {
		t.Fatal("expected not complete initially")
	}

	// Receive first piece
	err := m.ReceiveBlock(0, 0, data1)
	if err != nil {
		t.Fatalf("ReceiveBlock failed: %v", err)
	}

	if m.IsComplete() {
		t.Fatal("expected not complete with only 1 piece")
	}

	// Receive second piece
	err = m.ReceiveBlock(1, 0, data2)
	if err != nil {
		t.Fatalf("ReceiveBlock failed: %v", err)
	}

	if !m.IsComplete() {
		t.Fatal("expected complete after all pieces received")
	}
}

// Test 7: Bitfield encoding is correct
func TestBitfield(t *testing.T) {
	hashes := [][20]byte{
		sha1.Sum([]byte("p0")),
		sha1.Sum([]byte("p1")),
		sha1.Sum([]byte("p2")),
		sha1.Sum([]byte("p3")),
		sha1.Sum([]byte("p4")),
		sha1.Sum([]byte("p5")),
		sha1.Sum([]byte("p6")),
		sha1.Sum([]byte("p7")),
		sha1.Sum([]byte("p8")),
	}

	m := NewManager(100, 900, hashes)

	// Mark some pieces as verified
	m.mu.Lock()
	for _, idx := range []int{0, 2, 5, 8} {
		m.pieces[idx].mu.Lock()
		m.pieces[idx].State = PieceVerified
		m.pieces[idx].mu.Unlock()
	}
	m.mu.Unlock()

	bitfield := m.Bitfield()

	// Expected: pieces 0,2,5,8
	// Piece 0 = bit 0 (MSB of byte 0) = 10000000 = 0x80
	// Piece 2 = bit 2 (MSB-2 of byte 0) = 00100000 = 0x20
	// Piece 5 = bit 5 (MSB-5 of byte 0) = 00000100 = 0x04
	// Piece 8 = bit 0 of byte 1 (MSB of byte 1) = 10000000 = 0x80
	// Byte 0: 10100100 = 0xA4
	// Byte 1: 10000000 = 0x80

	if len(bitfield) != 2 {
		t.Errorf("expected bitfield length 2, got %d", len(bitfield))
	}

	if bitfield[0] != 0xA4 {
		t.Errorf("byte 0: expected 0xA4, got 0x%02X", bitfield[0])
	}

	if bitfield[1] != 0x80 {
		t.Errorf("byte 1: expected 0x80, got 0x%02X", bitfield[1])
	}
}

// Test 8: Concurrent ReceiveBlock calls are safe
func TestConcurrentReceiveBlock(t *testing.T) {
	numPieces := 10
	hashes := make([][20]byte, numPieces)
	pieceData := make([][]byte, numPieces)
	pieceLen := 16

	for i := 0; i < numPieces; i++ {
		// Create data of exact pieceLen bytes
		pieceData[i] = make([]byte, pieceLen)
		for j := 0; j < pieceLen; j++ {
			pieceData[i][j] = byte((i*10 + j) % 256)
		}
		hashes[i] = sha1.Sum(pieceData[i])
	}

	m := NewManager(pieceLen, int64(numPieces*pieceLen), hashes)

	var wg sync.WaitGroup
	errors := make([]error, 0)
	var errMu sync.Mutex

	// Launch goroutines to concurrently receive blocks
	for pieceIdx := 0; pieceIdx < numPieces; pieceIdx++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := pieceData[idx]
			err := m.ReceiveBlock(idx, 0, data)
			if err != nil {
				errMu.Lock()
				errors = append(errors, err)
				errMu.Unlock()
			}
		}(pieceIdx)
	}

	wg.Wait()

	if len(errors) > 0 {
		t.Fatalf("got errors during concurrent receive: %v", errors)
	}

	// Verify all pieces are verified
	if m.Downloaded() != numPieces {
		t.Errorf("expected %d verified pieces, got %d", numPieces, m.Downloaded())
	}

	if !m.IsComplete() {
		t.Fatal("expected all pieces to be verified")
	}
}

// Additional test: MarkRequested changes state correctly
func TestMarkRequested(t *testing.T) {
	hashes := [][20]byte{sha1.Sum([]byte("piece"))}
	m := NewManager(100, 100, hashes)

	m.MarkRequested(0)

	m.mu.RLock()
	piece := m.pieces[0]
	m.mu.RUnlock()

	piece.mu.Lock()
	if piece.State != PieceRequested {
		t.Errorf("expected state PieceRequested, got %v", piece.State)
	}
	piece.mu.Unlock()
}

// Additional test: Downloaded returns correct count
func TestDownloaded(t *testing.T) {
	data1 := []byte("piece1data123456") // 16 bytes
	data2 := []byte("piece2data123456") // 16 bytes
	data3 := []byte("piece3data123456") // 16 bytes

	hashes := [][20]byte{
		sha1.Sum(data1),
		sha1.Sum(data2),
		sha1.Sum(data3),
	}

	m := NewManager(16, 48, hashes)

	if m.Downloaded() != 0 {
		t.Errorf("expected 0 downloaded, got %d", m.Downloaded())
	}

	m.ReceiveBlock(0, 0, data1)
	if m.Downloaded() != 1 {
		t.Errorf("expected 1 downloaded, got %d", m.Downloaded())
	}

	m.ReceiveBlock(2, 0, data3)
	if m.Downloaded() != 2 {
		t.Errorf("expected 2 downloaded, got %d", m.Downloaded())
	}

	m.ReceiveBlock(1, 0, data2)
	if m.Downloaded() != 3 {
		t.Errorf("expected 3 downloaded, got %d", m.Downloaded())
	}
}
