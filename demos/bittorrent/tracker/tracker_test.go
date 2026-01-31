package tracker

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/mccartykim/wong-bittorrent/bencode"
)

// TestGeneratePeerID tests the peer ID generation format
func TestGeneratePeerID(t *testing.T) {
	peerID := GeneratePeerID()

	// Check length is 20 bytes
	if len(peerID) != 20 {
		t.Fatalf("peer ID length: got %d, want 20", len(peerID))
	}

	// Check prefix is "-WG0001-"
	expectedPrefix := []byte("-WG0001-")
	if !bytes.Equal(peerID[:8], expectedPrefix) {
		t.Fatalf("peer ID prefix: got %q, want %q", peerID[:8], expectedPrefix)
	}

	// Check that random bytes are not all zero
	randomBytes := peerID[8:]
	allZero := true
	for _, b := range randomBytes {
		if b != 0 {
			allZero = false
			break
		}
	}
	if allZero {
		t.Fatal("peer ID random bytes are all zero")
	}

	// Generate two peer IDs and verify they're different
	peerID2 := GeneratePeerID()
	if peerID == peerID2 {
		t.Fatal("two generated peer IDs should be different")
	}
}

// TestCompactPeersParsing tests parsing compact peer format
func TestCompactPeersParsing(t *testing.T) {
	// Create compact peer data: 6 bytes per peer
	// Peer 1: 192.168.1.1:6881
	// Peer 2: 10.0.0.1:6882
	var compactPeers []byte
	
	// Peer 1: 192.168.1.1 (c0.a8.01.01) port 6881 (1ae9)
	compactPeers = append(compactPeers, byte(192), byte(168), byte(1), byte(1))
	portBuf := make([]byte, 2)
	binary.BigEndian.PutUint16(portBuf, 6881)
	compactPeers = append(compactPeers, portBuf...)

	// Peer 2: 10.0.0.1 (0a.00.00.01) port 6882 (1aea)
	compactPeers = append(compactPeers, byte(10), byte(0), byte(0), byte(1))
	binary.BigEndian.PutUint16(portBuf, 6882)
	compactPeers = append(compactPeers, portBuf...)

	// Create mock tracker server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Build bencode response
		responseDict := map[string]interface{}{
			"interval": int64(300),
			"peers":    compactPeers,
		}
		encoded, err := bencode.Encode(responseDict)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte(fmt.Sprintf("bencode error: %v", err)))
			return
		}
		w.Header().Set("Content-Type", "application/x-bittorrent")
		w.Write(encoded)
	}))
	defer server.Close()

	// Mock bencode.Encode and Decode for this test
	originalEncode := bencode.Encode
	originalDecode := bencode.Decode

	bencode.Encode = func(v interface{}) ([]byte, error) {
		// Simple bencode encoding for testing
		return encodeTestDict(v)
	}

	bencode.Decode = func(data []byte) (interface{}, error) {
		return decodeTestDict(data)
	}

	defer func() {
		bencode.Encode = originalEncode
		bencode.Decode = originalDecode
	}()

	// Make announce request
	req := &AnnounceRequest{
		AnnounceURL: server.URL,
		InfoHash:    [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		PeerID:      GeneratePeerID(),
		Port:        6881,
		Uploaded:    0,
		Downloaded:  0,
		Left:        1000000,
	}

	resp, err := Announce(context.Background(), req)
	if err != nil {
		t.Fatalf("Announce failed: %v", err)
	}

	// Verify response
	if resp.Interval != 300 {
		t.Fatalf("interval: got %d, want 300", resp.Interval)
	}

	if len(resp.Peers) != 2 {
		t.Fatalf("peers count: got %d, want 2", len(resp.Peers))
	}

	// Check peer 1
	if !net.IP(resp.Peers[0].IP).Equal(net.IPv4(192, 168, 1, 1)) {
		t.Fatalf("peer 1 IP: got %s, want 192.168.1.1", resp.Peers[0].IP)
	}
	if resp.Peers[0].Port != 6881 {
		t.Fatalf("peer 1 port: got %d, want 6881", resp.Peers[0].Port)
	}

	// Check peer 2
	if !net.IP(resp.Peers[1].IP).Equal(net.IPv4(10, 0, 0, 1)) {
		t.Fatalf("peer 2 IP: got %s, want 10.0.0.1", resp.Peers[1].IP)
	}
	if resp.Peers[1].Port != 6882 {
		t.Fatalf("peer 2 port: got %d, want 6882", resp.Peers[1].Port)
	}
}

