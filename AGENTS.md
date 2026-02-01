# Wong: jj-first issue tracker (fork of beads)

## What is this

Wong is a jj-native issue tracker built on top of [beads](https://github.com/steveyegge/beads).
Issues are stored as JSON files in a parallel jj change off `root()`, tracked by the `wong-db` bookmark.
Multiple agents can work in parallel jj workspaces, each reading/writing issues atomically via flock-based locking.

## Project layout

```
wong_impl/                    # Go source (module: github.com/steveyegge/beads)
  internal/wongdb/            # Core wong-db package (the main code)
    wongdb.go                 # WongDB struct, Sync, WriteIssue, runJJ
    storage.go                # LoadAllIssues, IsReady, ReadyIssues
    decorator.go              # JJ operation decorator (hooks)
    aliases.go                # JJ revset alias helpers
    *_test.go                 # Unit + E2E tests (run with -race!)
    test_shell_injection.sh   # Integration test for shell injection fix
  internal/vcs/               # VCS abstraction layer (git/jj)
  wong-helpers.sh             # Shell helpers: wong-read/write/close/subtask/list/ready/dispatch/status
wongs/                        # Issue tracker (dogfooding wong!)
  wong-q*.json                # Quality/architecture issues
  wong-epic-*.json            # Future epics
demos/
  raytracer-agent/            # 5-agent ray tracer built via wong workflow
  bittorrent-agent/           # 6-agent bittorrent client built via wong workflow
  bittorrent/                 # Earlier bittorrent version
```

## Build and test

```bash
# Run all wongdb tests (the main test suite)
cd wong_impl && GOTOOLCHAIN=local go test -race ./internal/wongdb/ -timeout 3m

# Run shell injection integration tests
bash wong_impl/internal/wongdb/test_shell_injection.sh

# Run VCS abstraction tests
cd wong_impl && GOTOOLCHAIN=local go test ./internal/vcs/

# Build raytracer demo
cd demos/raytracer-agent && GOTOOLCHAIN=local go test ./... && GOTOOLCHAIN=local go build -o raytracer . && ./raytracer > output.ppm

# Build bittorrent demo
cd demos/bittorrent-agent && GOTOOLCHAIN=local go test ./...
```

**Important**: Always use `GOTOOLCHAIN=local` - there's no network access for toolchain downloads.

## Open issues (check wongs/ for current state)

Priority order for remaining work:

| ID | P | Status | Summary |
|----|---|--------|---------|
| q1 | 0 | closed | dirtyFiles race - fixed with sync.Mutex |
| q2 | 0 | closed | shell injection - fixed with os.environ |
| q3 | 1 | open   | Fragile jj error string matching in wongdb.go |
| q5 | 1 | open   | IsReady can't distinguish blocked vs missing dep |
| q6 | 1 | open   | Add -race and corruption tests (unblocked, q1 done) |
| q4 | 1 | open   | Vec3 zero-div panics (demo code) |
| q7 | 1 | open   | Bittorrent download loop incomplete (demo code) |
| q9 | 2 | open   | Silent error suppression in helpers/Sync |
| q8 | 2 | blocked | Agent code sharing (needs stacked-diffs epic) |
| epic | 2 | open  | Stacked diffs workflow |

Core infrastructure issues (q3, q5, q6) are higher priority than demo fixes (q4, q7).

## Key design decisions

- **Parallel jj change off root()**: wong-db is a bookmark pointing to a change whose parent is `root()`, completely separate from the working tree. This avoids merge conflicts with user code.
- **File-based locking (flock)**: Cross-process serialization for Sync operations. Lock file at `.jj/repo/wong-sync.lock`.
- **In-process mutex**: `sync.Mutex` on WongDB protects the `dirtyFiles` map from goroutine races (flock only works across OS processes).
- **Dirty file tracking**: `WriteIssue()` saves file content in memory so it can be restored after `jj workspace update-stale` overwrites the working copy.
- **Shell helpers use env vars**: Python snippets in wong-helpers.sh read values from `os.environ` instead of string interpolation to prevent injection.
