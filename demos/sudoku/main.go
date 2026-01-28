// Sudoku demo app - a web-based sudoku game with solve button.
// Part of the wong project (jj-first fork of beads).
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/steveyegge/wong/demos/sudoku/puzzle"
)

func main() {
	port := "8080"
	if p := os.Getenv("PORT"); p != "" {
		port = p
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/api/new", handleNewPuzzle)
	mux.HandleFunc("/api/solve", handleSolve)
	mux.HandleFunc("/api/validate", handleValidate)

	addr := ":" + port
	fmt.Printf("Sudoku server starting on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, mux))
}

type newPuzzleResponse struct {
	Board puzzle.Board `json:"board"`
	Clues int         `json:"clues"`
	Diff  string      `json:"difficulty"`
}

type solveRequest struct {
	Board puzzle.Board `json:"board"`
}

type solveResponse struct {
	Board  puzzle.Board `json:"board"`
	Solved bool        `json:"solved"`
}

type validateResponse struct {
	Valid    bool   `json:"valid"`
	Complete bool   `json:"complete"`
	Message  string `json:"message,omitempty"`
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

func handleNewPuzzle(w http.ResponseWriter, r *http.Request) {
	diffStr := r.URL.Query().Get("difficulty")
	if diffStr == "" {
		diffStr = "medium"
	}
	diff, err := puzzle.ParseDifficulty(diffStr)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	seed := time.Now().UnixNano()
	if s := r.URL.Query().Get("seed"); s != "" {
		if parsed, err := strconv.ParseInt(s, 10, 64); err == nil {
			seed = parsed
		}
	}

	rng := rand.New(rand.NewSource(seed))
	board := puzzle.Generate(rng, diff)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(newPuzzleResponse{
		Board: board,
		Clues: board.ClueCount(),
		Diff:  diff.String(),
	})
}

func handleSolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req solveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	board := req.Board
	solved := board.Solve()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(solveResponse{
		Board:  board,
		Solved: solved,
	})
}

func handleValidate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST required", http.StatusMethodNotAllowed)
		return
	}

	var req solveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	board := req.Board
	resp := validateResponse{
		Valid:    board.IsConsistent(),
		Complete: board.IsComplete(),
	}
	if resp.Complete {
		resp.Message = "Congratulations! Puzzle solved correctly!"
	} else if !resp.Valid {
		resp.Message = "There are conflicting values."
	} else {
		resp.Message = "No conflicts so far. Keep going!"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

const indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1.0">
<title>Wong Sudoku</title>
<style>
* { margin: 0; padding: 0; box-sizing: border-box; }
body {
  font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
  background: #1a1a2e;
  color: #e0e0e0;
  display: flex;
  flex-direction: column;
  align-items: center;
  min-height: 100vh;
  padding: 20px;
}
h1 {
  font-size: 2rem;
  margin-bottom: 8px;
  color: #e94560;
}
.subtitle {
  color: #888;
  margin-bottom: 20px;
  font-size: 0.9rem;
}
.controls {
  display: flex;
  gap: 10px;
  margin-bottom: 20px;
  flex-wrap: wrap;
  justify-content: center;
}
button {
  padding: 10px 20px;
  border: none;
  border-radius: 6px;
  cursor: pointer;
  font-size: 0.95rem;
  font-weight: 600;
  transition: transform 0.1s, background 0.2s;
}
button:hover { transform: translateY(-1px); }
button:active { transform: translateY(0); }
.btn-new { background: #0f3460; color: #e0e0e0; }
.btn-new:hover { background: #16498a; }
.btn-solve { background: #e94560; color: white; }
.btn-solve:hover { background: #d63851; }
.btn-validate { background: #533483; color: white; }
.btn-validate:hover { background: #6a42a8; }
.btn-clear { background: #444; color: #e0e0e0; }
.btn-clear:hover { background: #555; }
select {
  padding: 10px 16px;
  border: 1px solid #333;
  border-radius: 6px;
  background: #16213e;
  color: #e0e0e0;
  font-size: 0.95rem;
}
#board {
  display: grid;
  grid-template-columns: repeat(9, 1fr);
  gap: 1px;
  background: #444;
  border: 3px solid #e94560;
  border-radius: 4px;
  width: min(480px, 95vw);
  height: min(480px, 95vw);
}
.cell {
  background: #16213e;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: min(24px, 5vw);
  font-weight: 700;
  cursor: pointer;
  position: relative;
  user-select: none;
}
.cell.given {
  color: #e0e0e0;
  cursor: default;
}
.cell.user {
  color: #4fc3f7;
}
.cell.conflict {
  background: #5c1a1a;
  color: #ff6b6b;
}
.cell.selected {
  background: #1a3a5c;
}
.cell.highlight {
  background: #1a2a3e;
}
.cell input {
  width: 100%;
  height: 100%;
  border: none;
  background: transparent;
  color: inherit;
  text-align: center;
  font-size: inherit;
  font-weight: inherit;
  outline: none;
  caret-color: #e94560;
}
/* Box borders */
.cell:nth-child(3n) { border-right: 2px solid #e94560; }
.cell:nth-child(9n) { border-right: none; }
.cell:nth-child(n+19):nth-child(-n+27),
.cell:nth-child(n+46):nth-child(-n+54) {
  border-bottom: 2px solid #e94560;
}
#message {
  margin-top: 16px;
  padding: 10px 20px;
  border-radius: 6px;
  font-weight: 600;
  min-height: 40px;
  text-align: center;
}
.msg-success { background: #1a3a1a; color: #4caf50; }
.msg-error { background: #3a1a1a; color: #ff6b6b; }
.msg-info { background: #1a2a3a; color: #4fc3f7; }
.info {
  margin-top: 16px;
  color: #666;
  font-size: 0.8rem;
  text-align: center;
}
</style>
</head>
<body>
<h1>Wong Sudoku</h1>
<p class="subtitle">A demo project for the wong VCS toolkit</p>
<div class="controls">
  <select id="difficulty">
    <option value="easy">Easy</option>
    <option value="medium" selected>Medium</option>
    <option value="hard">Hard</option>
  </select>
  <button class="btn-new" onclick="newGame()">New Game</button>
  <button class="btn-solve" onclick="solveBoard()">Solve</button>
  <button class="btn-validate" onclick="validateBoard()">Check</button>
  <button class="btn-clear" onclick="clearUserInput()">Clear</button>
</div>
<div id="board"></div>
<div id="message"></div>
<p class="info">Use arrow keys to navigate. Type 1-9 to fill, Backspace/Delete to clear.</p>

<script>
let board = Array(9).fill(null).map(() => Array(9).fill(0));
let given = Array(9).fill(null).map(() => Array(9).fill(false));
let selectedCell = null;

function createBoard() {
  const el = document.getElementById('board');
  el.innerHTML = '';
  for (let r = 0; r < 9; r++) {
    for (let c = 0; c < 9; c++) {
      const cell = document.createElement('div');
      cell.className = 'cell';
      cell.dataset.row = r;
      cell.dataset.col = c;
      cell.addEventListener('click', () => selectCell(r, c));
      el.appendChild(cell);
    }
  }
}

function renderBoard() {
  const cells = document.querySelectorAll('.cell');
  cells.forEach(cell => {
    const r = parseInt(cell.dataset.row);
    const c = parseInt(cell.dataset.col);
    const val = board[r][c];
    cell.textContent = val > 0 ? val : '';
    cell.className = 'cell';
    if (given[r][c]) cell.classList.add('given');
    else if (val > 0) cell.classList.add('user');
    if (selectedCell && selectedCell[0] === r && selectedCell[1] === c) {
      cell.classList.add('selected');
    }
  });
}

function selectCell(r, c) {
  if (given[r][c]) return;
  selectedCell = [r, c];
  renderBoard();
}

function setMessage(text, type) {
  const el = document.getElementById('message');
  el.textContent = text;
  el.className = 'msg-' + type;
}

async function newGame() {
  const diff = document.getElementById('difficulty').value;
  setMessage('Generating...', 'info');
  try {
    const res = await fetch('/api/new?difficulty=' + diff);
    const data = await res.json();
    board = data.board;
    given = board.map(row => row.map(v => v > 0));
    selectedCell = null;
    renderBoard();
    setMessage(data.diff + ' puzzle (' + data.clues + ' clues)', 'info');
  } catch (e) {
    setMessage('Error generating puzzle: ' + e.message, 'error');
  }
}

async function solveBoard() {
  setMessage('Solving...', 'info');
  try {
    // Send only the given clues
    const clueBoard = board.map((row, r) => row.map((v, c) => given[r][c] ? v : 0));
    const res = await fetch('/api/solve', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({board: clueBoard})
    });
    const data = await res.json();
    if (data.solved) {
      board = data.board;
      renderBoard();
      setMessage('Solved!', 'success');
    } else {
      setMessage('No solution exists for this configuration.', 'error');
    }
  } catch (e) {
    setMessage('Error: ' + e.message, 'error');
  }
}

async function validateBoard() {
  try {
    const res = await fetch('/api/validate', {
      method: 'POST',
      headers: {'Content-Type': 'application/json'},
      body: JSON.stringify({board: board})
    });
    const data = await res.json();
    if (data.complete) {
      setMessage(data.message, 'success');
    } else if (!data.valid) {
      setMessage(data.message, 'error');
    } else {
      setMessage(data.message, 'info');
    }
  } catch (e) {
    setMessage('Error: ' + e.message, 'error');
  }
}

function clearUserInput() {
  for (let r = 0; r < 9; r++) {
    for (let c = 0; c < 9; c++) {
      if (!given[r][c]) board[r][c] = 0;
    }
  }
  renderBoard();
  setMessage('User input cleared.', 'info');
}

document.addEventListener('keydown', (e) => {
  if (!selectedCell) return;
  const [r, c] = selectedCell;

  if (e.key >= '1' && e.key <= '9') {
    board[r][c] = parseInt(e.key);
    renderBoard();
  } else if (e.key === 'Backspace' || e.key === 'Delete' || e.key === '0') {
    board[r][c] = 0;
    renderBoard();
  } else if (e.key === 'ArrowUp' && r > 0) {
    selectNextCell(r - 1, c, -1, 0);
  } else if (e.key === 'ArrowDown' && r < 8) {
    selectNextCell(r + 1, c, 1, 0);
  } else if (e.key === 'ArrowLeft' && c > 0) {
    selectNextCell(r, c - 1, 0, -1);
  } else if (e.key === 'ArrowRight' && c < 8) {
    selectNextCell(r, c + 1, 0, 1);
  }
});

function selectNextCell(r, c, dr, dc) {
  // Skip given cells
  while (r >= 0 && r < 9 && c >= 0 && c < 9) {
    if (!given[r][c]) {
      selectCell(r, c);
      return;
    }
    r += dr; c += dc;
  }
}

createBoard();
newGame();
</script>
</body>
</html>`
