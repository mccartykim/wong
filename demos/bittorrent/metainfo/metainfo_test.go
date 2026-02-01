package metainfo

import (
	"bytes"
	"crypto/sha1"
	"os"
	"path/filepath"
	"testing"

	"github.com/mccartykim/wong-bittorrent/bencode"
)

// TestParseSingleFileTorrent tests parsing a single-file torrent
func TestParseSingleFileTorrent(t *testing.T) {
	// Create a synthetic single-file torrent
	infoDict := map[string]interface{}{
		"name":         "test.txt",
		"length":       int64(1024),
		"piece length": int64(16384),
		"pieces":       "12345678901234567890abcdefghij1",
	}

	torrentDict := map[string]interface{}{
		"announce": "http://example.com/announce",
		"info":     infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	torrent, err := ParseFromBytes(data)
	if err != nil {
		t.Fatalf("Failed to parse torrent: %v", err)
	}

	if torrent.Announce != "http://example.com/announce" {
		t.Errorf("Expected announce 'http://example.com/announce', got '%s'", torrent.Announce)
	}

	if torrent.Name != "test.txt" {
		t.Errorf("Expected name 'test.txt', got '%s'", torrent.Name)
	}

	if torrent.Length != 1024 {
		t.Errorf("Expected length 1024, got %d", torrent.Length)
	}

	if torrent.PieceLength != 16384 {
		t.Errorf("Expected piece length 16384, got %d", torrent.PieceLength)
	}

	if len(torrent.Pieces) != 1 {
		t.Errorf("Expected 1 piece, got %d", len(torrent.Pieces))
	}

	if torrent.IsMultiFile() {
		t.Errorf("Expected single-file torrent, but IsMultiFile returned true")
	}

	if torrent.TotalLength() != 1024 {
		t.Errorf("Expected total length 1024, got %d", torrent.TotalLength())
	}

	// Verify InfoHash is computed correctly
	infoBencoded, _ := bencode.Encode(infoDict)
	expectedHash := sha1.Sum(infoBencoded)
	if torrent.InfoHash != expectedHash {
		t.Errorf("InfoHash mismatch")
	}
}

// TestParseMultiFileTorrent tests parsing a multi-file torrent
func TestParseMultiFileTorrent(t *testing.T) {
	filesData := []interface{}{
		map[string]interface{}{
			"length": int64(512),
			"path":   []interface{}{"subdir", "file1.txt"},
		},
		map[string]interface{}{
			"length": int64(768),
			"path":   []interface{}{"subdir", "file2.txt"},
		},
	}

	infoDict := map[string]interface{}{
		"name":         "myfiles",
		"piece length": int64(16384),
		"pieces":       "12345678901234567890abcdefghij1",
		"files":        filesData,
	}

	torrentDict := map[string]interface{}{
		"announce": "http://tracker.example.com:6969/announce",
		"info":     infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	torrent, err := ParseFromBytes(data)
	if err != nil {
		t.Fatalf("Failed to parse torrent: %v", err)
	}

	if torrent.Announce != "http://tracker.example.com:6969/announce" {
		t.Errorf("Expected announce URL, got '%s'", torrent.Announce)
	}

	if torrent.Name != "myfiles" {
		t.Errorf("Expected name 'myfiles', got '%s'", torrent.Name)
	}

	if !torrent.IsMultiFile() {
		t.Errorf("Expected multi-file torrent, but IsMultiFile returned false")
	}

	if len(torrent.Files) != 2 {
		t.Errorf("Expected 2 files, got %d", len(torrent.Files))
	}

	if torrent.Files[0].Length != 512 {
		t.Errorf("Expected first file length 512, got %d", torrent.Files[0].Length)
	}

	if torrent.Files[1].Length != 768 {
		t.Errorf("Expected second file length 768, got %d", torrent.Files[1].Length)
	}

	if len(torrent.Files[0].Path) != 2 || torrent.Files[0].Path[0] != "subdir" || torrent.Files[0].Path[1] != "file1.txt" {
		t.Errorf("Expected path ['subdir', 'file1.txt'], got %v", torrent.Files[0].Path)
	}

	expectedTotal := int64(512 + 768)
	if torrent.TotalLength() != expectedTotal {
		t.Errorf("Expected total length %d, got %d", expectedTotal, torrent.TotalLength())
	}
}

// TestInfoHashComputation tests that InfoHash is correctly computed
func TestInfoHashComputation(t *testing.T) {
	infoDict := map[string]interface{}{
		"name":         "ubuntu.iso",
		"length":       int64(1234567890),
		"piece length": int64(262144),
		"pieces":       "12345678901234567890",
	}

	torrentDict := map[string]interface{}{
		"announce": "http://tracker.ubuntu.com/announce",
		"info":     infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	torrent, err := ParseFromBytes(data)
	if err != nil {
		t.Fatalf("Failed to parse torrent: %v", err)
	}

	// Manually compute expected InfoHash
	infoBencoded, _ := bencode.Encode(infoDict)
	expectedHash := sha1.Sum(infoBencoded)

	if torrent.InfoHash != expectedHash {
		t.Errorf("InfoHash mismatch. Expected %x, got %x", expectedHash, torrent.InfoHash)
	}

	// Verify it's 20 bytes
	if len(torrent.InfoHash) != 20 {
		t.Errorf("Expected InfoHash to be 20 bytes, got %d", len(torrent.InfoHash))
	}
}

// TestAnnouncelist tests parsing announce-list (BEP 12)
func TestAnnounceList(t *testing.T) {
	announceList := []interface{}{
		[]interface{}{"http://tracker1.example.com/announce", "http://tracker2.example.com/announce"},
		[]interface{}{"http://backup.example.com/announce"},
	}

	infoDict := map[string]interface{}{
		"name":         "test",
		"length":       int64(100),
		"piece length": int64(16384),
		"pieces":       "12345678901234567890",
	}

	torrentDict := map[string]interface{}{
		"announce":      "http://primary.example.com/announce",
		"announce-list": announceList,
		"info":          infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	torrent, err := ParseFromBytes(data)
	if err != nil {
		t.Fatalf("Failed to parse torrent: %v", err)
	}

	if len(torrent.AnnounceList) != 2 {
		t.Errorf("Expected 2 announce tiers, got %d", len(torrent.AnnounceList))
	}

	if len(torrent.AnnounceList[0]) != 2 {
		t.Errorf("Expected first tier to have 2 URLs, got %d", len(torrent.AnnounceList[0]))
	}

	if torrent.AnnounceList[0][0] != "http://tracker1.example.com/announce" {
		t.Errorf("Expected first URL in tier 0, got %s", torrent.AnnounceList[0][0])
	}
}

// TestPiecesParsing tests that pieces are correctly parsed from concatenated bytes
func TestPiecesParsing(t *testing.T) {
	// Create a piece string with 3 pieces (3 * 20 bytes = 60 bytes)
	pieces := bytes.Repeat([]byte{0x01}, 20)
	pieces = append(pieces, bytes.Repeat([]byte{0x02}, 20)...)
	pieces = append(pieces, bytes.Repeat([]byte{0x03}, 20)...)

	infoDict := map[string]interface{}{
		"name":         "test",
		"length":       int64(1000),
		"piece length": int64(400),
		"pieces":       string(pieces),
	}

	torrentDict := map[string]interface{}{
		"announce": "http://example.com/announce",
		"info":     infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	torrent, err := ParseFromBytes(data)
	if err != nil {
		t.Fatalf("Failed to parse torrent: %v", err)
	}

	if len(torrent.Pieces) != 3 {
		t.Errorf("Expected 3 pieces, got %d", len(torrent.Pieces))
	}

	// Verify each piece hash
	for i := 0; i < 3; i++ {
		if len(torrent.Pieces[i]) != 20 {
			t.Errorf("Piece %d: expected 20 bytes, got %d", i, len(torrent.Pieces[i]))
		}
	}
}

// TestErrorMissingAnnounce tests error handling for missing announce
func TestErrorMissingAnnounce(t *testing.T) {
	infoDict := map[string]interface{}{
		"name":         "test",
		"length":       int64(100),
		"piece length": int64(16384),
		"pieces":       "12345678901234567890",
	}

	torrentDict := map[string]interface{}{
		// Missing announce
		"info": infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	torrent, err := ParseFromBytes(data)
	if err != nil {
		t.Fatalf("Failed to parse torrent: %v", err)
	}

	// Should succeed but with empty announce
	if torrent.Announce != "" {
		t.Errorf("Expected empty announce, got '%s'", torrent.Announce)
	}
}

// TestErrorMissingInfo tests error handling for missing info dict
func TestErrorMissingInfo(t *testing.T) {
	torrentDict := map[string]interface{}{
		"announce": "http://example.com/announce",
		// Missing info
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	_, err = ParseFromBytes(data)
	if err == nil {
		t.Errorf("Expected error for missing info dict, got nil")
	}
}

// TestErrorMissingPieceLength tests error handling for missing piece length
func TestErrorMissingPieceLength(t *testing.T) {
	infoDict := map[string]interface{}{
		"name":   "test",
		"length": int64(100),
		// Missing piece length
		"pieces": "12345678901234567890",
	}

	torrentDict := map[string]interface{}{
		"announce": "http://example.com/announce",
		"info":     infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	_, err = ParseFromBytes(data)
	if err == nil {
		t.Errorf("Expected error for missing piece length, got nil")
	}
}

// TestErrorMissingPieces tests error handling for missing pieces
func TestErrorMissingPieces(t *testing.T) {
	infoDict := map[string]interface{}{
		"name":         "test",
		"length":       int64(100),
		"piece length": int64(16384),
		// Missing pieces
	}

	torrentDict := map[string]interface{}{
		"announce": "http://example.com/announce",
		"info":     infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	_, err = ParseFromBytes(data)
	if err == nil {
		t.Errorf("Expected error for missing pieces, got nil")
	}
}

// TestErrorMissingName tests error handling for missing name
func TestErrorMissingName(t *testing.T) {
	infoDict := map[string]interface{}{
		// Missing name
		"length":       int64(100),
		"piece length": int64(16384),
		"pieces":       "12345678901234567890",
	}

	torrentDict := map[string]interface{}{
		"announce": "http://example.com/announce",
		"info":     infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	_, err = ParseFromBytes(data)
	if err == nil {
		t.Errorf("Expected error for missing name, got nil")
	}
}

// TestErrorMissingLengthAndFiles tests error handling when both length and files are missing
func TestErrorMissingLengthAndFiles(t *testing.T) {
	infoDict := map[string]interface{}{
		"name":         "test",
		"piece length": int64(16384),
		"pieces":       "12345678901234567890",
		// Missing both length and files
	}

	torrentDict := map[string]interface{}{
		"announce": "http://example.com/announce",
		"info":     infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	_, err = ParseFromBytes(data)
	if err == nil {
		t.Errorf("Expected error for missing length and files, got nil")
	}
}

// TestParseFromFile tests parsing a torrent from a file
func TestParseFromFile(t *testing.T) {
	// Create a temporary torrent file
	infoDict := map[string]interface{}{
		"name":         "test.bin",
		"length":       int64(2048),
		"piece length": int64(16384),
		"pieces":       "12345678901234567890",
	}

	torrentDict := map[string]interface{}{
		"announce": "http://example.com/announce",
		"info":     infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	// Write to temporary file
	tmpDir := t.TempDir()
	torrentPath := filepath.Join(tmpDir, "test.torrent")
	err = os.WriteFile(torrentPath, data, 0644)
	if err != nil {
		t.Fatalf("Failed to write temporary torrent file: %v", err)
	}

	// Parse from file
	torrent, err := ParseFromFile(torrentPath)
	if err != nil {
		t.Fatalf("Failed to parse torrent from file: %v", err)
	}

	if torrent.Name != "test.bin" {
		t.Errorf("Expected name 'test.bin', got '%s'", torrent.Name)
	}

	if torrent.Length != 2048 {
		t.Errorf("Expected length 2048, got %d", torrent.Length)
	}
}

// TestParseFromFileNotFound tests error handling for missing file
func TestParseFromFileNotFound(t *testing.T) {
	_, err := ParseFromFile("/nonexistent/path/to/file.torrent")
	if err == nil {
		t.Errorf("Expected error for missing file, got nil")
	}
}

// TestComplexMultiFile tests a more complex multi-file scenario
func TestComplexMultiFile(t *testing.T) {
	filesData := []interface{}{
		map[string]interface{}{
			"length": int64(1000),
			"path":   []interface{}{"movie.mkv"},
		},
		map[string]interface{}{
			"length": int64(2000),
			"path":   []interface{}{"subtitles", "en.srt"},
		},
		map[string]interface{}{
			"length": int64(3000),
			"path":   []interface{}{"subtitles", "fr.srt"},
		},
	}

	infoDict := map[string]interface{}{
		"name":         "movie_pack",
		"piece length": int64(262144),
		"pieces":       "1234567890123456789012345678901234567890",
		"files":        filesData,
	}

	torrentDict := map[string]interface{}{
		"announce": "http://tracker.example.com/announce",
		"info":     infoDict,
	}

	data, err := bencode.Encode(torrentDict)
	if err != nil {
		t.Fatalf("Failed to encode torrent: %v", err)
	}

	torrent, err := ParseFromBytes(data)
	if err != nil {
		t.Fatalf("Failed to parse torrent: %v", err)
	}

	expectedTotal := int64(1000 + 2000 + 3000)
	if torrent.TotalLength() != expectedTotal {
		t.Errorf("Expected total length %d, got %d", expectedTotal, torrent.TotalLength())
	}

	if len(torrent.Pieces) != 2 {
		t.Errorf("Expected 2 pieces, got %d", len(torrent.Pieces))
	}

	// Verify file paths
	if len(torrent.Files[1].Path) != 2 || torrent.Files[1].Path[0] != "subtitles" {
		t.Errorf("Expected nested path for second file")
	}
}
