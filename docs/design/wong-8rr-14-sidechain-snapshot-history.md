# wong-8rr.14: Sidechain Snapshot History Research

## Problem

wong stores issues on a `wong-db` change — a parallel branch off `root()` in jj.
Updates are squashed into wong-db, meaning the change is amended in place.
The question: should we preserve snapshot history as a first-class feature,
rather than squashing everything away?

## Experimental Setup

All experiments ran in `/tmp/wong-sidechain*` with jj 0.37.0.
The wong-db change lives off `root()` and contains `.wong/issues.jsonl`.

---

## Approach A: Squash-in-Place (Current Plan)

**Mechanism:** Repeatedly amend the single `wong-db` change via `jj describe`
and file modifications. The change ID stays the same; the commit ID changes
with each update.

```
@  ykxrxuun  wong-db fb20e7cf    ← always the same change, new commit hash
│  wong-db [updated ...] issues=3
◆  zzzzzzzz root() 00000000
```

**History access:** Via `jj op log` + `jj --at-op <id> file show -r wong-db <path>`.

**Verified:** Each past state of wong-db is recoverable through the operation log.
After 50 squash-in-place updates, the op log contained 306 lines (102 operations:
50 describe + 50 snapshot + 2 setup).

**Pruning:** `jj op abandon ..<op-id>` + `jj util gc --expire=now` reduced the
.jj directory from 528K to 370K (30% reduction) while discarding old operation
history. This means history is **opt-out** — you can prune it, but it accumulates
by default.

### Pros
- Single change in `jj log` — clean graph, no clutter
- Wong-db bookmark always points to the one change (no moving needed)
- Git backend stores fewer commit objects (just the latest tree)
- History "just works" via op log without any extra mechanism
- Simplest implementation in wong: just amend and describe

### Cons
- History lives in op log, not in the commit DAG — invisible to `jj log`
- `jj op abandon` + `jj util gc` can **destroy** history (non-recoverable)
- Op log is local-only; not transferred on clone/fetch
- Querying past states requires knowing operation IDs (less ergonomic)
- No way to `jj diff --from <snapshot-N> --to <snapshot-M>` without op log gymnastics

---

## Approach B: Explicit Change Chain

**Mechanism:** Instead of squashing, `jj commit` each update, building a chain:

```
@  uvntpukq  wong-db ae5e6ddd   (empty, working copy)
│
○  qxkzxlqx  68721d1f           wong-db snapshot 3: ISSUE-3 added
│
○  vkxtltky  e7d4104f           wong-db snapshot 2: ISSUE-2 added
│
○  oxttxlso  2b9e9e56           wong-db snapshot 1: ISSUE-1 added
│
◆  zzzzzzzz  root() 00000000
```

**History access:** Via `jj log -r 'descendants(<first-wong-db-change>)'` — visible
directly in the DAG.

**Verified:** Each snapshot is independently queryable:
- `jj file show -r <change-id> .wong/issues.jsonl` returns exact state at that snapshot
- `jj diff --from <snap-N> --to <snap-M>` shows incremental changes between any two snapshots
- Snapshot 1 had 2 lines, snapshot 16 had 16 lines — all correct

### Pros
- History is **in the DAG** — visible with `jj log`, survives clone/fetch
- Standard `jj diff --from X --to Y` works between any two snapshots
- No dependency on op log (which is local and prunable)
- Each snapshot has its own commit message — self-documenting audit trail
- Cannot be accidentally destroyed by `jj op abandon` or gc

### Cons
- Clutters `jj log` with N extra changes on the wong-db sidechain
- Wong-db bookmark must be moved to the tip on every update
- More git objects: 50 updates produced 591K vs 528K (12% larger pre-gc)
- Implementation is more complex: need to track tip, commit instead of amend
- Every snapshot stores a full copy of the tree (no delta compression at jj level;
  git packfiles may compress, but jj sees N separate commits)

---

## Approach C: Describe Annotations

**Mechanism:** Use `jj describe` to embed metadata (timestamps, counts) in the
wong-db change description.

```
@  ykxrxuun  wong-db fb20e7cf
│  wong-db [updated 2026-01-29T12:06:30Z] issues=3 open=2 closed=1
◆  zzzzzzzz root()
```

