package metainfo

import (
	"crypto/sha1"
	"testing"

	"github.com/example/bittorrent/bencode"
)

// TestSingleFileTorrent tests parsing of a single-file torrent.
func TestSingleFileTorrent(t *testing.T) {
	// Create a test single-file torrent
	torrent := buildTestTorrent(
		"http://tracker.example.com:8080/announce",
		&infoDict{
			PieceLength: 16384,
			Pieces:      makePieces(2),
			Name:        "test.txt",
			Length:      40000,
		},
	)

	meta, err := ParseTorrent(torrent)
	if err != nil {
		t.Fatalf("ParseTorrent failed: %v", err)
	}

	// Verify announce
	if meta.Announce != "http://tracker.example.com:8080/announce" {
		t.Errorf("Expected announce %s, got %s", "http://tracker.example.com:8080/announce", meta.Announce)
	}

	// Verify piece length
	if meta.PieceLength != 16384 {
		t.Errorf("Expected piece length 16384, got %d", meta.PieceLength)
	}

	// Verify number of pieces
	if len(meta.Pieces) != 2 {
		t.Errorf("Expected 2 pieces, got %d", len(meta.Pieces))
	}

	// Verify name
	if meta.Name != "test.txt" {
		t.Errorf("Expected name test.txt, got %s", meta.Name)
	}

	// Verify total length
	if meta.TotalLength != 40000 {
		t.Errorf("Expected total length 40000, got %d", meta.TotalLength)
	}

	// Verify files
	if len(meta.Files) != 1 {
		t.Errorf("Expected 1 file, got %d", len(meta.Files))
	}
	if meta.Files[0].Path != "test.txt" {
		t.Errorf("Expected file path test.txt, got %s", meta.Files[0].Path)
	}
	if meta.Files[0].Length != 40000 {
		t.Errorf("Expected file length 40000, got %d", meta.Files[0].Length)
	}

	// Verify info_hash is not zero
	zeroHash := [20]byte{}
	if meta.InfoHash == zeroHash {
		t.Errorf("InfoHash should not be zero")
	}

	t.Logf("Single-file torrent parsed successfully. InfoHash: %x", meta.InfoHash)
}

// TestMultiFileTorrent tests parsing of a multi-file torrent.
func TestMultiFileTorrent(t *testing.T) {
	// Create a test multi-file torrent
	torrent := buildTestTorrentMultiFile(
		"http://tracker.example.com:8080/announce",
		"myproject",
		[]map[string]interface{}{
			{
				"length": int64(1000),
				"path":   []interface{}{"dir1", "file1.txt"},
			},
			{
				"length": int64(2000),
				"path":   []interface{}{"dir2", "file2.bin"},
			},
			{
				"length": int64(3000),
				"path":   []interface{}{"readme.md"},
			},
		},
		16384,
		makePieces(1),
	)

	meta, err := ParseTorrent(torrent)
	if err != nil {
		t.Fatalf("ParseTorrent failed: %v", err)
	}

	// Verify announce
	if meta.Announce != "http://tracker.example.com:8080/announce" {
		t.Errorf("Expected announce %s, got %s", "http://tracker.example.com:8080/announce", meta.Announce)
	}

	// Verify piece length
	if meta.PieceLength != 16384 {
		t.Errorf("Expected piece length 16384, got %d", meta.PieceLength)
	}

	// Verify name (directory)
	if meta.Name != "myproject" {
		t.Errorf("Expected name myproject, got %s", meta.Name)
	}

	// Verify number of files
	if len(meta.Files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(meta.Files))
	}

	// Verify total length
	expectedTotal := int64(6000)
	if meta.TotalLength != expectedTotal {
		t.Errorf("Expected total length %d, got %d", expectedTotal, meta.TotalLength)
	}

	// Verify file paths and lengths
	expectedFiles := []struct {
		path   string
		length int64
	}{
		{"dir1/file1.txt", 1000},
		{"dir2/file2.bin", 2000},
		{"readme.md", 3000},
	}

	for i, expected := range expectedFiles {
		if i >= len(meta.Files) {
			t.Errorf("Expected file %d, but only have %d files", i, len(meta.Files))
			break
		}
		if meta.Files[i].Path != expected.path {
			t.Errorf("File %d: expected path %s, got %s", i, expected.path, meta.Files[i].Path)
		}
		if meta.Files[i].Length != expected.length {
			t.Errorf("File %d: expected length %d, got %d", i, expected.length, meta.Files[i].Length)
		}
	}

	t.Logf("Multi-file torrent parsed successfully. InfoHash: %x", meta.InfoHash)
}

