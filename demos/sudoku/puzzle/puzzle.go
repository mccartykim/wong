// Package puzzle provides sudoku board representation, validation, and solving.
package puzzle

import (
	"errors"
	"fmt"
	"math/rand"
)

const Size = 9
const BoxSize = 3

// Board represents a 9x9 sudoku grid. Zero means empty.
type Board [Size][Size]int

// Cell represents a row/column coordinate.
type Cell struct {
	Row, Col int
}

// Difficulty levels control how many clues remain.
type Difficulty int

const (
	Easy   Difficulty = iota // ~36-40 clues
	Medium                   // ~30-35 clues
	Hard                     // ~25-29 clues
)

// ClueCount returns the target number of clues for a difficulty.
func (d Difficulty) ClueCount() int {
	switch d {
	case Easy:
		return 38
	case Medium:
		return 30
	case Hard:
		return 25
	default:
		return 30
	}
}

// String returns the difficulty name.
func (d Difficulty) String() string {
	switch d {
	case Easy:
		return "easy"
	case Medium:
		return "medium"
	case Hard:
		return "hard"
	default:
		return "unknown"
	}
}

// ParseDifficulty converts a string to a Difficulty.
func ParseDifficulty(s string) (Difficulty, error) {
	switch s {
	case "easy":
		return Easy, nil
	case "medium":
		return Medium, nil
	case "hard":
		return Hard, nil
	default:
		return Easy, fmt.Errorf("unknown difficulty: %q", s)
	}
}

var (
	ErrInvalidValue = errors.New("value must be 1-9")
	ErrConflict     = errors.New("value conflicts with row, column, or box")
	ErrNoSolution   = errors.New("puzzle has no solution")
	ErrNotUnique    = errors.New("puzzle does not have a unique solution")
)

// IsValid checks if placing val at (row, col) is valid.
// Does not check if cell is empty.
func (b *Board) IsValid(row, col, val int) bool {
	if val < 1 || val > 9 {
		return false
	}
	// Check row
	for c := 0; c < Size; c++ {
		if c != col && b[row][c] == val {
			return false
		}
	}
	// Check column
	for r := 0; r < Size; r++ {
		if r != row && b[r][col] == val {
			return false
		}
	}
	// Check 3x3 box
	boxRow, boxCol := (row/BoxSize)*BoxSize, (col/BoxSize)*BoxSize
	for r := boxRow; r < boxRow+BoxSize; r++ {
		for c := boxCol; c < boxCol+BoxSize; c++ {
			if (r != row || c != col) && b[r][c] == val {
				return false
			}
		}
	}
	return true
}

// IsComplete returns true if the board is fully filled with valid values.
func (b *Board) IsComplete() bool {
	for r := 0; r < Size; r++ {
		for c := 0; c < Size; c++ {
			v := b[r][c]
			if v < 1 || v > 9 {
				return false
			}
		}
	}
	return b.IsConsistent()
}

// IsConsistent checks if the board has no conflicts (ignoring empty cells).
func (b *Board) IsConsistent() bool {
	for r := 0; r < Size; r++ {
		for c := 0; c < Size; c++ {
			if b[r][c] != 0 && !b.IsValid(r, c, b[r][c]) {
				return false
			}
		}
	}
	return true
}

// EmptyCells returns all cells with value 0.
func (b *Board) EmptyCells() []Cell {
	var cells []Cell
	for r := 0; r < Size; r++ {
		for c := 0; c < Size; c++ {
			if b[r][c] == 0 {
				cells = append(cells, Cell{r, c})
			}
		}
	}
	return cells
}

// ClueCount returns the number of non-empty cells.
func (b *Board) ClueCount() int {
	count := 0
	for r := 0; r < Size; r++ {
		for c := 0; c < Size; c++ {
			if b[r][c] != 0 {
				count++
			}
		}
	}
	return count
}

// Copy returns a deep copy of the board.
func (b *Board) Copy() Board {
	var copy Board
	for r := 0; r < Size; r++ {
		for c := 0; c < Size; c++ {
			copy[r][c] = b[r][c]
		}
	}
	return copy
}

