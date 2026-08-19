#!/usr/bin/env bash
set -euo pipefail

readonly avd_name="MentatApi36"
readonly serial="emulator-5554"
readonly script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
readonly android_dir="$(dirname "${script_dir}")"
readonly avd_home="${ANDROID_AVD_HOME:-${android_dir}/.avd}"
readonly emulator_pid_file="${avd_home}/emulator.pid"
readonly system_image="system-images;android-36;google_apis;x86_64"

require_sdk() {
  : "${ANDROID_SDK_ROOT:?ANDROID_SDK_ROOT must point to the Android SDK}"
}

create_avd() {
  require_sdk
  mkdir -p "${avd_home}"
  if ! avdmanager list avd | grep -Fq "Name: ${avd_name}"; then
    printf 'no\n' | avdmanager create avd --force --name "${avd_name}" --package "${system_image}" --device pixel_6
  fi
}

start() {
  create_avd
  if adb -s "${serial}" get-state 2>/dev/null | grep -qx device; then
    printf '%s\n' "${serial}"
    return
  fi

  emulator -avd "${avd_name}" -port 5554 -no-window -no-audio -no-boot-anim -no-snapshot -gpu swiftshader_indirect >"${avd_home}/emulator.log" 2>&1 &
  printf '%s\n' "$!" >"${emulator_pid_file}"

  for _ in $(seq 1 120); do
    if adb -s "${serial}" shell getprop sys.boot_completed 2>/dev/null | grep -qx 1; then
      printf '%s\n' "${serial}"
      return
    fi
    sleep 1
  done

  printf 'emulator did not complete boot; log follows:\n' >&2
  tail -n 100 "${avd_home}/emulator.log" >&2 || true
  return 1
}

stop() {
  adb -s "${serial}" emu kill >/dev/null 2>&1 || true
  if [[ -f "${emulator_pid_file}" ]]; then
    local pid
    pid="$(<"${emulator_pid_file}")"
    if kill -0 "${pid}" 2>/dev/null; then
      kill "${pid}" || true
      wait "${pid}" 2>/dev/null || true
    fi
    rm -f "${emulator_pid_file}"
  fi
}

case "${1:-}" in
  create-avd) create_avd ;;
  start) start ;;
  stop) stop ;;
  *)
    printf 'usage: %s {create-avd|start|stop}\n' "$0" >&2
    exit 64
    ;;
esac
