# Wong CLI Feature Parity with JJ

This document tracks which jj CLI commands are implemented, planned, or out of scope for the wong VCS abstraction layer.

## Legend
- ✅ **Implemented** - Available in VCS interface
- 🔄 **Planned** - Will be implemented
- ⏸️ **Deferred** - Lower priority, after core workflow
- ❌ **Out of Scope** - Not needed for wong/beads integration

## Core Commands

| jj Command | Status | Wong Method | Notes |
|------------|--------|-------------|-------|
| `abandon` | ✅ | `JujutsuVCS.Abandon()` | Abandons changes |
| `absorb` | ⏸️ | - | Advanced stacking feature |
| `bisect` | ❌ | - | Not needed for beads workflow |
| `bookmark` | ✅ | `ListBranches()`, `CreateBranch()`, `DeleteBranch()`, `MoveBranch()`, `SetBranch()`, `TrackBranch()`, `UntrackBranch()` | Full bookmark management |
| `commit` | ✅ | `Commit()` | Creates new change |
| `config` | ❌ | - | User manages config directly |
| `describe` | ✅ | `JujutsuVCS.Describe()` | Update change description |
| `diff` | ✅ | `Diff()` | Compare revisions |
| `diffedit` | ⏸️ | - | Interactive editing |
| `duplicate` | ⏸️ | - | Could be useful for subtasks |
| `edit` | ✅ | `Edit()` | Set working copy target |
| `evolog` | ❌ | - | Debugging/history exploration |
| `file` | ✅ | `TrackFiles()`, `UntrackFiles()`, `GetFileVersion()` | File operations |
| `fix` | ❌ | - | Formatting tool integration |
| `gerrit` | ❌ | - | Gerrit-specific |
| `git` | ✅ | `GitExport()`, `GitImport()`, `Fetch()`, `Push()` | Git interop |
| `help` | ❌ | - | CLI help |
| `interdiff` | ⏸️ | - | Advanced diff comparison |
| `log` | ✅ | `Log()` | Show history |
| `metaedit` | ⏸️ | - | Metadata editing |
| `new` | ✅ | `New()` | Create new change |
| `next` | ✅ | `Next()` | Navigate stack down |
| `operation` | ⏸️ | - | Operation log (undo/redo) |
| `parallelize` | ⏸️ | - | Advanced stacking |
| `prev` | ✅ | `Prev()` | Navigate stack up |
| `rebase` | ✅ | `JujutsuVCS.Rebase()` | Move changes |
| `redo` | ⏸️ | - | Redo operation |
| `resolve` | ✅ | `MarkResolved()`, `GetConflicts()` | Conflict resolution |
| `restore` | ⏸️ | - | Restore paths |
| `revert` | ⏸️ | - | Reverse changes |
| `root` | ✅ | `RepoRoot()` | Get workspace root |
| `show` | ✅ | `Show()` | Show change details |
| `sign` | ❌ | - | Cryptographic signing |
| `simplify-parents` | ⏸️ | - | Graph cleanup |
| `sparse` | ⏸️ | - | Sparse checkouts |
| `split` | ⏸️ | - | Split changes |
| `squash` | ✅ | `Squash()` | Combine changes |
| `status` | ✅ | `Status()` | Working copy status |
| `tag` | ⏸️ | - | Tag management |
| `undo` | ⏸️ | - | Undo operation |
| `unsign` | ❌ | - | Remove signatures |
| `util` | ❌ | - | Shell completions etc. |
| `version` | ❌ | - | Version info |
| `workspace` | ✅ | `ListWorkspaces()`, `CreateWorkspace()`, `RemoveWorkspace()`, `UpdateStaleWorkspace()` | Core for subtask orchestration |

## Git Subcommands

