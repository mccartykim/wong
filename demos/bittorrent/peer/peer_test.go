package peer

import (
	"bytes"
	"encoding/binary"
	"net"
	"testing"
)

// TestHandshakeSuccess tests successful handshake between two peers
func TestHandshakeSuccess(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	// Run server handshake in goroutine
	serverDone := make(chan error, 1)
	var serverConn *Conn
	go func() {
		var err error
		serverConn, err = Handshake(server, infoHash, serverID)
		serverDone <- err
	}()

	// Run client handshake
	clientConn, clientErr := Handshake(client, infoHash, clientID)

	if clientErr != nil {
		t.Fatalf("client handshake failed: %v", clientErr)
	}

	// Wait for server handshake to complete
	serverErr := <-serverDone
	if serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}

	// Verify client received correct info from server
	if clientConn.InfoHash != infoHash {
		t.Errorf("client: info hash mismatch, got %v, expected %v", clientConn.InfoHash, infoHash)
	}
	if clientConn.PeerID != serverID {
		t.Errorf("client: peer ID mismatch, got %v, expected %v", clientConn.PeerID, serverID)
	}

	// Verify server received correct info from client
	if serverConn.InfoHash != infoHash {
		t.Errorf("server: info hash mismatch, got %v, expected %v", serverConn.InfoHash, infoHash)
	}
	if serverConn.PeerID != clientID {
		t.Errorf("server: peer ID mismatch, got %v, expected %v", serverConn.PeerID, clientID)
	}
}

// TestHandshakeWrongInfoHash tests handshake with mismatched info hash
func TestHandshakeWrongInfoHash(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	correctHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	wrongHash := [20]byte{20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	// Server uses correct hash
	go func() {
		_, _ = Handshake(server, correctHash, serverID)
	}()

	// Client uses wrong hash
	_, err := Handshake(client, wrongHash, clientID)

	if err == nil {
		t.Fatal("expected handshake to fail with wrong info hash")
	}
}

// TestSendAndReceiveMessage tests sending and receiving messages
func TestSendAndReceiveMessage(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	var clientConn, serverConn *Conn
	serverDone := make(chan error, 1)

	go func() {
		var err error
		serverConn, err = Handshake(server, infoHash, serverID)
		serverDone <- err
	}()

	var err error
	clientConn, err = Handshake(client, infoHash, clientID)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}

	if serverErr := <-serverDone; serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}

	// Test sending message from client to server
	testMsg := &Message{
		ID:      MsgInterested,
		Payload: []byte{},
	}

	// Server must be reading before client writes (net.Pipe() is synchronous)
	receivedMsgChan := make(chan *Message, 1)
	readErrChan := make(chan error, 1)
	go func() {
		msg, err := serverConn.ReadMessage()
		receivedMsgChan <- msg
		readErrChan <- err
	}()

	err = clientConn.SendMessage(testMsg)
	if err != nil {
		t.Fatalf("send message failed: %v", err)
	}

	receivedMsg := <-receivedMsgChan
	if readErr := <-readErrChan; readErr != nil {
		t.Fatalf("read message failed: %v", readErr)
	}

	if receivedMsg.ID != MsgInterested {
		t.Errorf("message ID mismatch, got %d, expected %d", receivedMsg.ID, MsgInterested)
	}
}

// TestKeepAlive tests keep-alive message handling
func TestKeepAlive(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	var clientConn, serverConn *Conn
	serverDone := make(chan error, 1)

	go func() {
		var err error
		serverConn, err = Handshake(server, infoHash, serverID)
		serverDone <- err
	}()

	var err error
	clientConn, err = Handshake(client, infoHash, clientID)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}

	if serverErr := <-serverDone; serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}

	// Server must be reading before client sends keep-alive
	msgChan := make(chan *Message, 1)
	errChan := make(chan error, 1)
	go func() {
		msg, err := serverConn.ReadMessage()
		msgChan <- msg
		errChan <- err
	}()

	// Send keep-alive from client
	err = clientConn.SendKeepAlive()
	if err != nil {
		t.Fatalf("send keep-alive failed: %v", err)
	}

	// Read keep-alive on server (should return nil)
	msg := <-msgChan
	if readErr := <-errChan; readErr != nil {
		t.Fatalf("read keep-alive failed: %v", readErr)
	}

	if msg != nil {
		t.Errorf("expected nil message for keep-alive, got %v", msg)
	}
}

