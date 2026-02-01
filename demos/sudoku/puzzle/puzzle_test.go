package puzzle

import (
	"math/rand"
	"testing"
)

func TestBoard_IsValid(t *testing.T) {
	var b Board
	b[0][0] = 5

	// Same row conflict
	if b.IsValid(0, 5, 5) {
		t.Error("expected conflict in same row")
	}

	// Same col conflict
	if b.IsValid(5, 0, 5) {
		t.Error("expected conflict in same column")
	}

	// Same box conflict
	if b.IsValid(1, 1, 5) {
		t.Error("expected conflict in same box")
	}

	// Valid placement
	if !b.IsValid(3, 3, 5) {
		t.Error("expected valid placement at (3,3)")
	}

	// Out of range
	if b.IsValid(0, 1, 0) {
		t.Error("expected invalid for value 0")
	}
	if b.IsValid(0, 1, 10) {
		t.Error("expected invalid for value 10")
	}
}

func TestBoard_IsConsistent(t *testing.T) {
	var b Board
	if !b.IsConsistent() {
		t.Error("empty board should be consistent")
	}

	b[0][0] = 1
	b[0][1] = 2
	if !b.IsConsistent() {
		t.Error("no conflicts, should be consistent")
	}

	b[0][2] = 1 // conflict with [0][0]
	if b.IsConsistent() {
		t.Error("expected inconsistent with row conflict")
	}
}

func TestBoard_IsComplete(t *testing.T) {
	var b Board
	if b.IsComplete() {
		t.Error("empty board should not be complete")
	}

	// Fill with a solved puzzle
	rng := rand.New(rand.NewSource(42))
	fillBoard(&b, rng)
	if !b.IsComplete() {
		t.Error("solved board should be complete")
	}
}

func TestBoard_EmptyCells(t *testing.T) {
	var b Board
	empties := b.EmptyCells()
	if len(empties) != 81 {
		t.Errorf("expected 81 empty cells, got %d", len(empties))
	}

	b[0][0] = 1
	empties = b.EmptyCells()
	if len(empties) != 80 {
		t.Errorf("expected 80 empty cells, got %d", len(empties))
	}
}

func TestBoard_ClueCount(t *testing.T) {
	var b Board
	if b.ClueCount() != 0 {
		t.Errorf("expected 0 clues, got %d", b.ClueCount())
	}

	b[0][0] = 1
	b[5][5] = 9
	if b.ClueCount() != 2 {
		t.Errorf("expected 2 clues, got %d", b.ClueCount())
	}
}

func TestBoard_Copy(t *testing.T) {
	var b Board
	b[0][0] = 5
	c := b.Copy()
	c[0][0] = 9

	if b[0][0] != 5 {
		t.Error("copy should not affect original")
	}
	if c[0][0] != 9 {
		t.Error("copy value incorrect")
	}
}

func TestBoard_Set(t *testing.T) {
	var b Board

	if err := b.Set(0, 0, 5); err != nil {
		t.Fatalf("Set(0,0,5): %v", err)
	}
	if b[0][0] != 5 {
		t.Error("expected 5 at (0,0)")
	}

	// Conflict
	if err := b.Set(0, 1, 5); err != ErrConflict {
		t.Errorf("expected ErrConflict, got %v", err)
	}

	// Invalid value
	if err := b.Set(0, 1, 10); err != ErrInvalidValue {
		t.Errorf("expected ErrInvalidValue, got %v", err)
	}

	// Clear cell
	if err := b.Set(0, 0, 0); err != nil {
		t.Fatalf("Set(0,0,0): %v", err)
	}
	if b[0][0] != 0 {
		t.Error("expected 0 at (0,0) after clearing")
	}
}

func TestBoard_Solve_Empty(t *testing.T) {
	var b Board
	if !b.Solve() {
		t.Fatal("should solve empty board")
	}
	if !b.IsComplete() {
		t.Error("solved board should be complete")
	}
}

func TestBoard_Solve_Partial(t *testing.T) {
	var b Board
	// Set up a partial puzzle
	b[0][0] = 5
	b[0][1] = 3
	b[1][0] = 6
	b[2][1] = 9
	b[2][2] = 8

	if !b.Solve() {
		t.Fatal("should solve partial board")
	}
	if !b.IsComplete() {
		t.Error("solved board should be complete")
	}
}

func TestBoard_Solve_Impossible(t *testing.T) {
	var b Board
	// Two 5s in first row - unsolvable
	b[0][0] = 5
	b[0][1] = 5

	if b.Solve() {
		t.Error("should not solve impossible board")
	}
}