// TestNonCompactPeersParsing tests parsing non-compact peer format
func TestNonCompactPeersParsing(t *testing.T) {
	// Create mock tracker server with non-compact peer list
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseDict := map[string]interface{}{
			"interval": int64(300),
			"peers": []interface{}{
				map[string]interface{}{
					"ip":   "192.168.1.100",
					"port": int64(6881),
				},
				map[string]interface{}{
					"ip":   "10.0.0.50",
					"port": int64(6882),
				},
			},
		}
		encoded, err := bencode.Encode(responseDict)
		if err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/x-bittorrent")
		w.Write(encoded)
	}))
	defer server.Close()

	// Mock bencode functions
	originalEncode := bencode.Encode
	originalDecode := bencode.Decode

	bencode.Encode = func(v interface{}) ([]byte, error) {
		return encodeTestDict(v)
	}

	bencode.Decode = func(data []byte) (interface{}, error) {
		return decodeTestDict(data)
	}

	defer func() {
		bencode.Encode = originalEncode
		bencode.Decode = originalDecode
	}()

	// Make announce request
	req := &AnnounceRequest{
		AnnounceURL: server.URL,
		InfoHash:    [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20},
		PeerID:      GeneratePeerID(),
		Port:        6881,
		Uploaded:    0,
		Downloaded:  0,
		Left:        1000000,
	}

	resp, err := Announce(context.Background(), req)
	if err != nil {
		t.Fatalf("Announce failed: %v", err)
	}

	if resp.Interval != 300 {
		t.Fatalf("interval: got %d, want 300", resp.Interval)
	}

	if len(resp.Peers) != 2 {
		t.Fatalf("peers count: got %d, want 2", len(resp.Peers))
	}

	// Check peer 1
	if !net.IP(resp.Peers[0].IP).Equal(net.IPv4(192, 168, 1, 100)) {
		t.Fatalf("peer 1 IP: got %s, want 192.168.1.100", resp.Peers[0].IP)
	}
	if resp.Peers[0].Port != 6881 {
		t.Fatalf("peer 1 port: got %d, want 6881", resp.Peers[0].Port)
	}

	// Check peer 2
	if !net.IP(resp.Peers[1].IP).Equal(net.IPv4(10, 0, 0, 50)) {
		t.Fatalf("peer 2 IP: got %s, want 10.0.0.50", resp.Peers[1].IP)
	}
	if resp.Peers[1].Port != 6882 {
		t.Fatalf("peer 2 port: got %d, want 6882", resp.Peers[1].Port)
	}
}