// TestBitfieldOperations tests bitfield get/set operations
func TestBitfieldOperations(t *testing.T) {
	conn := &Conn{
		Bitfield: []byte{},
	}

	// Initially should not have any pieces
	if conn.HasPiece(0) {
		t.Error("piece 0 should not be present initially")
	}

	// Set piece 0
	conn.SetPiece(0)
	if !conn.HasPiece(0) {
		t.Error("piece 0 should be present after SetPiece")
	}

	// Set piece 7 (same byte)
	conn.SetPiece(7)
	if !conn.HasPiece(7) {
		t.Error("piece 7 should be present after SetPiece")
	}
	if !conn.HasPiece(0) {
		t.Error("piece 0 should still be present")
	}

	// Set piece 8 (next byte)
	conn.SetPiece(8)
	if !conn.HasPiece(8) {
		t.Error("piece 8 should be present after SetPiece")
	}

	// Verify piece that was never set
	if conn.HasPiece(1) {
		t.Error("piece 1 should not be present")
	}

	// Test bitfield byte representation
	if len(conn.Bitfield) != 2 {
		t.Errorf("bitfield length should be 2, got %d", len(conn.Bitfield))
	}

	// First byte should have bits 0 and 7 set
	// Bits are numbered 7-0 from left to right
	// Piece 0 = bit 7, piece 7 = bit 0
	// So byte should be 10000001 = 0x81
	if conn.Bitfield[0] != 0x81 {
		t.Errorf("bitfield[0] should be 0x81, got 0x%02x", conn.Bitfield[0])
	}

	// Second byte should have bit 7 set (piece 8)
	// So byte should be 10000000 = 0x80
	if conn.Bitfield[1] != 0x80 {
		t.Errorf("bitfield[1] should be 0x80, got 0x%02x", conn.Bitfield[1])
	}
}

// TestParsePiece tests parsing piece message payload
func TestParsePiece(t *testing.T) {
	blockData := []byte{1, 2, 3, 4, 5}
	payload := new(bytes.Buffer)
	binary.Write(payload, binary.BigEndian, uint32(42))  // index
	binary.Write(payload, binary.BigEndian, uint32(1024)) // begin
	payload.Write(blockData)                               // block data

	msg := &Message{
		ID:      MsgPiece,
		Payload: payload.Bytes(),
	}

	index, begin, data, err := ParsePiece(msg)
	if err != nil {
		t.Fatalf("ParsePiece failed: %v", err)
	}

	if index != 42 {
		t.Errorf("index mismatch, got %d, expected 42", index)
	}
	if begin != 1024 {
		t.Errorf("begin mismatch, got %d, expected 1024", begin)
	}
	if !bytes.Equal(data, blockData) {
		t.Errorf("block data mismatch, got %v, expected %v", data, blockData)
	}
}

// TestParseHave tests parsing have message payload
func TestParseHave(t *testing.T) {
	payload := new(bytes.Buffer)
	binary.Write(payload, binary.BigEndian, uint32(123))

	msg := &Message{
		ID:      MsgHave,
		Payload: payload.Bytes(),
	}

	index, err := ParseHave(msg)
	if err != nil {
		t.Fatalf("ParseHave failed: %v", err)
	}

	if index != 123 {
		t.Errorf("index mismatch, got %d, expected 123", index)
	}
}

// TestParseRequest tests parsing request message payload
func TestParseRequest(t *testing.T) {
	payload := new(bytes.Buffer)
	binary.Write(payload, binary.BigEndian, uint32(10))   // index
	binary.Write(payload, binary.BigEndian, uint32(2048)) // begin
	binary.Write(payload, binary.BigEndian, uint32(16384)) // length

	msg := &Message{
		ID:      MsgRequest,
		Payload: payload.Bytes(),
	}

	index, begin, length, err := ParseRequest(msg)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if index != 10 {
		t.Errorf("index mismatch, got %d, expected 10", index)
	}
	if begin != 2048 {
		t.Errorf("begin mismatch, got %d, expected 2048", begin)
	}
	if length != 16384 {
		t.Errorf("length mismatch, got %d, expected 16384", length)
	}
}

// TestSendInterested tests SendInterested convenience method
func TestSendInterested(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	var clientConn, serverConn *Conn
	serverDone := make(chan error, 1)

	go func() {
		var err error
		serverConn, err = Handshake(server, infoHash, serverID)
		serverDone <- err
	}()

	var err error
	clientConn, err = Handshake(client, infoHash, clientID)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}

	if serverErr := <-serverDone; serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}

	msgChan := make(chan *Message, 1)
	errChan := make(chan error, 1)
	go func() {
		msg, err := serverConn.ReadMessage()
		msgChan <- msg
		errChan <- err
	}()

	err = clientConn.SendInterested()
	if err != nil {
		t.Fatalf("SendInterested failed: %v", err)
	}

	msg := <-msgChan
	if readErr := <-errChan; readErr != nil {
		t.Fatalf("read message failed: %v", readErr)
	}

	if msg.ID != MsgInterested {
		t.Errorf("expected MsgInterested, got %d", msg.ID)
	}
}