// Set places a value on the board with validation.
func (b *Board) Set(row, col, val int) error {
	if val < 0 || val > 9 {
		return ErrInvalidValue
	}
	if val != 0 && !b.IsValid(row, col, val) {
		return ErrConflict
	}
	b[row][col] = val
	return nil
}

// String returns a human-readable board representation.
func (b *Board) String() string {
	var s string
	for r := 0; r < Size; r++ {
		if r > 0 && r%BoxSize == 0 {
			s += "------+-------+------\n"
		}
		for c := 0; c < Size; c++ {
			if c > 0 && c%BoxSize == 0 {
				s += " | "
			} else if c > 0 {
				s += " "
			}
			if b[r][c] == 0 {
				s += "."
			} else {
				s += fmt.Sprintf("%d", b[r][c])
			}
		}
		s += "\n"
	}
	return s
}

// Solve fills the board using backtracking. Returns true if solved.
func (b *Board) Solve() bool {
	return b.solve()
}

func (b *Board) solve() bool {
	// Find first empty cell
	for r := 0; r < Size; r++ {
		for c := 0; c < Size; c++ {
			if b[r][c] == 0 {
				for v := 1; v <= 9; v++ {
					if b.IsValid(r, c, v) {
						b[r][c] = v
						if b.solve() {
							return true
						}
						b[r][c] = 0
					}
				}
				return false
			}
		}
	}
	return true // No empty cells
}

// SolveWithLimit attempts to solve but stops after finding limit solutions.
// Returns the number of solutions found (up to limit).
func (b *Board) SolveWithLimit(limit int) int {
	count := 0
	b.countSolutions(&count, limit)
	return count
}

func (b *Board) countSolutions(count *int, limit int) {
	if *count >= limit {
		return
	}
	for r := 0; r < Size; r++ {
		for c := 0; c < Size; c++ {
			if b[r][c] == 0 {
				for v := 1; v <= 9; v++ {
					if b.IsValid(r, c, v) {
						b[r][c] = v
						b.countSolutions(count, limit)
						b[r][c] = 0
						if *count >= limit {
							return
						}
					}
				}
				return
			}
		}
	}
	*count++
}

// HasUniqueSolution returns true if the board has exactly one solution.
func (b *Board) HasUniqueSolution() bool {
	copy := b.Copy()
	return copy.SolveWithLimit(2) == 1
}

// Generate creates a new puzzle with the given difficulty.
func Generate(rng *rand.Rand, difficulty Difficulty) Board {
	// Step 1: Generate a fully solved board
	var solved Board
	fillBoard(&solved, rng)

	// Step 2: Remove cells while maintaining unique solution
	puzzle := solved.Copy()
	targetClues := difficulty.ClueCount()

	// Create shuffled list of all cells
	cells := make([]Cell, 0, Size*Size)
	for r := 0; r < Size; r++ {
		for c := 0; c < Size; c++ {
			cells = append(cells, Cell{r, c})
		}
	}
	rng.Shuffle(len(cells), func(i, j int) {
		cells[i], cells[j] = cells[j], cells[i]
	})

	for _, cell := range cells {
		if puzzle.ClueCount() <= targetClues {
			break
		}
		old := puzzle[cell.Row][cell.Col]
		if old == 0 {
			continue
		}
		puzzle[cell.Row][cell.Col] = 0
		if !puzzle.HasUniqueSolution() {
			puzzle[cell.Row][cell.Col] = old // Put it back
		}
	}

	return puzzle
}

// fillBoard fills an empty board with a valid solution using randomized backtracking.
func fillBoard(b *Board, rng *rand.Rand) bool {
	for r := 0; r < Size; r++ {
		for c := 0; c < Size; c++ {
			if b[r][c] == 0 {
				// Try values in random order
				vals := rng.Perm(9)
				for _, vi := range vals {
					v := vi + 1
					if b.IsValid(r, c, v) {
						b[r][c] = v
						if fillBoard(b, rng) {
							return true
						}
						b[r][c] = 0
					}
				}
				return false
			}
		}
	}
	return true
}
