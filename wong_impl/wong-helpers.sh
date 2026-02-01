#!/usr/bin/env bash
# wong-helpers.sh - Shell helpers for wong issue tracking in jj workspaces.
# Source this file in agent shells: source /path/to/wong-helpers.sh
#
# All wong-write/close/subtask operations use flock to prevent concurrent
# squash bookmark conflicts when multiple agents run in parallel workspaces.

set -euo pipefail

# Resolve the canonical .jj/repo path (handles workspace text-file indirection)
_wong_canonical_repo() {
    local repo_path=".jj/repo"
    if [ -f "$repo_path" ] && [ ! -d "$repo_path" ]; then
        # Workspace: .jj/repo is a text file pointing to canonical repo
        cat "$repo_path"
    else
        echo "$repo_path"
    fi
}

# Get the lock file path for wong-db sync serialization
_wong_lock_path() {
    echo "$(_wong_canonical_repo)/wong-sync.lock"
}

# Internal: squash .wong/ changes into wong-db with retry on stale WC
# Usage: _wong_sync <pending_file> <pending_data>
# Pass the file path and data as arguments (not globals) to avoid races.
_wong_sync() {
    local pending_file="${1:-}"
    local pending_data="${2:-}"
    local lock_path
    lock_path="$(_wong_lock_path)"
    touch "$lock_path"

    (
        flock -x 200
        # Inside the lock: update-stale, restore files, squash
        jj workspace update-stale 2>/dev/null || true

        # Re-write any .wong/ files that were on disk before locking
        # (update-stale may have overwritten them)
        if [ -n "$pending_file" ] && [ -n "$pending_data" ]; then
            mkdir -p "$(dirname "$pending_file")"
            printf '%s\n' "$pending_data" > "$pending_file"
        fi

        local result
        result=$(jj squash --into wong-db ".wong/" -u \
            --config 'revset-aliases."immutable_heads()"="none()"' 2>&1) || {
            if echo "$result" | grep -qi "nothing changed\|no changes"; then
                return 0
            fi
            echo "wong-sync error: $result" >&2
            return 1
        }
    ) 200>"$lock_path"
}

# wong-read <id> - Read an issue from wong-db by ID
wong-read() {
    local id="${1:?Usage: wong-read <issue-id>}"
    jj workspace update-stale 2>/dev/null || true
    jj file show -r wong-db ".wong/issues/${id}.json" 2>/dev/null
}

# wong-write <id> '<json>' - Write an issue and sync to wong-db atomically
wong-write() {
    local id="${1:?Usage: wong-write <issue-id> '<json>'}"
    local json="${2:?Usage: wong-write <issue-id> '<json>'}"
    local file=".wong/issues/${id}.json"

    mkdir -p .wong/issues
    printf '%s\n' "$json" > "$file"

    # Pass file/data as arguments (not globals) to avoid races
    _wong_sync "$file" "$json"
}

# wong-close <id> "reason" - Close an issue with a resolution note
wong-close() {
    local id="${1:?Usage: wong-close <issue-id> [reason]}"
    local reason="${2:-completed}"

    local current
    current=$(wong-read "$id") || {
        echo "wong-close: cannot read issue $id" >&2
        return 1
    }

    local updated
    updated=$(echo "$current" | WONG_REASON="$reason" python3 -c "
import sys, json, os
d = json.load(sys.stdin)
d['status'] = 'closed'
d['resolution'] = os.environ['WONG_REASON']
print(json.dumps(d, indent=2))
") || {
        echo "wong-close: failed to update JSON for $id" >&2
        return 1
    }

    wong-write "$id" "$updated"
}