// TestURLEncoding tests that info_hash is properly URL-encoded
func TestURLEncoding(t *testing.T) {
	var urlCaptured string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		urlCaptured = r.RequestURI
		responseDict := map[string]interface{}{
			"interval": int64(300),
			"peers":    []byte{},
		}
		encoded, _ := bencode.Encode(responseDict)
		w.Header().Set("Content-Type", "application/x-bittorrent")
		w.Write(encoded)
	}))
	defer server.Close()

	// Mock bencode functions
	originalEncode := bencode.Encode
	originalDecode := bencode.Decode

	bencode.Encode = func(v interface{}) ([]byte, error) {
		return encodeTestDict(v)
	}

	bencode.Decode = func(data []byte) (interface{}, error) {
		return decodeTestDict(data)
	}

	defer func() {
		bencode.Encode = originalEncode
		bencode.Decode = originalDecode
	}()

	// Create a specific info hash
	var infoHash [20]byte
	for i := 0; i < 20; i++ {
		infoHash[i] = byte(i)
	}

	req := &AnnounceRequest{
		AnnounceURL: server.URL,
		InfoHash:    infoHash,
		PeerID:      GeneratePeerID(),
		Port:        6881,
		Uploaded:    1000,
		Downloaded:  2000,
		Left:        3000,
	}

	_, err := Announce(context.Background(), req)
	if err != nil {
		t.Fatalf("Announce failed: %v", err)
	}

	// Check that info_hash parameter is in the URL
	if !bytes.Contains([]byte(urlCaptured), []byte("info_hash=")) {
		t.Fatalf("info_hash parameter not found in URL: %s", urlCaptured)
	}

	// Check that other parameters are present
	if !bytes.Contains([]byte(urlCaptured), []byte("port=6881")) {
		t.Fatalf("port parameter not found in URL: %s", urlCaptured)
	}
	if !bytes.Contains([]byte(urlCaptured), []byte("uploaded=1000")) {
		t.Fatalf("uploaded parameter not found in URL: %s", urlCaptured)
	}
	if !bytes.Contains([]byte(urlCaptured), []byte("downloaded=2000")) {
		t.Fatalf("downloaded parameter not found in URL: %s", urlCaptured)
	}
	if !bytes.Contains([]byte(urlCaptured), []byte("left=3000")) {
		t.Fatalf("left parameter not found in URL: %s", urlCaptured)
	}
	if !bytes.Contains([]byte(urlCaptured), []byte("compact=1")) {
		t.Fatalf("compact parameter not found in URL: %s", urlCaptured)
	}
}

// TestTrackerFailureReason tests handling of failure reason from tracker
func TestTrackerFailureReason(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		responseDict := map[string]interface{}{
			"failure reason": "Invalid info_hash",
		}
		encoded, _ := bencode.Encode(responseDict)
		w.Header().Set("Content-Type", "application/x-bittorrent")
		w.Write(encoded)
	}))
	defer server.Close()

	// Mock bencode functions
	originalEncode := bencode.Encode
	originalDecode := bencode.Decode

	bencode.Encode = func(v interface{}) ([]byte, error) {
		return encodeTestDict(v)
	}

	bencode.Decode = func(data []byte) (interface{}, error) {
		return decodeTestDict(data)
	}

	defer func() {
		bencode.Encode = originalEncode
		bencode.Decode = originalDecode
	}()

	req := &AnnounceRequest{
		AnnounceURL: server.URL,
		InfoHash:    [20]byte{},
		PeerID:      GeneratePeerID(),
		Port:        6881,
		Uploaded:    0,
		Downloaded:  0,
		Left:        0,
	}

	_, err := Announce(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for failure reason, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("tracker failure")) {
		t.Fatalf("error should contain 'tracker failure', got: %v", err)
	}
}

// TestHTTPError tests handling of HTTP errors
func TestHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("Server error"))
	}))
	defer server.Close()

	req := &AnnounceRequest{
		AnnounceURL: server.URL,
		InfoHash:    [20]byte{},
		PeerID:      GeneratePeerID(),
		Port:        6881,
		Uploaded:    0,
		Downloaded:  0,
		Left:        0,
	}

	_, err := Announce(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for HTTP 500, got nil")
	}
	if !bytes.Contains([]byte(err.Error()), []byte("status")) {
		t.Fatalf("error should mention status code, got: %v", err)
	}
}

// TestMalformedResponse tests handling of malformed bencode response
func TestMalformedResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-bittorrent")
		w.Write([]byte("not valid bencode"))
	}))
	defer server.Close()

	// Mock bencode functions to return error
	originalDecode := bencode.Decode

	bencode.Decode = func(data []byte) (interface{}, error) {
		return nil, fmt.Errorf("invalid bencode data")
	}

	defer func() {
		bencode.Decode = originalDecode
	}()

	req := &AnnounceRequest{
		AnnounceURL: server.URL,
		InfoHash:    [20]byte{},
		PeerID:      GeneratePeerID(),
		Port:        6881,
		Uploaded:    0,
		Downloaded:  0,
		Left:        0,
	}

	_, err := Announce(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for malformed response, got nil")
	}
}

// TestNilRequest tests handling of nil announce request
func TestNilRequest(t *testing.T) {
	_, err := Announce(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil request, got nil")
	}
}

// Helper functions for testing