// TestInfoHashComputation tests that info_hash is computed correctly.
func TestInfoHashComputation(t *testing.T) {
	// Create two torrents with identical info but different announce
	torrent1 := buildTestTorrent(
		"http://tracker1.example.com/announce",
		&infoDict{
			PieceLength: 16384,
			Pieces:      makePieces(1),
			Name:        "test.txt",
			Length:      10000,
		},
	)

	torrent2 := buildTestTorrent(
		"http://tracker2.example.com/announce",
		&infoDict{
			PieceLength: 16384,
			Pieces:      makePieces(1),
			Name:        "test.txt",
			Length:      10000,
		},
	)

	meta1, err := ParseTorrent(torrent1)
	if err != nil {
		t.Fatalf("ParseTorrent(torrent1) failed: %v", err)
	}

	meta2, err := ParseTorrent(torrent2)
	if err != nil {
		t.Fatalf("ParseTorrent(torrent2) failed: %v", err)
	}

	// InfoHash should be the same because the info dict is identical
	if meta1.InfoHash != meta2.InfoHash {
		t.Errorf("InfoHash should be identical for same info dict")
	}

	// But announce URLs are different
	if meta1.Announce == meta2.Announce {
		t.Errorf("Announce URLs should be different")
	}

	t.Logf("Info hash computation verified. Same info dict = same info hash")
}

// TestPieceParsing tests that pieces are correctly parsed as 20-byte chunks.
func TestPieceParsing(t *testing.T) {
	pieces := makePieces(3)
	torrent := buildTestTorrent(
		"http://tracker.example.com/announce",
		&infoDict{
			PieceLength: 16384,
			Pieces:      pieces,
			Name:        "test.txt",
			Length:      50000,
		},
	)

	meta, err := ParseTorrent(torrent)
	if err != nil {
		t.Fatalf("ParseTorrent failed: %v", err)
	}

	if len(meta.Pieces) != 3 {
		t.Errorf("Expected 3 pieces, got %d", len(meta.Pieces))
	}

	// Verify each piece is exactly 20 bytes
	for i, piece := range meta.Pieces {
		if len(piece) != 20 {
			t.Errorf("Piece %d should be 20 bytes, got %d", i, len(piece))
		}
	}

	t.Logf("Piece parsing verified. %d pieces correctly parsed", len(meta.Pieces))
}

// TestInvalidTorrent tests error handling for invalid torrents.
func TestInvalidTorrent(t *testing.T) {
	tests := []struct {
		name      string
		torrent   []byte
		expectErr bool
	}{
		{
			name:      "empty data",
			torrent:   []byte{},
			expectErr: true,
		},
		{
			name:      "invalid bencode",
			torrent:   []byte("not bencode"),
			expectErr: true,
		},
		{
			name:      "dict instead of root",
			torrent:   []byte("li1ei2ee"),
			expectErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseTorrent(test.torrent)
			if test.expectErr && err == nil {
				t.Errorf("Expected error, but got none")
			}
		})
	}
}

// Helper structs and functions for building test torrents

type infoDict struct {
	PieceLength int64
	Pieces      string
	Name        string
	Length      int64
}

// buildTestTorrent creates a bencoded single-file torrent for testing.
func buildTestTorrent(announce string, info *infoDict) []byte {
	torrentDict := map[string]interface{}{
		"announce": announce,
		"info": map[string]interface{}{
			"piece length": info.PieceLength,
			"pieces":       info.Pieces,
			"name":         info.Name,
			"length":       info.Length,
		},
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		panic(err)
	}
	return data
}

// buildTestTorrentMultiFile creates a bencoded multi-file torrent for testing.
func buildTestTorrentMultiFile(announce string, name string, files []map[string]interface{}, pieceLength int64, pieces string) []byte {
	// Convert []map[string]interface{} to []interface{}
	filesInterface := make([]interface{}, len(files))
	for i, f := range files {
		filesInterface[i] = f
	}

	torrentDict := map[string]interface{}{
		"announce": announce,
		"info": map[string]interface{}{
			"piece length": pieceLength,
			"pieces":       pieces,
			"name":         name,
			"files":        filesInterface,
		},
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		panic(err)
	}
	return data
}

// makePieces creates a test pieces string with the specified number of 20-byte SHA1 hashes.
func makePieces(count int) string {
	var pieces string
	for i := 0; i < count; i++ {
		// Create a deterministic 20-byte hash for each piece
		h := sha1.Sum([]byte{byte(i)})
		pieces += string(h[:])
	}
	return pieces
}
