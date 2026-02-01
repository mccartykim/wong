package diskio

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// FileEntry represents a file in the torrent's file list.
type FileEntry struct {
	Path   string // relative path
	Length int64
	Offset int64  // absolute offset in the torrent's byte stream
}

// Writer handles writing and reading pieces to/from disk.
type Writer struct {
	outputDir  string
	files      []FileEntry
	pieceLen   int
	totalLen   int64
	mu         sync.Mutex
	fileMap    map[string]*os.File // cache of open files
}

// NewWriter creates a new Writer and pre-creates all files.
// For single-file torrents: files has one entry
// For multi-file torrents: files has multiple entries, creates subdirectories under name/
func NewWriter(outputDir string, name string, pieceLength int, totalLength int64, files []FileEntry) (*Writer, error) {
	// Check if output directory exists
	if _, err := os.Stat(outputDir); err != nil {
		return nil, fmt.Errorf("output directory does not exist: %w", err)
	}

	w := &Writer{
		outputDir: outputDir,
		files:     files,
		pieceLen:  pieceLength,
		totalLen:  totalLength,
		fileMap:   make(map[string]*os.File),
	}

	// Create all files and directories
	for _, f := range files {
		fullPath := filepath.Join(outputDir, f.Path)
		dir := filepath.Dir(fullPath)

		// Create parent directories
		if err := os.MkdirAll(dir, 0755); err != nil {
			w.Close()
			return nil, fmt.Errorf("failed to create directory %s: %w", dir, err)
		}

		// Create or truncate the file to the correct size
		file, err := os.OpenFile(fullPath, os.O_CREATE|os.O_RDWR|os.O_TRUNC, 0644)
		if err != nil {
			w.Close()
			return nil, fmt.Errorf("failed to create file %s: %w", fullPath, err)
		}

		// Truncate to the correct size
		if err := file.Truncate(f.Length); err != nil {
			file.Close()
			w.Close()
			return nil, fmt.Errorf("failed to truncate file %s: %w", fullPath, err)
		}

		w.fileMap[f.Path] = file
	}

	return w, nil
}

// WritePiece writes a piece to disk, handling file boundaries.
func (w *Writer) WritePiece(index int, data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Calculate the byte offset for this piece
	pieceOffset := int64(index) * int64(w.pieceLen)

	// Write data across file boundaries if needed
	offset := int64(0)
	for offset < int64(len(data)) {
		// Find which file this offset belongs to
		file, fileOffset, err := w.findFileForOffset(pieceOffset + offset)
		if err != nil {
			return err
		}

		// Write as much as we can to this file
		remaining := int64(len(data)) - offset
		availableInFile := file.Length - fileOffset
		toWrite := remaining
		if toWrite > availableInFile {
			toWrite = availableInFile
		}

		// Seek to the correct position in the file
		if _, err := file.File.Seek(fileOffset, 0); err != nil {
			return fmt.Errorf("failed to seek in file %s: %w", file.Path, err)
		}

		// Write the data
		n, err := file.File.Write(data[offset : offset+toWrite])
		if err != nil {
			return fmt.Errorf("failed to write to file %s: %w", file.Path, err)
		}

		if int64(n) != toWrite {
			return fmt.Errorf("partial write to file %s: wrote %d, expected %d", file.Path, n, toWrite)
		}

		offset += toWrite
	}

	return nil
}

// ReadPiece reads a piece from disk, handling file boundaries.
func (w *Writer) ReadPiece(index int, length int) ([]byte, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	data := make([]byte, length)
	pieceOffset := int64(index) * int64(w.pieceLen)

	offset := int64(0)
	for offset < int64(length) {
		// Find which file this offset belongs to
		file, fileOffset, err := w.findFileForOffset(pieceOffset + offset)
		if err != nil {
			return nil, err
		}

		// Read as much as we can from this file
		remaining := int64(length) - offset
		availableInFile := file.Length - fileOffset
		toRead := remaining
		if toRead > availableInFile {
			toRead = availableInFile
		}

		// Seek to the correct position in the file
		if _, err := file.File.Seek(fileOffset, 0); err != nil {
			return nil, fmt.Errorf("failed to seek in file %s: %w", file.Path, err)
		}

		// Read the data
		n, err := file.File.Read(data[offset : offset+toRead])
		if err != nil && err != io.EOF {
			return nil, fmt.Errorf("failed to read from file %s: %w", file.Path, err)
		}

		if int64(n) != toRead {
			return nil, fmt.Errorf("partial read from file %s: read %d, expected %d", file.Path, n, toRead)
		}

		offset += toRead
	}

	return data, nil
}

// Close closes all open files.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var errs []error
	for _, file := range w.fileMap {
		if err := file.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close files: %v", errs)
	}

	return nil
}

// fileInfo holds a file reference along with metadata
type fileInfo struct {
	Path   string
	File   *os.File
	Length int64
	Offset int64
}

// findFileForOffset finds the file and offset within that file for a given absolute offset.
func (w *Writer) findFileForOffset(offset int64) (*fileInfo, int64, error) {
	for _, f := range w.files {
		if offset >= f.Offset && offset < f.Offset+f.Length {
			file, ok := w.fileMap[f.Path]
			if !ok {
				return nil, 0, fmt.Errorf("file not open: %s", f.Path)
			}
			return &fileInfo{
				Path:   f.Path,
				File:   file,
				Length: f.Length,
				Offset: f.Offset,
			}, offset - f.Offset, nil
		}
	}

	return nil, 0, fmt.Errorf("offset %d out of range (total: %d)", offset, w.totalLen)
}
