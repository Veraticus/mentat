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

# Score the front's consult-vs-answer judgment: online, needs LIVEKIT_INFERENCE_*
eval-voice *ARGS:
    # Never in `default`: this one spends tokens (a fraction of a cent) against
    # real Luna. The interpreter is the flake's voice-env because the runner
    # imports livekit-agents, which lives nowhere else; --no-link keeps a
    # result/ symlink out of the working tree.
    "$(nix build .#voice-env --no-link --print-out-paths)/bin/python" voice/evals/run.py {{ARGS}}
