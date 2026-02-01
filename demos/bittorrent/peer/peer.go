package peer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
)

// Message IDs for BitTorrent peer wire protocol (BEP 3)
const (
	MsgChoke         uint8 = 0
	MsgUnchoke       uint8 = 1
	MsgInterested    uint8 = 2
	MsgNotInterested uint8 = 3
	MsgHave          uint8 = 4
	MsgBitfield      uint8 = 5
	MsgRequest       uint8 = 6
	MsgPiece         uint8 = 7
	MsgCancel        uint8 = 8
)

// Message represents a peer wire protocol message
type Message struct {
	ID      uint8
	Payload []byte
}

// Conn represents a connection to a peer
type Conn struct {
	conn     net.Conn
	InfoHash [20]byte
	PeerID   [20]byte
	Bitfield []byte
	Choked   bool
}

// Handshake performs the BitTorrent handshake with a peer.
// Format: <pstrlen=19><pstr="BitTorrent protocol"><reserved=8 bytes><info_hash=20><peer_id=20>
func Handshake(conn net.Conn, infoHash, peerID [20]byte) (*Conn, error) {
	pstr := "BitTorrent protocol"
	handshakeSize := 1 + len(pstr) + 8 + 20 + 20

	// Build our handshake
	handshake := make([]byte, handshakeSize)
	handshake[0] = byte(len(pstr))
	copy(handshake[1:], pstr)
	copy(handshake[1+len(pstr)+8:], infoHash[:])
	copy(handshake[1+len(pstr)+8+20:], peerID[:])

	// Read peer's handshake concurrently with writing ours
	// This is necessary because net.Pipe() is synchronous and blocks on writes
	peerBuf := make([]byte, handshakeSize)
	readDone := make(chan error, 1)

	go func() {
		_, err := io.ReadFull(conn, peerBuf)
		readDone <- err
	}()

	// Send our handshake
	if _, err := conn.Write(handshake); err != nil {
		return nil, fmt.Errorf("write handshake: %w", err)
	}

	// Wait for read to complete
	err := <-readDone
	if err != nil {
		return nil, fmt.Errorf("read handshake: %w", err)
	}

	// Verify pstr length and pstr
	if peerBuf[0] != byte(len(pstr)) {
		return nil, fmt.Errorf("invalid pstr length: got %d, expected %d", peerBuf[0], len(pstr))
	}
	if string(peerBuf[1:1+len(pstr)]) != pstr {
		return nil, fmt.Errorf("invalid pstr: got %q, expected %q", string(peerBuf[1:1+len(pstr)]), pstr)
	}

	// Extract peer's info hash
	peerInfoHash := [20]byte{}
	copy(peerInfoHash[:], peerBuf[1+len(pstr)+8:1+len(pstr)+8+20])

	// Verify info hash matches
	if peerInfoHash != infoHash {
		return nil, fmt.Errorf("info hash mismatch: got %v, expected %v", peerInfoHash, infoHash)
	}

	// Extract peer's peer ID
	peerPeerID := [20]byte{}
	copy(peerPeerID[:], peerBuf[1+len(pstr)+8+20:])

	return &Conn{
		conn:     conn,
		InfoHash: peerInfoHash,
		PeerID:   peerPeerID,
		Choked:   true,
		Bitfield: []byte{},
	}, nil
}

// ReadMessage reads a message from the peer.
// Format: <length=4 bytes big-endian><id=1 byte><payload>
// Keep-alive is 4 zero bytes (length=0)
func (c *Conn) ReadMessage() (*Message, error) {
	lengthBuf := make([]byte, 4)
	_, err := io.ReadFull(c.conn, lengthBuf)
	if err != nil {
		return nil, fmt.Errorf("read message length: %w", err)
	}

	length := binary.BigEndian.Uint32(lengthBuf)

	// Keep-alive: length=0
	if length == 0 {
		return nil, nil
	}

	// Read ID and payload
	buf := make([]byte, length)
	_, err = io.ReadFull(c.conn, buf)
	if err != nil {
		return nil, fmt.Errorf("read message data: %w", err)
	}

	msg := &Message{
		ID:      buf[0],
		Payload: buf[1:],
	}

	return msg, nil
}

// SendMessage sends a message to the peer.
func (c *Conn) SendMessage(msg *Message) error {
	// Build the message: <length><id><payload>
	length := uint32(1 + len(msg.Payload))
	buf := new(bytes.Buffer)

	if err := binary.Write(buf, binary.BigEndian, length); err != nil {
		return fmt.Errorf("write length: %w", err)
	}
	if err := binary.Write(buf, binary.BigEndian, msg.ID); err != nil {
		return fmt.Errorf("write id: %w", err)
	}
	if _, err := buf.Write(msg.Payload); err != nil {
		return fmt.Errorf("write payload: %w", err)
	}

	_, err := c.conn.Write(buf.Bytes())
	return err
}