// TestSendNotInterested tests SendNotInterested convenience method
func TestSendNotInterested(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	var clientConn, serverConn *Conn
	serverDone := make(chan error, 1)

	go func() {
		var err error
		serverConn, err = Handshake(server, infoHash, serverID)
		serverDone <- err
	}()

	var err error
	clientConn, err = Handshake(client, infoHash, clientID)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}

	if serverErr := <-serverDone; serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}

	msgChan := make(chan *Message, 1)
	errChan := make(chan error, 1)
	go func() {
		msg, err := serverConn.ReadMessage()
		msgChan <- msg
		errChan <- err
	}()

	err = clientConn.SendNotInterested()
	if err != nil {
		t.Fatalf("SendNotInterested failed: %v", err)
	}

	msg := <-msgChan
	if readErr := <-errChan; readErr != nil {
		t.Fatalf("read message failed: %v", readErr)
	}

	if msg.ID != MsgNotInterested {
		t.Errorf("expected MsgNotInterested, got %d", msg.ID)
	}
}

// TestSendRequest tests SendRequest convenience method
func TestSendRequest(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	var clientConn, serverConn *Conn
	serverDone := make(chan error, 1)

	go func() {
		var err error
		serverConn, err = Handshake(server, infoHash, serverID)
		serverDone <- err
	}()

	var err error
	clientConn, err = Handshake(client, infoHash, clientID)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}

	if serverErr := <-serverDone; serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}

	msgChan := make(chan *Message, 1)
	errChan := make(chan error, 1)
	go func() {
		msg, err := serverConn.ReadMessage()
		msgChan <- msg
		errChan <- err
	}()

	err = clientConn.SendRequest(5, 1024, 16384)
	if err != nil {
		t.Fatalf("SendRequest failed: %v", err)
	}

	msg := <-msgChan
	if readErr := <-errChan; readErr != nil {
		t.Fatalf("read message failed: %v", readErr)
	}

	if msg.ID != MsgRequest {
		t.Errorf("expected MsgRequest, got %d", msg.ID)
	}

	index, begin, length, err := ParseRequest(msg)
	if err != nil {
		t.Fatalf("ParseRequest failed: %v", err)
	}

	if index != 5 || begin != 1024 || length != 16384 {
		t.Errorf("request data mismatch: index=%d, begin=%d, length=%d", index, begin, length)
	}
}

// TestSendHave tests SendHave convenience method
func TestSendHave(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	var clientConn, serverConn *Conn
	serverDone := make(chan error, 1)

	go func() {
		var err error
		serverConn, err = Handshake(server, infoHash, serverID)
		serverDone <- err
	}()

	var err error
	clientConn, err = Handshake(client, infoHash, clientID)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}

	if serverErr := <-serverDone; serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}

	msgChan := make(chan *Message, 1)
	errChan := make(chan error, 1)
	go func() {
		msg, err := serverConn.ReadMessage()
		msgChan <- msg
		errChan <- err
	}()

	err = clientConn.SendHave(42)
	if err != nil {
		t.Fatalf("SendHave failed: %v", err)
	}

	msg := <-msgChan
	if readErr := <-errChan; readErr != nil {
		t.Fatalf("read message failed: %v", readErr)
	}

	if msg.ID != MsgHave {
		t.Errorf("expected MsgHave, got %d", msg.ID)
	}

	index, err := ParseHave(msg)
	if err != nil {
		t.Fatalf("ParseHave failed: %v", err)
	}

	if index != 42 {
		t.Errorf("expected index 42, got %d", index)
	}
}

// TestSendChoke tests SendChoke convenience method
func TestSendChoke(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	var clientConn, serverConn *Conn
	serverDone := make(chan error, 1)

	go func() {
		var err error
		serverConn, err = Handshake(server, infoHash, serverID)
		serverDone <- err
	}()

	var err error
	clientConn, err = Handshake(client, infoHash, clientID)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}

	if serverErr := <-serverDone; serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}

	msgChan := make(chan *Message, 1)
	errChan := make(chan error, 1)
	go func() {
		msg, err := serverConn.ReadMessage()
		msgChan <- msg
		errChan <- err
	}()

	err = clientConn.SendChoke()
	if err != nil {
		t.Fatalf("SendChoke failed: %v", err)
	}

	msg := <-msgChan
	if readErr := <-errChan; readErr != nil {
		t.Fatalf("read message failed: %v", readErr)
	}

	if msg.ID != MsgChoke {
		t.Errorf("expected MsgChoke, got %d", msg.ID)
	}
}

