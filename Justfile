# lint + test (default)
default: lint test test-ha test-voice

# eslint (strict-type-checked) + tsc --strict + knip
lint:
    npm run lint

# vitest, offline (no claude binary, no network)
test:
    npm test

# HA custom component (ha/): stdlib-only, no homeassistant install needed
test-ha:
    python3 -m unittest discover -s ha/tests

# Voice stream adapter (voice/): stdlib-only, no livekit install needed
test-voice:
    python3 -m unittest discover -s voice/tests

# Create a new worktree at .worktrees/BRANCH.
new-worktree BRANCH:
    #!/usr/bin/env bash
    set -euo pipefail
    if git show-ref --verify --quiet "refs/heads/{{BRANCH}}"; then
        git worktree add ".worktrees/{{BRANCH}}" "{{BRANCH}}"
    else
        git worktree add -b "{{BRANCH}}" ".worktrees/{{BRANCH}}"
    fi
    echo ""
    echo "Worktree ready: .worktrees/{{BRANCH}}"
    echo "Next: cd .worktrees/{{BRANCH}}"

# Remove the worktree at .worktrees/BRANCH.
rm-worktree BRANCH:
    git worktree remove ".worktrees/{{BRANCH}}"