// SendKeepAlive sends a keep-alive message (4 zero bytes).
func (c *Conn) SendKeepAlive() error {
	_, err := c.conn.Write([]byte{0, 0, 0, 0})
	return err
}

// SendInterested sends an interested message.
func (c *Conn) SendInterested() error {
	return c.SendMessage(&Message{ID: MsgInterested})
}

// SendNotInterested sends a not interested message.
func (c *Conn) SendNotInterested() error {
	return c.SendMessage(&Message{ID: MsgNotInterested})
}

// SendRequest sends a request message.
// Payload: index(4) + begin(4) + length(4)
func (c *Conn) SendRequest(index, begin, length uint32) error {
	payload := new(bytes.Buffer)
	binary.Write(payload, binary.BigEndian, index)
	binary.Write(payload, binary.BigEndian, begin)
	binary.Write(payload, binary.BigEndian, length)

	return c.SendMessage(&Message{
		ID:      MsgRequest,
		Payload: payload.Bytes(),
	})
}

// SendHave sends a have message.
// Payload: index(4)
func (c *Conn) SendHave(index uint32) error {
	payload := new(bytes.Buffer)
	binary.Write(payload, binary.BigEndian, index)

	return c.SendMessage(&Message{
		ID:      MsgHave,
		Payload: payload.Bytes(),
	})
}

// SendChoke sends a choke message.
func (c *Conn) SendChoke() error {
	return c.SendMessage(&Message{ID: MsgChoke})
}

// SendUnchoke sends an unchoke message.
func (c *Conn) SendUnchoke() error {
	return c.SendMessage(&Message{ID: MsgUnchoke})
}

// SendBitfield sends a bitfield message.
// Payload: raw bitfield bytes
func (c *Conn) SendBitfield(bitfield []byte) error {
	return c.SendMessage(&Message{
		ID:      MsgBitfield,
		Payload: bitfield,
	})
}

// ParsePiece parses a piece message.
// Payload: index(4) + begin(4) + block(variable)
func ParsePiece(msg *Message) (index, begin uint32, data []byte, err error) {
	if msg.ID != MsgPiece {
		err = fmt.Errorf("expected piece message (ID 7), got ID %d", msg.ID)
		return
	}

	if len(msg.Payload) < 8 {
		err = fmt.Errorf("piece payload too short: %d bytes", len(msg.Payload))
		return
	}

	index = binary.BigEndian.Uint32(msg.Payload[0:4])
	begin = binary.BigEndian.Uint32(msg.Payload[4:8])
	data = msg.Payload[8:]

	return
}

// ParseHave parses a have message.
// Payload: index(4)
func ParseHave(msg *Message) (uint32, error) {
	if msg.ID != MsgHave {
		return 0, fmt.Errorf("expected have message (ID 4), got ID %d", msg.ID)
	}

	if len(msg.Payload) != 4 {
		return 0, fmt.Errorf("have payload must be 4 bytes, got %d", len(msg.Payload))
	}

	return binary.BigEndian.Uint32(msg.Payload), nil
}

// ParseRequest parses a request message.
// Payload: index(4) + begin(4) + length(4)
func ParseRequest(msg *Message) (index, begin, length uint32, err error) {
	if msg.ID != MsgRequest {
		err = fmt.Errorf("expected request message (ID 6), got ID %d", msg.ID)
		return
	}

	if len(msg.Payload) != 12 {
		err = fmt.Errorf("request payload must be 12 bytes, got %d", len(msg.Payload))
		return
	}

	index = binary.BigEndian.Uint32(msg.Payload[0:4])
	begin = binary.BigEndian.Uint32(msg.Payload[4:8])
	length = binary.BigEndian.Uint32(msg.Payload[8:12])

	return
}

// HasPiece checks if a piece is in the bitfield.
func (c *Conn) HasPiece(index int) bool {
	byteIndex := index / 8
	bitIndex := 7 - (index % 8)

	if byteIndex >= len(c.Bitfield) {
		return false
	}

	return (c.Bitfield[byteIndex] & (1 << uint(bitIndex))) != 0
}

// SetPiece marks a piece as present in the bitfield.
func (c *Conn) SetPiece(index int) {
	byteIndex := index / 8
	bitIndex := 7 - (index % 8)

	if byteIndex >= len(c.Bitfield) {
		// Extend bitfield if necessary
		newBitfield := make([]byte, byteIndex+1)
		copy(newBitfield, c.Bitfield)
		c.Bitfield = newBitfield
	}

	c.Bitfield[byteIndex] |= (1 << uint(bitIndex))
}

// Close closes the connection to the peer.
func (c *Conn) Close() error {
	return c.conn.Close()
}
