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

# Android Gradle tests run inside the pinned Android SDK development shell.
test-android:
    nix develop .#android -c bash -ceu 'cd android && gradle test'

# Boot a headless emulator and prove the assistant role receives KEYCODE_ASSIST.
android-e2e:
    nix develop .#android -c bash -ceu '\
        export ANDROID_AVD_HOME="$PWD/android/.avd"; \
        export ANDROID_USER_HOME="$PWD/android/.android"; \
        export GRADLE_USER_HOME="$PWD/android/.gradle"; \
        trap "android/scripts/emulator.sh stop; adb kill-server" EXIT; \
        (cd android && gradle assembleDebug); \
        android/scripts/emulator.sh create-avd; \
        serial="$(android/scripts/emulator.sh start)"; \
        adb -s "$serial" install -r android/app/build/outputs/apk/debug/app-debug.apk; \
        api_level="$(adb -s "$serial" shell getprop ro.build.version.sdk)"; \
        printf "Assistant role image: google_apis API %s\\n" "$api_level"; \
        adb -s "$serial" shell cmd role add-role-holder --user 0 android.app.role.ASSISTANT gg.savecraft.mentat; \
        adb -s "$serial" logcat -c; \
        adb -s "$serial" shell input keyevent KEYCODE_ASSIST; \
        for _ in $(seq 1 30); do \
          if adb -s "$serial" logcat -d -s MentatAssist:I | grep -Fq MENTAT_ASSIST_RECEIVED; then \
            adb -s "$serial" logcat -d -s MentatAssist:I | grep -F MENTAT_ASSIST_RECEIVED; \
            exit 0; \
          fi; \
          sleep 1; \
        done; \
        adb -s "$serial" logcat -d -s MentatAssist:I; \
        exit 1'
