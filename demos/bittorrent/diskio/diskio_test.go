package diskio

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// Test 1: Single-file write and read-back
func TestSingleFileWriteAndReadBack(t *testing.T) {
	tempDir := t.TempDir()
	pieceLen := 16 * 1024 // 16 KB
	fileLen := int64(32 * 1024)

	files := []FileEntry{
		{
			Path:   "file.bin",
			Length: fileLen,
			Offset: 0,
		},
	}

	w, err := NewWriter(tempDir, "test", pieceLen, fileLen, files)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	defer w.Close()

	// Write two pieces
	piece0Data := bytes.Repeat([]byte{0xAA}, pieceLen)
	piece1Data := bytes.Repeat([]byte{0xBB}, pieceLen)

	if err := w.WritePiece(0, piece0Data); err != nil {
		t.Fatalf("WritePiece(0) failed: %v", err)
	}

	if err := w.WritePiece(1, piece1Data); err != nil {
		t.Fatalf("WritePiece(1) failed: %v", err)
	}

	// Read back and verify
	read0, err := w.ReadPiece(0, pieceLen)
	if err != nil {
		t.Fatalf("ReadPiece(0) failed: %v", err)
	}

	if !bytes.Equal(read0, piece0Data) {
		t.Fatalf("piece 0 mismatch: expected %d bytes of 0xAA, got different data", pieceLen)
	}

	read1, err := w.ReadPiece(1, pieceLen)
	if err != nil {
		t.Fatalf("ReadPiece(1) failed: %v", err)
	}

	if !bytes.Equal(read1, piece1Data) {
		t.Fatalf("piece 1 mismatch: expected %d bytes of 0xBB, got different data", pieceLen)
	}
}

