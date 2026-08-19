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
