---
allowed-tools: Bash, Task, AskUserQuestion
description: Build and run MindTrial eval suite with live voice race commentary
---

The user wants to start a MindTrial eval race with live voice commentary.
Arguments: $ARGUMENTS (optional: config file, log path, loop interval, announcer mode)

Parse $ARGUMENTS:
- First arg: config file path (default: config-eval-top3-cicd.yaml)
- Second arg: log file path (default: logs/eval.log)
- Third arg: loop interval in minutes (default: 5)
- Fourth arg: optional flag — "with-announcer" or omitted

If the fourth arg is not provided, use AskUserQuestion to ask:
"How should the race announcer run?"
With options:
1. "with-announcer" — launch silently in the background (fully automatic, speaks every ~N min)
2. "optional-announcer" — I'll run `/loop Nm /announce-model-comparison` myself after this (gives you control via /loop, which supports CronDelete and auto-cancels when the race ends)
Use their answer to set the mode before continuing.

Use the Bash tool to do the following steps:

**1. Ensure log directory and transcript exist**
```bash
CONFIG_FILE="${1:-config-eval-top3-cicd.yaml}"
LOG_FILE="${2:-logs/eval.log}"
mkdir -p "$(dirname "$LOG_FILE")"
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
CONFIG_SLUG=$(basename "$CONFIG_FILE" .yaml)
RESULTS_DIR="results/$CONFIG_SLUG/$(date '+%m-%d-%Y')/$(date '+%I-%M%p' | tr '[:upper:]' '[:lower:]')"
mkdir -p "$RESULTS_DIR"
echo "$RESULTS_DIR" > /tmp/.race_results_dir
cat > "$RESULTS_DIR/transcript.txt" << HEADER
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  MINDTRIAL RACE — LIVE COMMENTARY TRANSCRIPT
  Started: $TIMESTAMP
  Config:  $CONFIG_FILE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

HEADER
echo "Transcript initialized at $RESULTS_DIR/transcript.txt"
```

**2. Build MindTrial**
```bash
echo "Building MindTrial..."
go build -o mindtrial ./cmd/mindtrial/
echo "Build complete."
```

**3. Kill any previous eval and start the eval suite in the background**
```bash
# Kill any existing eval first
if [ -f /tmp/.eval_pid ]; then
  OLD_PID=$(cat /tmp/.eval_pid)
  kill "$OLD_PID" 2>/dev/null && echo "Killed previous eval (PID: $OLD_PID)" || true
fi

CONFIG_FILE="${1:-config-eval-top3-cicd.yaml}"
LOG_FILE="${2:-logs/eval.log}"
nohup ./mindtrial -config "$CONFIG_FILE" -log "$LOG_FILE" run > /dev/null 2>&1 &
EVAL_PID=$!
echo "$EVAL_PID" > /tmp/.eval_pid
echo "MindTrial eval started (PID: $EVAL_PID)"
echo "Config: $CONFIG_FILE"
echo "Log:    $LOG_FILE"
```

**4. Wait for providers to initialize, then show opening lines**
```bash
sleep 5
head -20 "${2:-logs/eval.log}" 2>/dev/null || echo "(log not yet available)"
```

**5. Speak the race start announcement**
```bash
NUM_PROVIDERS=$(grep -c "starting [0-9]* tasks on this provider" "${2:-logs/eval.log}" 2>/dev/null || echo 3)
NUM_CONFIGS=$(grep "starting [0-9]* tasks on this provider" "${2:-logs/eval.log}" 2>/dev/null | grep -o "in [0-9]* configurations" | awk '{sum+=$2} END {print sum}')
NUM_CONFIGS=${NUM_CONFIGS:-6}
echo "Ladies and gentlemen, welcome to MindTrial! ${NUM_CONFIGS} model configurations across ${NUM_PROVIDERS} providers are lined up, the tasks are loaded, and we are LIVE. Let the race begin!" > /tmp/commentary.txt && kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9 2>/dev/null || echo "(kokoro-tts not available)"
```

**6. Launch announcer — behavior depends on the fourth argument (mode)**

The sleep duration is the third arg in seconds: `INTERVAL_SECS=$((${3:-3} * 60))`

If the fourth arg is "with-announcer":
- Use the Task tool with `run_in_background: true` and `subagent_type: "general-purpose"`, passing this prompt (substitute the actual sleep duration):

---
You are the live voice announcer for a MindTrial AI model race. The race log is at logs/eval.log in the current working directory (/Users/Ryan/Desktop/CODE/ryan-circleci/MindTrial).

Run the following loop until the race is over. Each iteration:

STEP 1 — Wait between updates
Run: sleep 300

STEP 2 — Check for race completion
Run: tail -5 logs/eval.log 2>/dev/null
If the output contains "all tasks in all configurations have finished on all providers", the race is OVER — go to STEP 4. Otherwise go to STEP 3.

STEP 3 — Live commentary (race still running)
Get the leaderboard:
  grep "task has finished" logs/eval.log | sed 's/.*] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
Get recent events:
  tail -60 logs/eval.log
Write 2-4 sentences of live commentary in the style of Vin Scully narrating a championship race between AI models:
- Reference models by short name: Claude, GPT, Gemini
- Call out the leader and close battles
- Compare speeds when two models finished the same task
- Treat ERR lines as dramatic setbacks
- Use racing metaphors: pulling ahead, gaining ground, the homestretch
- Under 80 words, plain ASCII only — NO apostrophes, quotes, backticks, backslashes, or special characters. No contractions. This text goes directly to TTS.
(a) Append to transcript.txt: blank line, separator with datetime and update number, your commentary.
(b) Write commentary to /tmp/commentary.txt and speak: kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9
Then go back to STEP 1.

STEP 4 — Final wrap-up (race over)
1. Get final leaderboard: grep "task has finished" logs/eval.log | sed 's/.*] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
2. Get provider finish times: grep "all tasks in all configurations have finished on this provider" logs/eval.log
3. Write 2-3 sentences of Vin Scully farewell commentary — winner, final standings, plain ASCII only.
4. Append to transcript.txt: blank line, "━━━ <datetime> — FINAL ━━━", then the sign-off.
5. Write sign-off to /tmp/commentary.txt and speak: kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9
6. Write results_summary.md: heading "MindTrial Race Results — <datetime>", config used, status COMPLETE, final leaderboard as markdown table, provider finish order, notable moments, full transcript.txt contents.
7. Write "Results summary written. What a race folks. Until next time!" to /tmp/commentary.txt and speak it.
8. Stop — you are done.
---

If no fourth arg (default):
- Do NOT launch a Task.
- Tell the user to start the announcer with:

/loop 5m /announce-model-comparison

**7. Tell the user the race is running**

Tell the user:
- The eval is running (show PID, config, log path)
- If with-announcer: the announcer is running silently in the background, will speak every ~5 minutes
- If no flag: they need to run the /loop command above to start the announcer
- Watch live: `tail -f transcript.txt`
- Stop everything: `/run stop-model-comparison`
- Auto announcer: `/run run-model-comparison config.yaml logs/eval.log 3 with-announcer`