**Assessment:** This is a **complement** to either A or B, not a standalone approach.
It provides at-a-glance metadata on the current state but no history by itself.
Useful for encoding the latest summary regardless of which history strategy is used.

---

## Query Mechanisms Comparison

| Query                                  | Approach A (squash)              | Approach B (chain)               |
|----------------------------------------|----------------------------------|----------------------------------|
| Current state                          | `jj file show -r wong-db`       | `jj file show -r wong-db`       |
| State at time T                        | `jj --at-op <op> file show ...` | `jj file show -r <snapshot>`    |
| Diff between two states                | Not directly possible            | `jj diff --from X --to Y`       |
| List all historical states             | `jj op log` (parse output)       | `jj log -r 'descendants(root-change)'` |
| Survives clone/fetch?                  | No (op log is local)             | Yes (commits in DAG)             |
| Survives gc?                           | No (pruneable)                   | Yes (reachable commits)          |
| Survives `jj op abandon`?             | No                               | Yes                              |

---

## Space and Complexity Tradeoffs

### Space (50 updates, ~150 bytes per issue)

| Metric                    | Approach A  | Approach B  |
|---------------------------|-------------|-------------|
| .jj size before gc        | 528K        | 591K        |
| .jj size after gc         | 370K        | N/A (nothing to gc) |
| Op log entries            | ~102        | ~52         |
| Commit objects            | 1 (latest)  | 51          |
| Overhead per snapshot     | ~3K (op)    | ~1.2K (commit + tree) |

After gc, Approach A is smaller because old tree objects are discarded.
Without gc, the overhead difference is modest (~12%).

### Complexity

| Aspect                    | Approach A  | Approach B  |
|---------------------------|-------------|-------------|
| Update code               | `describe` + file write | `commit` + file write + bookmark move |
| Read current state        | Identical   | Identical   |
| Read historical state     | Parse op log, use `--at-op` | Query DAG with revset |
| Pruning old history       | `op abandon` + `gc`    | Squash old snapshots into base |
| Multi-machine sync        | Not possible (op log local) | Works via push/fetch |

---

## Recommendation

**Use Approach A (squash-in-place) as the default, with an opt-in Approach B
mode for repos that need audit trails.**

### Rationale

1. **Simplicity wins for the common case.** Most wong users want a clean `jj log`
   with their actual work, not 50+ wong-db snapshots cluttering the DAG. Squash-in-place
   keeps wong-db as a single invisible sidechain change.

2. **Op log history is "good enough" for local use.** The experiment confirmed that
   `jj --at-op` perfectly recovers any past state of wong-db. For a single developer
   debugging "what did my issues look like yesterday?", this works.

3. **The chain approach has a real cost.** Every `jj log` shows the chain unless
   users add revset filters. This is UX friction that most users will not want.

4. **For audit/compliance needs, offer an explicit flag.** Something like
   `wong config set db.history chain` could switch to Approach B, creating the
   descendant chain. This is a clean opt-in for teams that need:
   - Cross-machine history (survives clone)
   - Tamper-evident audit trail
   - Diff between arbitrary historical states

5. **Always use Approach C (describe annotations).** Regardless of A vs B, embed
   metadata in the wong-db description. It is free and provides at-a-glance state.

### Implementation Notes

- Default (squash): `jj describe` the wong-db change with updated metadata,
  write files, let jj snapshot. Done.
- Chain mode: `jj commit` instead of amend, move the `wong-db` bookmark to
  the new tip. Use `jj log -r 'ancestors(wong-db) & descendants(<root-change>)'`
  to list history.
- Pruning chain mode: `jj squash --from <old-snapshots> --into <base>` to
  collapse old history while keeping recent snapshots.
- Hide from default log: Users can add `revsets.log = "all() ~ descendants(<wong-db-root>)"`
  to their config to hide the sidechain from `jj log`.

### Key Insight

jj's op log gives us snapshot history **for free** with the squash approach.
The chain approach is only needed when history must be **durable** (survive
gc, survive clone, be visible to other machines). Frame it as a durability
upgrade, not a default.
