# Epic: Stacked Diffs Workflow

## The designer's story

Wong exists because multi-agent coding has a coordination problem. You can
spin up 5 agents in parallel jj workspaces, but they're islands. Agent B
needs Agent A's Vec3 type. Agent C needs both. Right now somebody does
`cp` between workspace dirs, or Agent 6 independently rewrites everything
from scratch, or someone tries cross-workspace go module imports (which
don't work).

The real fix isn't any one clever hack. It's that the agents should be
producing *stacked changes* — a chain of jj changes where each one builds
on the previous, and every agent can see the code below them in the stack.
This is how good human engineers work with jj: you build a stack of
reviewable changes, each self-contained but layered.

The stacked diffs workflow also solves the review problem (humans can look
at each change independently and request amendments — jj auto-rebases
everything above), and the mainline tracking problem (when main moves
forward, the stack gets rebased automatically).

But all of that is the destination. We shouldn't build it all at once.

**Phase 0** solves the immediate pain: agents can't share code. One
command — `wong-prime` — that pulls a closed dependency's committed files
into the current workspace. That's it. No stack management, no review
flow, no rebase watcher. Just "give me the code my dependency produced."

**Phase 1** adds proper stacking. The lead agent's dependency graph maps
directly to a jj change stack: foundation changes at the bottom, dependent
changes on top. When a sub-agent finishes, their change slots into the
right place in the stack.

**Phase 2** pushes stacks for human review. This is mostly
`jj git push` with bookmark management.

**Phase 3** watches main and auto-rebases. This is where it gets
interesting — rebase failures become wongs that agents pick up, creating
a self-healing feedback loop.

Each phase is independently useful. You don't need Phase 3 to benefit
from Phase 0.

---

## The user's story

### Today (Phase 0 not yet built)

You ask 5 agents to build a web server. The lead agent decomposes
it into config, models, router, middleware, handler — with a dependency
graph. The agents run in parallel jj workspaces.

Agent "models" finishes first. Agent "handler" needs the model types.
You either:
- Manually copy files between workspace directories
- Tell the handler agent to rewrite the types itself
- Waste time on import path gymnastics that don't work

### After Phase 0: wong-prime

Same setup, but when Agent "handler" starts, it runs:

```bash
wong-prime http-models
```

This reads the `http-models` wong from wong-db, finds which workspace
(or jj change) produced the closed work, and copies those files into
the handler's workspace. Now the handler agent has the actual model
types to import. No manual copying.

### After Phase 1: stack-build

The lead agent doesn't just create wongs — it creates a *change stack*.
The dependency graph `config -> models, router -> middleware, handler`
becomes a chain of jj changes:

```
@  (handler - working)
|
o  middleware
|
o  router
|
o  models
|
o  config
|
o  wong-db
```

Each agent edits its own change in the stack (`jj edit <change-id>`).
When models finishes and config is already done, router's change
automatically sees both — because they're below it in the stack.

No copying. No priming. The code is just *there*.

### After Phase 2: review-push

You push the stack for review:

```bash
wong-review push
```

This pushes the stack as a series of bookmarked changes. A reviewer
(human or CI) can look at each change independently:

```
[handler]     +45 lines  "HTTP endpoint handlers"
[middleware]  +28 lines  "Logging and recovery middleware"
[router]     +32 lines  "Request routing"
[models]     +18 lines  "Data models"
[config]     +12 lines  "Server configuration"
```

The reviewer says "models needs a Validate method." An agent amends the
models change. jj auto-rebases everything above. Only the models change
needs re-review — the rest just gets a clean rebase.

### After Phase 3: rebase-watch

Main moves forward — someone else lands a PR, or another agent stack
gets merged. The rebase watcher:

1. Detects that main advanced past the stack's base
2. Creates a `rebase-config` wong: "rebase config onto new main, run tests"
3. An agent picks it up, does the rebase, runs tests
4. If tests pass: wong closed, stack updated
5. If tests fail: wong stays open with failure context, agent fixes

The whole stack stays current with main. When it's time to merge, there
are no surprises.

---

## The swarm's instructions

*For the AI agents that will implement and consume this system.*

### Vocabulary

- **wong**: an issue in wong-db. JSON file in `.wong/issues/`. Has an ID,
  status, dependencies, and whatever fields the workflow needs.
- **wong-db**: the jj bookmark pointing to a change off `root()` that
  stores all wong data. Accessed via `jj file show -r wong-db`.
- **prime**: to pull context into your workspace before starting work.
  Currently means "read your wong from wong-db." After Phase 0, also
  means "pull dependency code."
- **stack**: an ordered chain of jj changes. Parent-child in jj's DAG.
  Bottom of stack = foundation. Top = most dependent.
- **slot**: a change in the stack assigned to a specific wong. One wong
  per slot.

### Phase 0: wong-prime (unblocks q8)

**What you build**: A `wong-prime <dependency-id>` command (shell + Go).

**Behavior**:
1. Read the dependency wong from wong-db
2. Verify it's closed (error if not — can't prime on incomplete work)
3. Find the jj change where the dependency's code lives. Options:
   - The wong has a `change_id` field set when the agent closed it
   - Or: scan workspaces for the one that produced this wong's files