func TestBoard_HasUniqueSolution(t *testing.T) {
	rng := rand.New(rand.NewSource(42))
	puzzle := Generate(rng, Easy)
	if !puzzle.HasUniqueSolution() {
		t.Error("generated puzzle should have unique solution")
	}
}

func TestBoard_SolveWithLimit(t *testing.T) {
	var b Board
	// Empty board has many solutions
	count := b.SolveWithLimit(3)
	if count < 2 {
		t.Errorf("empty board should have multiple solutions, got %d", count)
	}
}

func TestBoard_String(t *testing.T) {
	var b Board
	b[0][0] = 5
	s := b.String()
	if len(s) == 0 {
		t.Error("expected non-empty string")
	}
	if s[0] != '5' {
		t.Errorf("expected '5' at start, got %c", s[0])
	}
}

func TestGenerate_Easy(t *testing.T) {
	rng := rand.New(rand.NewSource(123))
	puzzle := Generate(rng, Easy)

	clues := puzzle.ClueCount()
	if clues < 25 || clues > 45 {
		t.Errorf("easy puzzle should have ~38 clues, got %d", clues)
	}
	if !puzzle.IsConsistent() {
		t.Error("generated puzzle should be consistent")
	}
	if !puzzle.HasUniqueSolution() {
		t.Error("generated puzzle should have unique solution")
	}
}

func TestGenerate_Medium(t *testing.T) {
	rng := rand.New(rand.NewSource(456))
	puzzle := Generate(rng, Medium)

	clues := puzzle.ClueCount()
	if clues < 20 || clues > 40 {
		t.Errorf("medium puzzle should have ~30 clues, got %d", clues)
	}
	if !puzzle.HasUniqueSolution() {
		t.Error("generated puzzle should have unique solution")
	}
}

func TestGenerate_Hard(t *testing.T) {
	rng := rand.New(rand.NewSource(789))
	puzzle := Generate(rng, Hard)

	clues := puzzle.ClueCount()
	if clues < 17 || clues > 35 {
		t.Errorf("hard puzzle should have ~25 clues, got %d", clues)
	}
	if !puzzle.HasUniqueSolution() {
		t.Error("generated puzzle should have unique solution")
	}
}

func TestGenerate_Deterministic(t *testing.T) {
	p1 := Generate(rand.New(rand.NewSource(42)), Easy)
	p2 := Generate(rand.New(rand.NewSource(42)), Easy)

	if p1 != p2 {
		t.Error("same seed should produce same puzzle")
	}
}

func TestGenerate_DifferentSeeds(t *testing.T) {
	p1 := Generate(rand.New(rand.NewSource(1)), Easy)
	p2 := Generate(rand.New(rand.NewSource(2)), Easy)

	if p1 == p2 {
		t.Error("different seeds should produce different puzzles")
	}
}

func TestDifficulty_String(t *testing.T) {
	tests := []struct {
		d    Difficulty
		want string
	}{
		{Easy, "easy"},
		{Medium, "medium"},
		{Hard, "hard"},
		{Difficulty(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.d.String(); got != tt.want {
			t.Errorf("Difficulty(%d).String() = %q, want %q", tt.d, got, tt.want)
		}
	}
}

func TestParseDifficulty(t *testing.T) {
	d, err := ParseDifficulty("easy")
	if err != nil || d != Easy {
		t.Errorf("ParseDifficulty(easy) = %v, %v", d, err)
	}

	_, err = ParseDifficulty("impossible")
	if err == nil {
		t.Error("expected error for unknown difficulty")
	}
}

func TestDifficulty_ClueCount(t *testing.T) {
	if Easy.ClueCount() <= Hard.ClueCount() {
		t.Error("easy should have more clues than hard")
	}
}

func BenchmarkSolve_Empty(b *testing.B) {
	for i := 0; i < b.N; i++ {
		var board Board
		board.Solve()
	}
}

func BenchmarkSolve_Generated(b *testing.B) {
	rng := rand.New(rand.NewSource(42))
	puzzle := Generate(rng, Hard)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy := puzzle.Copy()
		copy.Solve()
	}
}

func BenchmarkGenerate_Easy(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rng := rand.New(rand.NewSource(int64(i)))
		Generate(rng, Easy)
	}
}

func BenchmarkGenerate_Hard(b *testing.B) {
	for i := 0; i < b.N; i++ {
		rng := rand.New(rand.NewSource(int64(i)))
		Generate(rng, Hard)
	}
}
