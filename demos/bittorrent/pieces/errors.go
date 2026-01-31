package pieces

import "errors"

var (
	ErrInvalidPieceIndex = errors.New("invalid piece index")
	ErrBlockOutOfRange   = errors.New("block is out of range for piece")
)