// TestSendUnchoke tests SendUnchoke convenience method
func TestSendUnchoke(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	var clientConn, serverConn *Conn
	serverDone := make(chan error, 1)

	go func() {
		var err error
		serverConn, err = Handshake(server, infoHash, serverID)
		serverDone <- err
	}()

	var err error
	clientConn, err = Handshake(client, infoHash, clientID)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}

	if serverErr := <-serverDone; serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}

	msgChan := make(chan *Message, 1)
	errChan := make(chan error, 1)
	go func() {
		msg, err := serverConn.ReadMessage()
		msgChan <- msg
		errChan <- err
	}()

	err = clientConn.SendUnchoke()
	if err != nil {
		t.Fatalf("SendUnchoke failed: %v", err)
	}

	msg := <-msgChan
	if readErr := <-errChan; readErr != nil {
		t.Fatalf("read message failed: %v", readErr)
	}

	if msg.ID != MsgUnchoke {
		t.Errorf("expected MsgUnchoke, got %d", msg.ID)
	}
}

// TestSendBitfield tests SendBitfield convenience method
func TestSendBitfield(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	infoHash := [20]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}
	clientID := [20]byte{100, 101, 102, 103, 104, 105, 106, 107, 108, 109, 110, 111, 112, 113, 114, 115, 116, 117, 118, 119}
	serverID := [20]byte{200, 201, 202, 203, 204, 205, 206, 207, 208, 209, 210, 211, 212, 213, 214, 215, 216, 217, 218, 219}

	var clientConn, serverConn *Conn
	serverDone := make(chan error, 1)

	go func() {
		var err error
		serverConn, err = Handshake(server, infoHash, serverID)
		serverDone <- err
	}()

	var err error
	clientConn, err = Handshake(client, infoHash, clientID)
	if err != nil {
		t.Fatalf("client handshake failed: %v", err)
	}

	if serverErr := <-serverDone; serverErr != nil {
		t.Fatalf("server handshake failed: %v", serverErr)
	}

	msgChan := make(chan *Message, 1)
	errChan := make(chan error, 1)
	go func() {
		msg, err := serverConn.ReadMessage()
		msgChan <- msg
		errChan <- err
	}()

	bitfield := []byte{0xFF, 0x00, 0xAA}
	err = clientConn.SendBitfield(bitfield)
	if err != nil {
		t.Fatalf("SendBitfield failed: %v", err)
	}

	msg := <-msgChan
	if readErr := <-errChan; readErr != nil {
		t.Fatalf("read message failed: %v", readErr)
	}

	if msg.ID != MsgBitfield {
		t.Errorf("expected MsgBitfield, got %d", msg.ID)
	}

	if !bytes.Equal(msg.Payload, bitfield) {
		t.Errorf("bitfield mismatch, got %v, expected %v", msg.Payload, bitfield)
	}
}

// TestParseHaveInvalidMessage tests ParseHave with wrong message type
func TestParseHaveInvalidMessage(t *testing.T) {
	msg := &Message{
		ID:      MsgInterested,
		Payload: []byte{1, 2, 3, 4},
	}

	_, err := ParseHave(msg)
	if err == nil {
		t.Fatal("expected ParseHave to fail with wrong message type")
	}
}

// TestParsePieceInvalidMessage tests ParsePiece with wrong message type
func TestParsePieceInvalidMessage(t *testing.T) {
	msg := &Message{
		ID:      MsgInterested,
		Payload: []byte{1, 2, 3, 4},
	}

	_, _, _, err := ParsePiece(msg)
	if err == nil {
		t.Fatal("expected ParsePiece to fail with wrong message type")
	}
}

// TestParseRequestInvalidMessage tests ParseRequest with wrong message type
func TestParseRequestInvalidMessage(t *testing.T) {
	msg := &Message{
		ID:      MsgInterested,
		Payload: []byte{1, 2, 3, 4},
	}

	_, _, _, err := ParseRequest(msg)
	if err == nil {
		t.Fatal("expected ParseRequest to fail with wrong message type")
	}
}
