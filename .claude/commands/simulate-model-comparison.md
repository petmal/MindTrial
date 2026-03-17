---
allowed-tools: Bash, Task, AskUserQuestion
description: Simulate a MindTrial eval race (no API tokens) with live voice commentary
---

The user wants to simulate a MindTrial eval race without making real API calls.
This uses scripts/simulate-model-comparison.sh to generate realistic log output that the voice announcer narrates automatically.
Arguments: $ARGUMENTS (optional: num_tasks, speed multiplier, announcer mode)

Parse $ARGUMENTS:
- First arg: number of tasks per model (default: 30)
- Second arg: speed multiplier (default: 1, real-time pacing)
- Third arg: optional flag — "with-announcer" or omitted

If the third arg is not provided, use AskUserQuestion to ask:
"How should the race announcer run?"
With options:
1. "with-announcer" — launch silently in the background (fully automatic, speaks every ~1 min)
2. "optional-announcer" — I'll run `/loop 1m /announce-model-comparison` myself after this (gives you the command)
Use their answer to set the mode before continuing.

Use the Bash tool to do the following steps:

**1. Initialize transcript**
```bash
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
RESULTS_DIR="results/simulation/$(date '+%m-%d-%Y')/$(date '+%I-%M%p' | tr '[:upper:]' '[:lower:]')"
LOG_FILE="$RESULTS_DIR/eval.log"
mkdir -p "$RESULTS_DIR"
echo "$RESULTS_DIR" > /tmp/.race_results_dir
echo "$LOG_FILE" > /tmp/.race_log_file
cat > "$RESULTS_DIR/transcript.txt" << HEADER
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  MINDTRIAL RACE — LIVE COMMENTARY TRANSCRIPT
  Started: $TIMESTAMP
  Mode:    SIMULATION (no API calls)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

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
LOG_FILE=$(cat /tmp/.race_log_file)
nohup bash scripts/simulate-model-comparison.sh "$LOG_FILE" "$NUM_TASKS" "$SPEED" > /dev/null 2>&1 &
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
head -15 "$(cat /tmp/.race_log_file)" 2>/dev/null || echo "(log not yet available)"
```

**4. Speak the race start announcement**
```bash
echo "Ladies and gentlemen, welcome to a MindTrial simulation! Six model configurations are about to go head to head. No real tokens, all the drama. Let the race begin!" > /tmp/commentary.txt && kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9 2>/dev/null || echo "(kokoro-tts not available)"
```

**5. Launch announcer — behavior depends on the third argument (mode)**

The announcer prompt is the same regardless of mode:

```
ANNOUNCER_PROMPT="
You are the live voice announcer for a MindTrial AI model race.

First, resolve paths:
  RESULTS_DIR=$(cat /tmp/.race_results_dir 2>/dev/null || echo ".")
  LOG_FILE=$(cat /tmp/.race_log_file 2>/dev/null || echo "logs/eval.log")
  TRANSCRIPT="$RESULTS_DIR/transcript.txt"

Run the following loop until the race is over. Each iteration:

STEP 1 — Wait 60 seconds
Run: sleep 60

STEP 2 — Check for race completion
Run: tail -5 "$LOG_FILE" 2>/dev/null
If the output contains 'all tasks in all configurations have finished on all providers', the race is OVER — go to STEP 4. Otherwise go to STEP 3.

STEP 3 — Live commentary (race still running)
Get the leaderboard:
  grep 'task has finished' "$LOG_FILE" | sed 's/.*] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
Get recent events:
  tail -60 "$LOG_FILE"
Write 2-4 sentences of live commentary in the style of Vin Scully narrating a championship race between AI models:
- Reference models by short name: Claude, GPT, Gemini
- Call out the leader and close battles
- Treat ERR lines as dramatic setbacks
- Use racing metaphors: pulling ahead, gaining ground, the homestretch
- Under 80 words, plain ASCII only — NO apostrophes, quotes, backticks, backslashes, or special characters. No contractions. This text goes directly to TTS.
Then:
(a) Append to $TRANSCRIPT: blank line, separator with datetime and update number, your commentary.
(b) Write commentary to /tmp/commentary.txt and speak: kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9
Then go back to STEP 1.

STEP 4 — Final wrap-up (race over)
1. Get final leaderboard: grep 'task has finished' "$LOG_FILE" | sed 's/.*] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
2. Get provider finish times: grep 'all tasks in all configurations have finished on this provider' "$LOG_FILE"
3. Write 2-3 sentences of Vin Scully farewell commentary — winner, final standings, plain ASCII only.
4. Append to transcript.txt: blank line, '━━━ <datetime> — FINAL ━━━', then the sign-off.
5. Write sign-off to /tmp/commentary.txt and speak: kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9
6. Write results_summary.md: heading 'MindTrial Race Results — <datetime>', mode SIMULATION, final leaderboard as markdown table, provider finish order, notable moments, full transcript.txt contents.
7. Write 'Results summary written. What a race folks. Until next time!' to /tmp/commentary.txt and speak it.
8. Stop — you are done.
"
```

If the mode is "with-announcer":
- Use the Task tool with `run_in_background: true` and `subagent_type: "general-purpose"`, passing the announcer prompt above.
- The announcer runs silently in the background.

If the mode is "optional-announcer":
- Do NOT launch a Task.
- Tell the user to start the announcer with:

/loop 1m /announce-model-comparison

**6. Tell the user the race is running**

Tell the user:
- The simulation is running (6 configs: GPT-5.4, GPT-5.2, Gemini 3.1 Pro, Gemini 2.5 Flash, Claude Opus 4.6, Claude Sonnet 4.6)
- If with-announcer: the announcer is running silently in the background, will speak every ~1 minute
- If no flag: they need to run the /loop command above to start the announcer
- Watch live: `tail -f transcript.txt`
- Stop: `/run stop-model-comparison`
- Default is 30 tasks at 1x speed (~5-6 minutes), 4-5 announcements
- Quick sprint: `/run simulate-model-comparison 10 10`
- Auto announcer: `/run simulate-model-comparison 30 1 with-announcer`
