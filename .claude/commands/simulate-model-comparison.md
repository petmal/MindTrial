---
allowed-tools: Bash, AskUserQuestion, CronCreate
description: Simulate a MindTrial eval race (no API tokens), with optional live voice commentary
---

The user wants to simulate a MindTrial eval race without making real API calls.
This uses scripts/simulate-model-comparison.sh to generate realistic log output.
Arguments: $ARGUMENTS (optional: num_tasks, speed multiplier, `with-announcer`)

Parse $ARGUMENTS:
- First numeric arg (not immediately following `with-announcer`): number of tasks per model (default: 30)
- Second numeric arg (not immediately following `with-announcer`): speed multiplier (default: 1, real-time pacing)
- Any arg equal to `with-announcer`: enable voice announcer
- A numeric arg immediately following `with-announcer` in the arg list: announcer loop interval in minutes (default: 1 if not specified)

**Step 0 — Determine announcer mode**

If `with-announcer` is NOT present in $ARGUMENTS, use AskUserQuestion to ask:
"Want live voice commentary for this race?"
With options:
1. Yes — enable the voice announcer (requires kokoro-tts)
2. No — run silently (no TTS, no announcer loop)

Set `ANNOUNCER_ENABLED` to `true` or `false` based on the flag or the user's answer before continuing.

If `ANNOUNCER_ENABLED` is `true` AND no interval was specified in $ARGUMENTS, use AskUserQuestion to ask:
"How often should the announcer check in?"
With options:
1. Every 1 minute (default)
2. Every 2 minutes
3. Every 5 minutes

Set `ANNOUNCER_INTERVAL` to the chosen number of minutes (default: 1 if skipped or unspecified).

Use the Bash tool to do the following steps:

**1. Initialize transcript**
```bash
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
RESULTS_DIR="results/simulation/$(date '+%Y-%m-%d')/$(date '+%H-%M-%S')"
LOG_FILE="$RESULTS_DIR/eval.log"
mkdir -p "$RESULTS_DIR"
echo "$RESULTS_DIR" > /tmp/.eval_results_dir
echo "$LOG_FILE" > /tmp/.eval_log_file
echo "${ANNOUNCER_INTERVAL:-1}" > /tmp/.eval_loop_interval_mins
cat > "$RESULTS_DIR/transcript.txt" << HEADER
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  MODEL COMPARISON — LIVE COMMENTARY TRANSCRIPT
  Started: $TIMESTAMP
  Mode:    SIMULATION (no API calls)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

HEADER
echo "Transcript initialized at $RESULTS_DIR/transcript.txt"
```

**2. Start the simulation in the background**
```bash
# Kill any existing simulation first
if [ -f /tmp/.eval_pid ]; then
  OLD_PID=$(cat /tmp/.eval_pid)
  kill "$OLD_PID" 2>/dev/null && echo "Killed previous simulation (PID: $OLD_PID)" || true
fi

NUM_TASKS="${1:-30}"
SPEED="${2:-1}"
LOG_FILE=$(cat /tmp/.eval_log_file)
nohup python3 scripts/simulate-model-comparison.sh "$LOG_FILE" "$NUM_TASKS" "$SPEED" > /dev/null 2>&1 &
SIM_PID=$!
echo "$SIM_PID" > /tmp/.eval_pid
echo "Simulation started (PID: $SIM_PID)"
echo "Tasks per model: $NUM_TASKS"
echo "Speed: ${SPEED}x"
echo "Log: $LOG_FILE"
```

**3. Wait for simulation to begin, confirm it's writing**
```bash
sleep 3
head -15 "$(cat /tmp/.eval_log_file)" 2>/dev/null || echo "(log not yet available)"
```

**4. Open log and transcript in separate Terminal windows**
```bash
bash scripts/open-dashboard.sh
```

**5. (ANNOUNCER ONLY) Speak the race start announcement**

Skip this step entirely if `ANNOUNCER_ENABLED` is `false`.

```bash
echo "Ladies and gentlemen, welcome to a MindTrial simulation! Six model configurations are about to go head to head. No real tokens, all the drama. Let the race begin!" > /tmp/commentary.txt && kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9 2>/dev/null || echo "(kokoro-tts not available)"
```

**6. (ANNOUNCER ONLY) Auto-start the announcer loop**

Skip this step entirely if `ANNOUNCER_ENABLED` is `false`.

Use the CronCreate tool to schedule the announcer automatically:
- `cron`: `*/${ANNOUNCER_INTERVAL:-1} * * * *` (every N minutes, using the parsed interval)
- `prompt`: `/announce-model-comparison`
- `recurring`: `true`

Save the returned job ID to show the user.

**7. Tell the user the race is running**

Tell the user:
- The simulation is running (show PID, tasks, speed, log path)
- If announcer is enabled: the announcer loop is running every `ANNOUNCER_INTERVAL` minute(s) (show job ID, cancel with CronDelete if needed)
- If announcer is disabled: mention they can re-run with `with-announcer` to enable voice commentary, or manually kick off commentary at any time with `/loop 1m /announce-model-comparison`
- Watch live:
  - Transcript: `tail -f <RESULTS_DIR>/transcript.txt`
  - Log: `tail -f <LOG_FILE>`
- Stop: `/stop-model-comparison`
- Default is 30 tasks at 1x speed (~5-6 minutes)
- Quick sprint: `/simulate-model-comparison 10 2`