| jj git Command | Status | Wong Method | Notes |
|----------------|--------|-------------|-------|
| `git clone` | ❌ | - | Initial setup only |
| `git export` | ✅ | `GitExport()` | Export to git |
| `git fetch` | ✅ | `Fetch()` | Fetch from remote |
| `git import` | ✅ | `GitImport()` | Import from git |
| `git init` | ❌ | - | Initial setup only |
| `git push` | ✅ | `Push()` | Push to remote |
| `git remote` | ✅ | `GetRemote()`, `HasRemote()` | Remote management |
| `git submodule` | ❌ | - | Submodule support |

## Workspace Subcommands

| jj workspace Command | Status | Wong Method | Notes |
|----------------------|--------|-------------|-------|
| `workspace add` | ✅ | `CreateWorkspace()` | Create workspace |
| `workspace forget` | ✅ | `RemoveWorkspace()` | Remove workspace |
| `workspace list` | ✅ | `ListWorkspaces()` | List workspaces |
| `workspace root` | ✅ | `RepoRoot()` | Get root path |
| `workspace update-stale` | ✅ | `UpdateStaleWorkspace()` | Handle stale workspaces |

## Bookmark Subcommands

| jj bookmark Command | Status | Wong Method | Notes |
|---------------------|--------|-------------|-------|
| `bookmark create` | ✅ | `CreateBranch()` | Create bookmark |
| `bookmark delete` | ✅ | `DeleteBranch()` | Delete bookmark |
| `bookmark forget` | 🔄 | - | Forget bookmark |
| `bookmark list` | ✅ | `ListBranches()` | List bookmarks |
| `bookmark move` | ✅ | `MoveBranch()` | Move bookmark |
| `bookmark rename` | 🔄 | - | Rename bookmark |
| `bookmark set` | ✅ | `SetBranch()` | Set bookmark |
| `bookmark track` | ✅ | `TrackBranch()` | Track remote |
| `bookmark untrack` | ✅ | `UntrackBranch()` | Untrack remote |

## File Subcommands

| jj file Command | Status | Wong Method | Notes |
|-----------------|--------|-------------|-------|
| `file annotate` | ❌ | - | Blame/annotate |
| `file chmod` | ❌ | - | Change permissions |
| `file list` | ⏸️ | - | List files |
| `file show` | ✅ | `GetFileVersion()` | Show file at revision |
| `file track` | ✅ | `TrackFiles()` | Track files |
| `file untrack` | ✅ | `UntrackFiles()` | Untrack files |

## Priority Summary

### P0 - Core Workflow (✅ Done)
- `status`, `commit`, `log`, `diff`, `show`
- `workspace add/forget/list`
- `git fetch/push/export/import`
- `squash`, `new`, `edit`, `rebase`, `abandon`

### P1 - Stack Navigation & File Ops (✅ Done)
- `next`, `prev` - Navigate change stack
- `workspace update-stale` - Handle stale workspaces
- `file track/untrack` - File management

### P2 - Bookmark Management (✅ Done)
- `bookmark delete/move/set/track/untrack`

### P3 - Advanced Features (⏸️ Deferred)
- `absorb`, `split`, `duplicate`
- `operation`, `undo`, `redo`
- `sparse`, `interdiff`, `metaedit`

### Out of Scope (❌)
- `bisect`, `config`, `help`, `version`, `util`
- `sign`, `unsign`, `fix`, `gerrit`
- `git clone/init/submodule`

## Implementation Progress

| Category | Implemented | Planned | Deferred | Out of Scope | Total |
|----------|-------------|---------|----------|--------------|-------|
| Core | 22 | 0 | 11 | 9 | 42 |
| Git | 5 | 0 | 0 | 3 | 8 |
| Workspace | 5 | 0 | 0 | 0 | 5 |
| Bookmark | 7 | 2 | 0 | 0 | 9 |
| File | 3 | 0 | 1 | 2 | 6 |
| **Total** | **42** | **2** | **12** | **14** | **70** |

**Coverage: 60% implemented, 63% with planned**
