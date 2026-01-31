package peer

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

const (
	HandshakeLength = 68 // 1 + 19 + 8 + 20 + 20
	BlockSize       = 16384
)

// Message types
const (
	MsgChoke        byte = 0
	MsgUnchoke      byte = 1
	MsgInterested   byte = 2
	MsgNotInterested byte = 3
	MsgHave         byte = 4
	MsgBitfield     byte = 5
	MsgRequest      byte = 6
	MsgPiece        byte = 7
	MsgCancel       byte = 8
)

// Message represents a peer wire protocol message.
type Message struct {
	Type byte
	Data []byte
}

// RequestMessage represents a request for a block.
type RequestMessage struct {
	Index  uint32
	Begin  uint32
	Length uint32
}

// PieceMessage represents a piece block data.
type PieceMessage struct {
	Index uint32
	Begin uint32
	Block []byte
}

// Peer represents a connection to a peer.
type Peer struct {
	IP   net.IP
	Port uint16
	conn net.Conn

	// Bitfield tracking
	Bitfield []byte
	NumPieces int

	// Connection state
	AmChoking      bool
	AmInterested   bool
	PeerChoking    bool
	PeerInterested bool

	// Channels for async message handling
	msgChan chan *Message
	errChan chan error
	closed  bool
}

// NewPeer creates a new peer connection.
func NewPeer(ip net.IP, port uint16) *Peer {
	return &Peer{
		IP:         ip,
		Port:       port,
		AmChoking:  true,
		PeerChoking: true,
		msgChan:    make(chan *Message, 10),
		errChan:    make(chan error, 1),
	}
}

// Connect establishes a TCP connection to the peer.
func (p *Peer) Connect(timeout time.Duration) error {
	addr := fmt.Sprintf("%s:%d", p.IP.String(), p.Port)
	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return fmt.Errorf("failed to connect to peer %s: %w", addr, err)
	}
	p.conn = conn
	return nil
}

// Handshake performs the BEP 3 handshake.
func (p *Peer) Handshake(infoHash [20]byte, peerID [20]byte) error {
	handshake := make([]byte, HandshakeLength)
	handshake[0] = 19
	copy(handshake[1:20], []byte("BitTorrent protocol"))
	// 8 reserved bytes (all 0)
	copy(handshake[28:48], infoHash[:])
	copy(handshake[48:68], peerID[:])

	// Send handshake
	if err := p.write(handshake); err != nil {
		return fmt.Errorf("failed to send handshake: %w", err)
	}

	// Receive handshake
	response := make([]byte, HandshakeLength)
	if err := p.readFull(response); err != nil {
		return fmt.Errorf("failed to receive handshake: %w", err)
	}

	// Verify handshake
	if response[0] != 19 || string(response[1:20]) != "BitTorrent protocol" {
		return fmt.Errorf("invalid handshake from peer")
	}

	if !bytes.Equal(response[28:48], infoHash[:]) {
		return fmt.Errorf("info_hash mismatch with peer")
	}

	// Start message reader goroutine
	go p.readMessages()

	return nil
}

// SendMessage sends a message to the peer.
func (p *Peer) SendMessage(msg *Message) error {
	return p.sendMessage(msg.Type, msg.Data)
}

// SendRequest sends a request message for a block.
func (p *Peer) SendRequest(index, begin, length uint32) error {
	data := make([]byte, 12)
	binary.BigEndian.PutUint32(data[0:4], index)
	binary.BigEndian.PutUint32(data[4:8], begin)
	binary.BigEndian.PutUint32(data[8:12], length)
	return p.sendMessage(MsgRequest, data)
}

// SendInterested sends an interested message.
func (p *Peer) SendInterested() error {
	p.AmInterested = true
	return p.sendMessage(MsgInterested, nil)
}

// SendUninterested sends a not-interested message.
func (p *Peer) SendUninterested() error {
	p.AmInterested = false
	return p.sendMessage(MsgNotInterested, nil)
}

// ReceiveMessages returns a channel for receiving messages.
func (p *Peer) ReceiveMessages() <-chan *Message {
	return p.msgChan
}

// ReceiveErrors returns a channel for receiving errors.
func (p *Peer) ReceiveErrors() <-chan error {
	return p.errChan
}

// Close closes the peer connection.
func (p *Peer) Close() error {
	if p.closed {
		return nil
	}
	p.closed = true
	close(p.msgChan)
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}

// Private helper methods

func (p *Peer) sendMessage(msgType byte, data []byte) error {
	if p.conn == nil {
		return fmt.Errorf("not connected to peer")
	}

	length := 1 + len(data)
	msg := make([]byte, 4+length)
	binary.BigEndian.PutUint32(msg[0:4], uint32(length))
	msg[4] = msgType
	if len(data) > 0 {
		copy(msg[5:], data)
	}

	return p.write(msg)
}

func (p *Peer) write(data []byte) error {
	p.conn.SetWriteDeadline(time.Now().Add(30 * time.Second))
	_, err := p.conn.Write(data)
	return err
}

func (p *Peer) readFull(buf []byte) error {
	p.conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	_, err := io.ReadFull(p.conn, buf)
	return err
}

func (p *Peer) readMessages() {
	defer func() {
		if !p.closed {
			close(p.errChan)
		}
	}()

	lenBuf := make([]byte, 4)
	for {
		// Read message length
		if err := p.readFull(lenBuf); err != nil {
			if err != io.EOF {
				p.errChan <- err
			}
			return
		}

		msgLen := binary.BigEndian.Uint32(lenBuf)
		if msgLen == 0 {
			// Keep-alive message
			continue
		}

		// Read message type and data
		msgData := make([]byte, msgLen)
		if err := p.readFull(msgData); err != nil {
			p.errChan <- err
			return
		}

		msg := &Message{
			Type: msgData[0],
			Data: msgData[1:],
		}

		// Handle message
		if err := p.handleMessage(msg); err != nil {
			p.errChan <- err
			return
		}

		select {
		case p.msgChan <- msg:
		default:
			// Channel full, skip
		}
	}
}

func (p *Peer) handleMessage(msg *Message) error {
	switch msg.Type {
	case MsgChoke:
		p.PeerChoking = true
	case MsgUnchoke:
		p.PeerChoking = false
	case MsgInterested:
		p.PeerInterested = true
	case MsgNotInterested:
		p.PeerInterested = false
	case MsgBitfield:
		p.Bitfield = msg.Data
	case MsgHave:
		if len(msg.Data) < 4 {
			return fmt.Errorf("invalid have message")
		}
		index := binary.BigEndian.Uint32(msg.Data)
		byteIdx := index / 8
		bitIdx := 7 - (index % 8)
		if int(byteIdx) < len(p.Bitfield) {
			p.Bitfield[byteIdx] |= 1 << bitIdx
		}
	}
	return nil
}

// HasPiece checks if the peer has a specific piece.
func (p *Peer) HasPiece(index uint32) bool {
	if p.Bitfield == nil {
		return false
	}
	byteIdx := index / 8
	if int(byteIdx) >= len(p.Bitfield) {
		return false
	}
	bitIdx := 7 - (index % 8)
	return p.Bitfield[byteIdx]&(1<<bitIdx) != 0
}

// SetNumPieces sets the total number of pieces for bitfield initialization.
func (p *Peer) SetNumPieces(numPieces int) {
	p.NumPieces = numPieces
	p.Bitfield = make([]byte, (numPieces+7)/8)
}