// Test 2: Multi-file write spanning file boundary
func TestMultiFileSpanningBoundary(t *testing.T) {
	tempDir := t.TempDir()
	pieceLen := 20 * 1024 // 20 KB, will span file boundary
	file1Len := int64(15 * 1024)
	file2Len := int64(25 * 1024)
	totalLen := file1Len + file2Len

	files := []FileEntry{
		{
			Path:   "part1.bin",
			Length: file1Len,
			Offset: 0,
		},
		{
			Path:   "part2.bin",
			Length: file2Len,
			Offset: file1Len,
		},
	}

	w, err := NewWriter(tempDir, "test", pieceLen, totalLen, files)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	defer w.Close()

	// Write a piece that spans the file boundary
	pieceData := bytes.Repeat([]byte{0xCC}, pieceLen)
	if err := w.WritePiece(0, pieceData); err != nil {
		t.Fatalf("WritePiece(0) failed: %v", err)
	}

	// Read back and verify
	readData, err := w.ReadPiece(0, pieceLen)
	if err != nil {
		t.Fatalf("ReadPiece(0) failed: %v", err)
	}

	if !bytes.Equal(readData, pieceData) {
		t.Fatalf("piece spanning boundary mismatch: got different data")
	}

	// Verify data was split correctly across files
	file1Path := filepath.Join(tempDir, "part1.bin")
	file1Data, err := os.ReadFile(file1Path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	if len(file1Data) != int(file1Len) {
		t.Fatalf("file1 size mismatch: expected %d, got %d", file1Len, len(file1Data))
	}

	// First 15 KB should be 0xCC
	for i, b := range file1Data {
		if b != 0xCC {
			t.Fatalf("file1 data mismatch at offset %d: expected 0xCC, got 0x%02X", i, b)
		}
	}

	file2Path := filepath.Join(tempDir, "part2.bin")
	file2Data, err := os.ReadFile(file2Path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	// First 5 KB of file2 should be 0xCC (to complete the 20 KB piece)
	for i := 0; i < 5*1024 && i < len(file2Data); i++ {
		if file2Data[i] != 0xCC {
			t.Fatalf("file2 data mismatch at offset %d: expected 0xCC, got 0x%02X", i, file2Data[i])
		}
	}
}

// Test 3: Write all pieces, verify file sizes match
func TestWriteAllPiecesAndVerifySizes(t *testing.T) {
	tempDir := t.TempDir()
	pieceLen := 10 * 1024 // 10 KB
	file1Len := int64(25 * 1024)
	file2Len := int64(15 * 1024)
	totalLen := file1Len + file2Len

	files := []FileEntry{
		{
			Path:   "file1.bin",
			Length: file1Len,
			Offset: 0,
		},
		{
			Path:   "file2.bin",
			Length: file2Len,
			Offset: file1Len,
		},
	}

	w, err := NewWriter(tempDir, "test", pieceLen, totalLen, files)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}

	// Write all pieces
	numPieces := (totalLen + int64(pieceLen) - 1) / int64(pieceLen)
	for i := 0; i < int(numPieces); i++ {
		size := pieceLen
		if int64(i+1)*int64(pieceLen) > totalLen {
			size = int(totalLen - int64(i)*int64(pieceLen))
		}
		pieceData := bytes.Repeat([]byte{byte(i)}, size)
		if err := w.WritePiece(i, pieceData); err != nil {
			t.Fatalf("WritePiece(%d) failed: %v", i, err)
		}
	}

	w.Close()

	// Verify file sizes
	file1Path := filepath.Join(tempDir, "file1.bin")
	file1Info, err := os.Stat(file1Path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if file1Info.Size() != file1Len {
		t.Fatalf("file1 size mismatch: expected %d, got %d", file1Len, file1Info.Size())
	}

	file2Path := filepath.Join(tempDir, "file2.bin")
	file2Info, err := os.Stat(file2Path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if file2Info.Size() != file2Len {
		t.Fatalf("file2 size mismatch: expected %d, got %d", file2Len, file2Info.Size())
	}
}

// Test 4: Last piece shorter than piece length
func TestLastPieceShorterThanPieceLength(t *testing.T) {
	tempDir := t.TempDir()
	pieceLen := 10 * 1024 // 10 KB
	totalLen := int64(25500)  // Not a multiple of pieceLen

	files := []FileEntry{
		{
			Path:   "partial.bin",
			Length: totalLen,
			Offset: 0,
		},
	}

	w, err := NewWriter(tempDir, "test", pieceLen, totalLen, files)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	defer w.Close()

	// Write full pieces
	piece0Data := bytes.Repeat([]byte{0x11}, pieceLen)
	piece1Data := bytes.Repeat([]byte{0x22}, pieceLen)

	if err := w.WritePiece(0, piece0Data); err != nil {
		t.Fatalf("WritePiece(0) failed: %v", err)
	}
	if err := w.WritePiece(1, piece1Data); err != nil {
		t.Fatalf("WritePiece(1) failed: %v", err)
	}

	// Write last piece (shorter)
	lastPieceLen := totalLen - 2*int64(pieceLen)
	lastPieceData := bytes.Repeat([]byte{0x44}, int(lastPieceLen))
	if err := w.WritePiece(2, lastPieceData); err != nil {
		t.Fatalf("WritePiece(2) failed: %v", err)
	}

	// Read and verify
	read0, err := w.ReadPiece(0, pieceLen)
	if err != nil {
		t.Fatalf("ReadPiece(0) failed: %v", err)
	}

	if !bytes.Equal(read0, piece0Data) {
		t.Fatalf("piece 0 mismatch")
	}

	read1, err := w.ReadPiece(1, pieceLen)
	if err != nil {
		t.Fatalf("ReadPiece(1) failed: %v", err)
	}

	if !bytes.Equal(read1, piece1Data) {
		t.Fatalf("piece 1 mismatch")
	}

	read2, err := w.ReadPiece(2, int(lastPieceLen))
	if err != nil {
		t.Fatalf("ReadPiece(2) failed: %v", err)
	}

	if !bytes.Equal(read2, lastPieceData) {
		t.Fatalf("last piece mismatch")
	}
}

// Test 5: ReadPiece returns correct data
func TestReadPieceCorrectData(t *testing.T) {
	tempDir := t.TempDir()
	pieceLen := 8 * 1024
	totalLen := int64(3 * pieceLen)

	files := []FileEntry{
		{
			Path:   "data.bin",
			Length: totalLen,
			Offset: 0,
		},
	}

	w, err := NewWriter(tempDir, "test", pieceLen, totalLen, files)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	defer w.Close()

	// Write specific data to each piece
	testData := [][]byte{
		bytes.Repeat([]byte{0x12}, pieceLen),
		bytes.Repeat([]byte{0x34}, pieceLen),
		bytes.Repeat([]byte{0x56}, pieceLen),
	}

	for i, data := range testData {
		if err := w.WritePiece(i, data); err != nil {
			t.Fatalf("WritePiece(%d) failed: %v", i, err)
		}
	}

	// Read and verify each piece
	for i, expectedData := range testData {
		readData, err := w.ReadPiece(i, pieceLen)
		if err != nil {
			t.Fatalf("ReadPiece(%d) failed: %v", i, err)
		}

		if !bytes.Equal(readData, expectedData) {
			t.Fatalf("piece %d data mismatch", i)
		}
	}
}

// Test 6: Error handling - write to non-existent directory
func TestErrorNonExistentOutputDir(t *testing.T) {
	// Use a path in a directory that definitely won't be accessible
	nonExistentDir := "/dev/null/bittorrent-test-nonexistent/subdir"
	pieceLen := 16 * 1024
	totalLen := int64(32 * 1024)

	files := []FileEntry{
		{
			Path:   "file.bin",
			Length: totalLen,
			Offset: 0,
		},
	}

	// NewWriter should fail because parent directory doesn't exist
	_, err := NewWriter(nonExistentDir, "test", pieceLen, totalLen, files)
	if err == nil {
		t.Fatalf("NewWriter should have failed for non-existent directory")
	}
}

// Test 7: Multi-file with nested directories
func TestMultiFileNestedDirectories(t *testing.T) {
	tempDir := t.TempDir()
	pieceLen := 10 * 1024
	totalLen := int64(2 * pieceLen)

	files := []FileEntry{
		{
			Path:   "subdir1/file1.bin",
			Length: int64(pieceLen),
			Offset: 0,
		},
		{
			Path:   "subdir2/subdir3/file2.bin",
			Length: int64(pieceLen),
			Offset: int64(pieceLen),
		},
	}

	w, err := NewWriter(tempDir, "test", pieceLen, totalLen, files)
	if err != nil {
		t.Fatalf("NewWriter failed: %v", err)
	}
	defer w.Close()

	// Write and read back
	piece0Data := bytes.Repeat([]byte{0xFF}, pieceLen)
	piece1Data := bytes.Repeat([]byte{0x00}, pieceLen)

	if err := w.WritePiece(0, piece0Data); err != nil {
		t.Fatalf("WritePiece(0) failed: %v", err)
	}
	if err := w.WritePiece(1, piece1Data); err != nil {
		t.Fatalf("WritePiece(1) failed: %v", err)
	}

	// Verify directories were created
	file1Path := filepath.Join(tempDir, "subdir1/file1.bin")
	if _, err := os.Stat(file1Path); err != nil {
		t.Fatalf("file1 not created: %v", err)
	}

	file2Path := filepath.Join(tempDir, "subdir2/subdir3/file2.bin")
	if _, err := os.Stat(file2Path); err != nil {
		t.Fatalf("file2 not created: %v", err)
	}

	// Read back and verify
	read0, err := w.ReadPiece(0, pieceLen)
	if err != nil {
		t.Fatalf("ReadPiece(0) failed: %v", err)
	}

	if !bytes.Equal(read0, piece0Data) {
		t.Fatalf("piece 0 mismatch")
	}

	read1, err := w.ReadPiece(1, pieceLen)
	if err != nil {
		t.Fatalf("ReadPiece(1) failed: %v", err)
	}

	if !bytes.Equal(read1, piece1Data) {
		t.Fatalf("piece 1 mismatch")
	}
}
