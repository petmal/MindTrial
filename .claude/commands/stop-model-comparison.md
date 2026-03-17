---
allowed-tools: Bash
description: Stop the running MindTrial eval suite and finalize the race transcript
---

Stop the MindTrial eval race.

Use the Bash tool to run:

**1. Kill the eval process and finalize transcript**
```bash
if [ -f /tmp/.eval_pid ]; then
  PID=$(cat /tmp/.eval_pid)
  if kill "$PID" 2>/dev/null; then
    echo "MindTrial eval stopped (PID $PID)."
  else
    echo "Process $PID not found (may have already finished)."
  fi
  rm -f /tmp/.eval_pid
  TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
  echo "" >> transcript.txt
  echo "━━━ Race Stopped: $TIMESTAMP ━━━" >> transcript.txt
  echo "Transcript finalized at transcript.txt"
else
  echo "No eval PID file found at /tmp/.eval_pid — nothing to stop."
fi
```

**2. Speak the stop announcement**
```bash
echo "The race has been stopped. MindTrial eval is shutting down. Check the transcript for the full play by play." | kokoro-tts - --stream --voice am_michael 2>/dev/null || echo "(kokoro-tts not available)"
```

**3. Show final leaderboard from the log (if available)**
```bash
LOG_FILE="logs/eval.log"
if [ -f "$LOG_FILE" ]; then
  echo ""
  echo "=== Final Leaderboard (tasks completed per model) ==="
  grep "task has finished" "$LOG_FILE" | sed 's/.*\] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
  echo ""
fi
```

Tell the user:
- The eval process has been stopped and the transcript finalized
- If they had a /loop running, they should cancel it now (type `/loop` and select cancel, or close the session)
- The leaderboard above shows how far each model got before the race was stopped
- They can review the full commentary in `transcript.txt`