4. Copy the dependency's files into the current workspace working copy:
   `jj file show -r <dep-change-id> <path>` for each file
5. The files are now in the working copy, importable, compilable

**New wong fields needed**:
- `change_id`: the jj change ID where this wong's work was committed
- `files`: list of file paths produced (optional, for selective prime)

**What agents do differently**:
- When closing a wong, record `change_id` from `jj log -r @ -T change_id`
- When starting work, run `wong-prime <dep-id>` for each blocking dep
  that's closed
- `wong-ready` already checks if deps are closed — prime is the next step

**Shell API**:
```bash
# Pull dependency code into current workspace
wong-prime http-models

# Prime all closed dependencies for a wong
wong-prime-all http-handler
```

**Go API**:
```go
func (db *WongDB) Prime(ctx context.Context, depID string) error
func (db *WongDB) PrimeAll(ctx context.Context, issueID string) error
```

**Tests** (no jj needed):
- Prime on non-closed wong returns error
- Prime records which files were pulled
- PrimeAll skips already-primed deps

**Tests** (jj needed):
- Prime actually copies files between workspaces
- Files are importable after prime

---

### Phase 1: stack-build

**What you build**: A `wong-stack` command that arranges changes in
dependency order.

**Behavior**:
1. Read all open wongs and their dependency graph
2. Topological sort: leaves (no deps) at bottom, dependents on top
3. For each wong in order, create a jj change:
   `jj new <parent-change> -m "wong: <id> - <title>"`
4. Record the change ID in the wong's `slot_change_id` field
5. Agents `jj edit <slot_change_id>` to work on their assigned change

**Key insight**: jj's `--insert-after` and `--insert-before` let you
splice changes into the middle of a stack. When a dependency finishes
and its change gets content, everything above auto-rebases.

**New wong fields**:
- `slot_change_id`: the change in the stack assigned to this wong
- `stack_id`: identifies which stack this wong belongs to

**What agents do differently**:
- Instead of working in separate workspaces, agents `jj edit` their
  slot in the stack
- When done, they describe their change and `jj new` to get out
- Dependencies are visible by construction — they're below in the stack

---

### Phase 2: review-push

**What you build**: `wong-review push` and `wong-review status`.

**Behavior**:
- Create a bookmark for each change in the stack: `review/<wong-id>`
- Push all bookmarks: `jj git push -b review/<wong-id>`
- Track review state in the wong: `review_status` field

**What agents do differently**:
- After completing work, run `wong-review push`
- When a human requests changes on a specific layer, the agent
  `jj edit`s that change and amends it
- After amendment, `wong-review push` again — reviewers see the delta

---

### Phase 3: rebase-watch

**What you build**: A watcher (daemon or polling) that creates rebase
wongs when main advances.

**Behavior**:
1. Periodically check if main has advanced past the stack's base
2. If yes, create a wong: `rebase-<stack-id>-<timestamp>`
   - Description: "Rebase stack onto <new-main-commit>"
   - Priority: 1 (agents should pick this up soon)
3. The rebase agent:
   - `jj rebase -s <stack-bottom> -d <new-main>`
   - Run tests on each change in the stack
   - If all pass: close the rebase wong
   - If any fail: update the rebase wong with failure details, leave open

**Conflict handling**:
- jj marks conflicts inline (no conflict markers in files — jj's
  conflict model is richer)
- If rebase produces conflicts, the rebase wong includes which files
  conflict and the conflict descriptions
- An agent picks up the wong, resolves conflicts, runs tests

---

### Implementation order

```
Phase 0: wong-prime
  ├── Add change_id/files fields to types.Issue
  ├── Implement wong-close recording change_id
  ├── Implement Prime() in Go
  ├── Implement wong-prime in shell
  ├── Tests (unit + jj integration)
  └── Unblocks q8

Phase 1: stack-build (depends on Phase 0)
  ├── Topological sort of wong dependency graph
  ├── Change stack creation via jj new
  ├── Slot assignment and tracking
  └── Agent workflow: jj edit instead of workspace-per-agent

Phase 2: review-push (depends on Phase 1)
  ├── Bookmark management per stack slot
  ├── Push/pull review state
  └── Amendment workflow

Phase 3: rebase-watch (depends on Phase 1)
  ├── Main advancement detection
  ├── Rebase wong creation
  ├── Rebase execution + test running
  └── Conflict resolution wong creation
```

Phase 0 is a few hundred lines of code. Each subsequent phase roughly
doubles. Phase 3 is the most complex because it introduces a feedback
loop (wongs creating wongs).

---

### What stays the same

- Wong-db storage model (JSON files on parallel jj change)
- Flock-based sync serialization
- Shell helpers API (additive, not breaking)
- The wong itself as the unit of work assignment

### What changes

- Agents stop using one-workspace-per-agent (after Phase 1)
- Agents start using `jj edit` to navigate the stack
- Closing a wong records provenance (change_id, files)
- The lead agent produces both a dependency graph AND a change stack
