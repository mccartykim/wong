package piecemanager

import (
	"testing"

	"github.com/example/bittorrent/metainfo"
)

// TestPieceManagerCreation tests creating a piece manager.
func TestPieceManagerCreation(t *testing.T) {
	meta := &metainfo.TorrentMeta{
		PieceLength: 16384,
		TotalLength: 32768,
		Files: []metainfo.FileInfo{
			{Length: 32768, Path: "test.txt"},
		},
		Pieces: make([][20]byte, 2),
	}

	tempDir := t.TempDir()

	pm, err := NewPieceManager(meta, tempDir)
	if err != nil {
		t.Fatalf("failed to create piece manager: %v", err)
	}

	if pm == nil {
		t.Fatal("piece manager is nil")
	}

	numPieces := pm.GetNumPieces()
	if numPieces != 2 {
		t.Errorf("expected 2 pieces, got %d", numPieces)
	}

	pm.Close()
}

// TestGetPendingBlock tests getting a pending block.
func TestGetPendingBlock(t *testing.T) {
	meta := &metainfo.TorrentMeta{
		PieceLength: 16384,
		TotalLength: 32768,
		Files: []metainfo.FileInfo{
			{Length: 32768, Path: "test.txt"},
		},
		Pieces: make([][20]byte, 2),
	}

	tempDir := t.TempDir()

	pm, err := NewPieceManager(meta, tempDir)
	if err != nil {
		t.Fatalf("failed to create piece manager: %v", err)
	}
	defer pm.Close()

	block := pm.GetPendingBlock()
	if block == nil {
		t.Fatal("expected a pending block")
	}

	if block.Index != 0 {
		t.Errorf("expected piece index 0, got %d", block.Index)
	}

	if block.Begin != 0 {
		t.Errorf("expected begin 0, got %d", block.Begin)
	}
}

// TestReceiveBlock tests receiving a block.
func TestReceiveBlock(t *testing.T) {
	meta := &metainfo.TorrentMeta{
		PieceLength: 16384,
		TotalLength: 32768,
		Files: []metainfo.FileInfo{
			{Length: 32768, Path: "test.txt"},
		},
		Pieces: make([][20]byte, 2),
	}

	tempDir := t.TempDir()

	pm, err := NewPieceManager(meta, tempDir)
	if err != nil {
		t.Fatalf("failed to create piece manager: %v", err)
	}
	defer pm.Close()

	// Create test block data
	blockData := make([]byte, BlockSize)
	for i := range blockData {
		blockData[i] = byte(i % 256)
	}

	// Receive a block
	err = pm.ReceiveBlock(0, 0, blockData)
	if err != nil {
		t.Fatalf("failed to receive block: %v", err)
	}

	// Check that piece state changed
	state := pm.GetPieceState(0)
	if state != StateComplete {
		t.Errorf("expected state InProgress, got %d", state)
	}
}

// TestInvalidPieceIndex tests receiving a block with invalid piece index.
func TestInvalidPieceIndex(t *testing.T) {
	meta := &metainfo.TorrentMeta{
		PieceLength: 16384,
		TotalLength: 32768,
		Files: []metainfo.FileInfo{
			{Length: 32768, Path: "test.txt"},
		},
		Pieces: make([][20]byte, 2),
	}

	tempDir := t.TempDir()

	pm, err := NewPieceManager(meta, tempDir)
	if err != nil {
		t.Fatalf("failed to create piece manager: %v", err)
	}
	defer pm.Close()

	blockData := make([]byte, BlockSize)

	// Try to receive a block for an invalid piece
	err = pm.ReceiveBlock(100, 0, blockData)
	if err == nil {
		t.Error("expected error for invalid piece index")
	}
}

// TestProgress tests getting download progress.
func TestProgress(t *testing.T) {
	meta := &metainfo.TorrentMeta{
		PieceLength: 16384,
		TotalLength: 32768,
		Files: []metainfo.FileInfo{
			{Length: 32768, Path: "test.txt"},
		},
		Pieces: make([][20]byte, 2),
	}

	tempDir := t.TempDir()

	pm, err := NewPieceManager(meta, tempDir)
	if err != nil {
		t.Fatalf("failed to create piece manager: %v", err)
	}
	defer pm.Close()

	downloaded, total := pm.GetProgress()
	if downloaded != 0 {
		t.Errorf("expected 0 downloaded, got %d", downloaded)
	}

	if total != meta.TotalLength {
		t.Errorf("expected total %d, got %d", meta.TotalLength, total)
	}
}

// TestGetPieceState tests getting piece state.
func TestGetPieceState(t *testing.T) {
	meta := &metainfo.TorrentMeta{
		PieceLength: 16384,
		TotalLength: 32768,
		Files: []metainfo.FileInfo{
			{Length: 32768, Path: "test.txt"},
		},
		Pieces: make([][20]byte, 2),
	}

	tempDir := t.TempDir()

	pm, err := NewPieceManager(meta, tempDir)
	if err != nil {
		t.Fatalf("failed to create piece manager: %v", err)
	}
	defer pm.Close()

	state := pm.GetPieceState(0)
	if state != StatePending {
		t.Errorf("expected state Pending, got %d", state)
	}

	// Check invalid piece index
	state = pm.GetPieceState(100)
	if state != StatePending {
		t.Errorf("expected state Pending for invalid index, got %d", state)
	}
}
