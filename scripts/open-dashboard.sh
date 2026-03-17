#!/usr/bin/env bash
# Opens two Terminal.app windows to tail the live eval log and transcript.
# Usage: ./scripts/watch-race.sh [log_file] [transcript_file]

set -euo pipefail

LOG_FILE="${1:-$(cat /tmp/.eval_log_file 2>/dev/null)}"
RESULTS_DIR="${2:-$(cat /tmp/.eval_results_dir 2>/dev/null)}"
TRANSCRIPT="$RESULTS_DIR/transcript.txt"

if [[ -z "$LOG_FILE" || -z "$RESULTS_DIR" ]]; then
  echo "No active race found. Start one with /run-model-comparison first." >&2
  exit 1
fi

ABS_LOG="$(cd "$(dirname "$LOG_FILE")" && pwd)/$(basename "$LOG_FILE")"
ABS_TRANSCRIPT="$(cd "$(dirname "$TRANSCRIPT")" && pwd)/$(basename "$TRANSCRIPT")"

echo "Opening tail windows for:"
echo "  Log:        $ABS_LOG"
echo "  Transcript: $ABS_TRANSCRIPT"

open_terminal_window() {
  local file="$1"
  osascript -e "tell application \"Terminal\" to do script \"tail -f '$file'\"" \
            -e "tell application \"Terminal\" to activate"
}

open_terminal_window "$ABS_LOG"
open_terminal_window "$ABS_TRANSCRIPT"

echo "Done."