func encodeTestDict(v interface{}) ([]byte, error) {
	// Simple test encoding that handles the cases we need
	dict, ok := v.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("not a dict")
	}

	var buf bytes.Buffer
	buf.WriteString("d")

	// Encode interval
	if iv, ok := dict["interval"]; ok {
		buf.WriteString("8:interval")
		switch val := iv.(type) {
		case int64:
			buf.WriteString(fmt.Sprintf("i%de", val))
		case int:
			buf.WriteString(fmt.Sprintf("i%de", val))
		}
	}

	// Encode peers if it's bytes (compact format)
	if peers, ok := dict["peers"]; ok {
		buf.WriteString("5:peers")
		switch val := peers.(type) {
		case []byte:
			buf.WriteString(fmt.Sprintf("%d:", len(val)))
			buf.Write(val)
		case []interface{}:
			// Handle list format
			buf.WriteString(fmt.Sprintf("l"))
			for _, p := range val {
				if pDict, ok := p.(map[string]interface{}); ok {
					buf.WriteString("d")
					if ip, ok := pDict["ip"].(string); ok {
						buf.WriteString(fmt.Sprintf("2:ip%d:%s", len(ip), ip))
					}
					if port, ok := pDict["port"].(int64); ok {
						buf.WriteString(fmt.Sprintf("4:porti%de", port))
					} else if port, ok := pDict["port"].(int); ok {
						buf.WriteString(fmt.Sprintf("4:porti%de", port))
					}
					buf.WriteString("e")
				}
			}
			buf.WriteString("e")
		}
	}

	// Encode failure reason if present
	if reason, ok := dict["failure reason"]; ok {
		reasonStr := fmt.Sprintf("%v", reason)
		buf.WriteString(fmt.Sprintf("14:failure reason%d:%s", len(reasonStr), reasonStr))
	}

	buf.WriteString("e")
	return buf.Bytes(), nil
}

func decodeTestDict(data []byte) (interface{}, error) {
	// Simple test decoder that parses our test bencode
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}

	if data[0] != 'd' {
		return nil, fmt.Errorf("not a dict")
	}

	result := make(map[string]interface{})

	// Simple parsing - this is just for testing
	dataStr := string(data)

	// Parse interval
	if bytes.Contains(data, []byte("8:interval")) {
		idx := bytes.Index(data, []byte("8:intervali"))
		if idx != -1 {
			start := idx + len("8:intervali")
			end := bytes.Index(data[start:], []byte("e"))
			if end != -1 {
				val := string(data[start : start+end])
				var num int64
				fmt.Sscanf(val, "%d", &num)
				result["interval"] = num
			}
		}
	}

	// Parse peers (compact format)
	if bytes.Contains(data, []byte("5:peers")) {
		idx := bytes.Index(data, []byte("5:peers"))
		if idx != -1 {
			start := idx + len("5:peers")
			// Find the length
			colonIdx := bytes.Index(data[start:], []byte(":"))
			if colonIdx != -1 {
				lenStr := string(data[start : start+colonIdx])
				var peerLen int
				fmt.Sscanf(lenStr, "%d", &peerLen)
				peersStart := start + colonIdx + 1
				if dataStr[len("d")] == 'l' {
					// Non-compact format - list of dicts
					result["peers"] = parsePeerList(data[peersStart:])
				} else {
					// Compact format
					result["peers"] = data[peersStart : peersStart+peerLen]
				}
			}
		}
	}

	// Parse failure reason
	if bytes.Contains(data, []byte("failure reason")) {
		idx := bytes.Index(data, []byte("14:failure reason"))
		if idx != -1 {
			start := idx + len("14:failure reason")
			colonIdx := bytes.Index(data[start:], []byte(":"))
			if colonIdx != -1 {
				lenStr := string(data[start : start+colonIdx])
				var reasonLen int
				fmt.Sscanf(lenStr, "%d", &reasonLen)
				reasonStart := start + colonIdx + 1
				result["failure reason"] = string(data[reasonStart : reasonStart+reasonLen])
			}
		}
	}

	return result, nil
}

func parsePeerList(data []byte) []interface{} {
	var peers []interface{}
	// Simple peer list parsing
	// This is a simplified version for testing
	return peers
}