# wong-subtask <id> <title> <parent-id> [description] - Create a subtask issue
wong-subtask() {
    local id="${1:?Usage: wong-subtask <id> <title> <parent-id> [description]}"
    local title="${2:?Usage: wong-subtask <id> <title> <parent-id> [description]}"
    local parent="${3:?Usage: wong-subtask <id> <title> <parent-id> [description]}"
    local desc="${4:-}"

    local json
    json=$(WONG_ID="$id" WONG_TITLE="$title" WONG_PARENT="$parent" WONG_DESC="$desc" python3 -c "
import json, os
d = {
    'id': os.environ['WONG_ID'],
    'title': os.environ['WONG_TITLE'],
    'status': 'open',
    'parent': os.environ['WONG_PARENT'],
    'description': os.environ['WONG_DESC']
}
print(json.dumps(d, indent=2))
")

    wong-write "$id" "$json"
}

# wong-list - List all issue IDs in wong-db
wong-list() {
    jj workspace update-stale 2>/dev/null || true
    jj file list -r wong-db 2>/dev/null | grep '.wong/issues/' | sed 's|.wong/issues/||;s|\.json||'
}

# wong-ready <id> - Check if an issue's dependencies are all closed.
# Returns 0 (true) if ready, 1 (false) if blocked.
# Prints blocking dependency IDs to stderr if not ready.
wong-ready() {
    local id="${1:?Usage: wong-ready <issue-id>}"

    local issue
    issue=$(wong-read "$id") || {
        echo "wong-ready: cannot read issue $id" >&2
        return 1
    }

    # Extract dependencies array
    local deps
    deps=$(echo "$issue" | python3 -c "
import sys, json
d = json.load(sys.stdin)
deps = d.get('dependencies', d.get('depends_on', []))
if isinstance(deps, list):
    for dep in deps:
        print(dep)
" 2>/dev/null) || true

    if [ -z "$deps" ]; then
        # No dependencies - always ready
        return 0
    fi

    local blocked=()
    while IFS= read -r dep_id; do
        [ -z "$dep_id" ] && continue
        local dep_status
        dep_status=$(wong-read "$dep_id" 2>/dev/null | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d.get('status', 'unknown'))
" 2>/dev/null) || dep_status="unknown"

        if [ "$dep_status" != "closed" ]; then
            blocked+=("$dep_id ($dep_status)")
        fi
    done <<< "$deps"

    if [ ${#blocked[@]} -gt 0 ]; then
        echo "wong-ready: $id blocked by: ${blocked[*]}" >&2
        return 1
    fi

    return 0
}

# wong-dispatch - List all issues that are ready to be worked on.
# An issue is "ready" if:
#   1. Its status is "open" or "blocked"
#   2. All its dependencies are "closed"
wong-dispatch() {
    local all_ids
    all_ids=$(wong-list)

    local ready_ids=()
    while IFS= read -r id; do
        [ -z "$id" ] && continue

        local status
        status=$(wong-read "$id" 2>/dev/null | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d.get('status', 'unknown'))
" 2>/dev/null) || continue

        # Skip already closed or unknown
        if [ "$status" = "closed" ] || [ "$status" = "unknown" ]; then
            continue
        fi

        # Check if dependencies are met
        if wong-ready "$id" 2>/dev/null; then
            ready_ids+=("$id")
        fi
    done <<< "$all_ids"

    if [ ${#ready_ids[@]} -gt 0 ]; then
        printf '%s\n' "${ready_ids[@]}"
    fi
}

# wong-status - Print a summary table of all issues
wong-status() {
    local all_ids
    all_ids=$(wong-list)

    printf "%-12s %-10s %-50s\n" "ID" "STATUS" "TITLE"
    printf "%-12s %-10s %-50s\n" "---" "------" "-----"

    while IFS= read -r id; do
        [ -z "$id" ] && continue
        local info
        info=$(wong-read "$id" 2>/dev/null | python3 -c "
import sys, json
d = json.load(sys.stdin)
print(d.get('status', '?') + '|' + d.get('title', '(no title)')[:50])
" 2>/dev/null) || info="?|(error)"

        local status title
        status="${info%%|*}"
        title="${info#*|}"
        printf "%-12s %-10s %-50s\n" "$id" "$status" "$title"
    done <<< "$all_ids"
}
