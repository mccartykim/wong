# Wong jj-Native Architecture Design

## Overview

Wong stores issue-tracking data directly in jj's change graph, eliminating the
need for git hooks, daemons, or background processes. The architecture uses a
single dedicated jj change ("wong-db") as a parallel branch off `root()` to hold
per-issue JSON files, with lazy sync on wong command invocation.

**Design goal:** Zero commands to remember beyond `wong` and `jj`.

## User Workflow

```
# One-time setup
cd my-jj-repo
wong init                     # creates wong-db change, bookmark, config

# Daily workflow -- just use jj and wong normally
jj new main -m "my feature"
wong create "fix the widget"  # creates .wong/issues/wong-abc.json, syncs to wong-db
wong list                     # reads from wong-db (syncs first)
jj commit -m "done"
wong close wong-abc           # updates issue, syncs to wong-db
```

No wrappers. No daemons. No aliases. No `wong jj` prefix.

## Architecture Decisions

All decisions validated empirically with jj 0.37.0.

### 1. Topology: Parallel branch off root()

```
@  (working copy - merge child of main + wong-db)
├─╮
│ ◆  wong-db (immutable)     ← .wong/issues/*.json
○ │  main tip
├─╯
◆  root()
```

**Why:** Complete file isolation. `.wong/` files never appear in main's tree,
working copy filesystem, or `jj file show -r @`. Ancestor topology was rejected
(files leak to all descendants). True orphans don't exist in jj; root() children
are the closest equivalent.

**Access:** `jj file show -r wong-db .wong/issues/ID.json` reads any issue
without switching working copy.

### 2. Storage: Per-issue files, not single JSONL

```
.wong/
  issues/
    wong-abc.json     # one file per issue
    wong-def.json
    wong-8rr.json
  config.yaml         # repo config
  metadata.json       # backend version
```

**Why:** Single-file JSONL **always conflicts** when two agents append
concurrently (both appending at EOF is ambiguous to 3-way merge). Per-issue
files make concurrent new-issue creation conflict-free. The only conflict
scenario is two agents editing the *same issue* simultaneously, which is a
genuine semantic conflict.

### 3. Sync: Lazy on wong commands, atomic single-command

```go
// Called at the start of every wong read/write command
func syncWongDB() error {
    return exec.Command("jj",
        "squash", "--into", "wong-db", ".wong/", "-u",
        "--config", `revset-aliases."immutable_heads()"="none()"`,
    ).Run()
}
```

**Why:**
- **Lazy sync** on wong command invocation means zero overhead on normal `jj`
  commands. No wrapper, no daemon, no shell alias.
- **Atomic `--config` override** temporarily lifts immutability for that single
  invocation only. On-disk config is never modified. No crash window. Concurrent
  jj processes still see wong-db as immutable.
- **Idempotent**: running with no `.wong/` changes succeeds harmlessly.
- **~108ms** sync time is imperceptible during wong command startup.

### 4. Protection: immutable_heads() as firmware guard

```toml
# .jj/repo/config.toml (set by wong init)
[revset-aliases]
"immutable_heads()" = "wong-db"
```

**Why:** Prevents accidental `jj edit wong-db`, `jj describe wong-db`, or
`jj squash --into wong-db` by users. All mutating operations blocked with clear
error. Read-only operations (log, diff, file show, new) unaffected.

Visual indicator: jj renders immutable commits with `◆` diamond glyph.

Wong CLI bypasses this atomically via per-command `--config` override.

### 5. Merge working copy for repeatable sync

```
# wong init creates working copy as merge child of main + wong-db
jj new <main-tip> <wong-db> -m "working change"
```

**Why:** `jj squash --into wong-db '.wong/'` only works **repeatably** when the
working copy has wong-db as a parent. Without merge parent, the second squash
computes the diff against the wrong baseline and produces conflicts. With merge
parent, the diff for `.wong/` is relative to wong-db's tree, so only the delta
since last squash is included.

### 6. History: Squash-in-place with op log (default)

**Default:** Squash updates into wong-db (single change, clean DAG). History
lives in `jj op log`, queryable via:
```
jj --at-op <op-id> file show -r wong-db .wong/issues/ID.json
```

**Opt-in chain mode:** For teams needing durable audit trails that survive clone:
```
wong config set db.history chain
```
Builds `wong-db → snap1 → snap2 → ...`, visible in DAG via
`jj log -r 'descendants(wong-db)'`.

### 7. No decorator, no daemon (primary path)

**Lazy sync** eliminates the need for either:
- **Decorator rejected as primary:** Requires `wong jj <cmd>` wrapper, breaks
  muscle memory, breaks tab completion.
- **Daemon rejected:** Extra process, platform-specific inotify/fsevents,
  battery drain, deployment complexity.
- **Decorator available as opt-in** for users wanting real-time consistency.

## wong init Flow

```
wong init
```

1. Detect jj repo (`.jj/` exists)
2. Create wong-db change: `jj new root() -m "wong-db: issue tracker storage"`
3. Create `.wong/` directory structure with metadata
4. Snapshot: `jj commit -m "wong-db: initial setup"`
5. Create bookmark: `jj bookmark create wong-db`
6. Set immutability: `jj config set --repo 'revset-aliases."immutable_heads()"' 'wong-db'`
7. Return to user's previous working copy
8. Re-create working copy as merge: `jj new <previous-wc> <wong-db>`

## Performance Budget

| Operation | Time | Notes |
|-----------|------|-------|
| jj status (baseline) | ~94-153ms | Unaffected by wong |
| jj log (baseline) | ~121ms | Unaffected by wong |
| wong sync (no changes) | ~78ms | No-op squash |
| wong sync (with data) | ~108-182ms | Atomic squash |
| wong list (total) | ~200-300ms | Sync + read + display |

## Migration from Beads

```
wong migrate export    # reads beads.db → .wong/issues/<ID>.json
wong init              # creates wong-db, squashes .wong/ into it
wong list              # verify parity
```

Coexistence: `.beads/` and `.wong/` can coexist. Beads uses git hooks;
wong uses wong-db change. One-way export only (beads → wong).

## Commands Summary

| Command | What it does |
|---------|-------------|
| `wong init` | Set up wong-db change, bookmark, immutability, merge WC |
| `wong list` | Sync + list open issues |
| `wong show ID` | Sync + show issue details |
| `wong create "title"` | Create .wong/issues/ID.json, sync to wong-db |
| `wong close ID` | Update issue status, sync to wong-db |
| `wong comment ID "text"` | Add comment, sync to wong-db |
| `wong sync` | Explicit manual sync (usually unnecessary) |
| `wong migrate export` | One-time beads → wong migration |
