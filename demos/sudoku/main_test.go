package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/steveyegge/wong/demos/sudoku/puzzle"
)

func TestHandleIndex(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Errorf("expected text/html, got %s", ct)
	}
	if !strings.Contains(w.Body.String(), "Wong Sudoku") {
		t.Error("expected 'Wong Sudoku' in response")
	}
}

func TestHandleIndex_NotFound(t *testing.T) {
	req := httptest.NewRequest("GET", "/nonexistent", nil)
	w := httptest.NewRecorder()
	handleIndex(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleNewPuzzle_Default(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/new", nil)
	w := httptest.NewRecorder()
	handleNewPuzzle(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp newPuzzleResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("json decode: %v", err)
	}
	if resp.Diff != "medium" {
		t.Errorf("expected medium difficulty, got %s", resp.Diff)
	}
	if resp.Clues < 20 || resp.Clues > 45 {
		t.Errorf("unexpected clue count: %d", resp.Clues)
	}
	if !resp.Board.IsConsistent() {
		t.Error("generated board is not consistent")
	}
}

func TestHandleNewPuzzle_WithDifficulty(t *testing.T) {
	for _, diff := range []string{"easy", "medium", "hard"} {
		t.Run(diff, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/api/new?difficulty="+diff, nil)
			w := httptest.NewRecorder()
			handleNewPuzzle(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d", w.Code)
			}
			var resp newPuzzleResponse
			json.Unmarshal(w.Body.Bytes(), &resp)
			if resp.Diff != diff {
				t.Errorf("expected %s, got %s", diff, resp.Diff)
			}
		})
	}
}

func TestHandleNewPuzzle_WithSeed(t *testing.T) {
	req1 := httptest.NewRequest("GET", "/api/new?seed=42&difficulty=easy", nil)
	w1 := httptest.NewRecorder()
	handleNewPuzzle(w1, req1)

	req2 := httptest.NewRequest("GET", "/api/new?seed=42&difficulty=easy", nil)
	w2 := httptest.NewRecorder()
	handleNewPuzzle(w2, req2)

	if w1.Body.String() != w2.Body.String() {
		t.Error("same seed should produce same puzzle")
	}
}

func TestHandleNewPuzzle_InvalidDifficulty(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/new?difficulty=extreme", nil)
	w := httptest.NewRecorder()
	handleNewPuzzle(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSolve(t *testing.T) {
	// Create a solvable puzzle
	var board puzzle.Board
	board[0][0] = 5
	board[0][1] = 3
	board[1][0] = 6

	body, _ := json.Marshal(solveRequest{Board: board})
	req := httptest.NewRequest("POST", "/api/solve", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleSolve(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp solveResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Solved {
		t.Error("expected puzzle to be solved")
	}
	if !resp.Board.IsComplete() {
		t.Error("expected complete board in response")
	}
}

func TestHandleSolve_Impossible(t *testing.T) {
	var board puzzle.Board
	board[0][0] = 5
	board[0][1] = 5 // conflict

	body, _ := json.Marshal(solveRequest{Board: board})
	req := httptest.NewRequest("POST", "/api/solve", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleSolve(w, req)

	var resp solveResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Solved {
		t.Error("should not solve impossible board")
	}
}

func TestHandleSolve_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/solve", nil)
	w := httptest.NewRecorder()
	handleSolve(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSolve_InvalidJSON(t *testing.T) {
	req := httptest.NewRequest("POST", "/api/solve", strings.NewReader("not json"))
	w := httptest.NewRecorder()
	handleSolve(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleValidate_Complete(t *testing.T) {
	// Build a complete board
	var board puzzle.Board
	board.Solve()

	body, _ := json.Marshal(solveRequest{Board: board})
	req := httptest.NewRequest("POST", "/api/validate", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleValidate(w, req)

	var resp validateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Valid || !resp.Complete {
		t.Error("expected valid and complete")
	}
	if !strings.Contains(resp.Message, "Congratulations") {
		t.Errorf("expected congratulations message, got %q", resp.Message)
	}
}

func TestHandleValidate_PartialValid(t *testing.T) {
	var board puzzle.Board
	board[0][0] = 5

	body, _ := json.Marshal(solveRequest{Board: board})
	req := httptest.NewRequest("POST", "/api/validate", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleValidate(w, req)

	var resp validateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp.Valid {
		t.Error("expected valid for partial board with no conflicts")
	}
	if resp.Complete {
		t.Error("expected not complete for partial board")
	}
}

func TestHandleValidate_Conflict(t *testing.T) {
	var board puzzle.Board
	board[0][0] = 5
	board[0][1] = 5

	body, _ := json.Marshal(solveRequest{Board: board})
	req := httptest.NewRequest("POST", "/api/validate", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	handleValidate(w, req)

	var resp validateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Valid {
		t.Error("expected invalid for conflicting board")
	}
}

func TestHandleValidate_MethodNotAllowed(t *testing.T) {
	req := httptest.NewRequest("GET", "/api/validate", nil)
	w := httptest.NewRecorder()
	handleValidate(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
