---
allowed-tools: Bash
description: Build and run MindTrial eval suite with live voice race commentary via /loop
---

The user wants to start a MindTrial eval race with live voice commentary.
Arguments: $ARGUMENTS (optional: config file, log path, loop interval)

Parse $ARGUMENTS:
- First arg: config file path (default: config-eval-top3-cicd.yaml)
- Second arg: log file path (default: logs/eval.log)
- Third arg: loop interval (default: 3m)

Use the Bash tool to do the following steps:

**1. Ensure log directory and transcript exist**
```bash
CONFIG_FILE="${1:-config-eval-top3-cicd.yaml}"
LOG_FILE="${2:-logs/eval.log}"
mkdir -p "$(dirname "$LOG_FILE")"
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
cat > transcript.txt << HEADER
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  MINDTRIAL RACE — LIVE COMMENTARY TRANSCRIPT
  Started: $TIMESTAMP
  Config:  $CONFIG_FILE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

HEADER
echo "Transcript initialized at transcript.txt"
```

**2. Build MindTrial**
```bash
echo "Building MindTrial..."
go build -o mindtrial ./cmd/mindtrial/
echo "Build complete."
```

**3. Start the eval suite in the background**
```bash
CONFIG_FILE="${1:-config-eval-top3-cicd.yaml}"
LOG_FILE="${2:-logs/eval.log}"
nohup ./mindtrial -config "$CONFIG_FILE" -log "$LOG_FILE" run > /dev/null 2>&1 &
EVAL_PID=$!
echo "$EVAL_PID" > /tmp/.eval_pid
echo "MindTrial eval started (PID: $EVAL_PID)"
echo "Config: $CONFIG_FILE"
echo "Log:    $LOG_FILE"
```

**4. Wait a few seconds for providers to initialize, then read the opening lines of the log**
```bash
sleep 5
head -20 "${2:-logs/eval.log}" 2>/dev/null || echo "(log not yet available)"
```

**5. Speak the race start announcement via Kokoro TTS**
```bash
echo "Ladies and gentlemen, welcome to MindTrial! The models are lined up, the tasks are loaded, and we are LIVE! Let the race begin!" | kokoro-tts - --stream --voice af_heart 2>/dev/null || echo "(kokoro-tts not available — install with: pipx install kokoro-tts --python python3.12)"
```

**6. Confirm setup to the user, then instruct them to start the /loop**

Tell the user:
- The eval is running (show PID, config, log path)
- The opening lines of the log show which models are racing
- Now they should start the live voice commentator by running the /loop command below
- They can watch the transcript live with `tail -f transcript.txt` in a split pane
- They can stop everything later with `/run stop-model-comparison`

Give the user this exact /loop command to copy-paste (adjust the interval from the parsed args).
IMPORTANT: Output this as a single copyable block. Replace "3m" with the user's chosen interval. Replace "logs/eval.log" with the user's chosen log path if different.

---

/loop 3m Do the following steps in order using Bash. The MindTrial eval log is at logs/eval.log.

STEP 1 — CHECK FOR RACE COMPLETION
Run these two commands:
  cat /tmp/.eval_pid 2>/dev/null | xargs ps -p 2>/dev/null
  tail -5 logs/eval.log 2>/dev/null
If the log contains "all tasks in all configurations have finished on all providers", the race is OVER — go to STEP 2B. Otherwise go to STEP 2A.

STEP 2A — IF THE RACE IS STILL RUNNING: give live commentary
First, get the leaderboard by running:
  grep "task has finished" logs/eval.log | sed 's/.*] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
This shows task completion counts per model/run, sorted by leader.

Then get the latest action by running:
  tail -60 logs/eval.log

Now write 2-4 sentences of live race commentary in the style of legendary baseball announcer Vin Scully calling a championship race between AI models. Rules:
- Reference specific models by short name (Claude, GPT, Gemini) and their task counts from the leaderboard
- Call out the LEADER (most tasks completed) and any close battles
- When two models finished the same task, compare their speeds — who was faster?
- If there are ERR or error lines, treat them as dramatic setbacks or penalties
- Use racing and competition metaphors — laps, positions, gaining ground, pulling ahead, the homestretch
- Keep it under 80 words, no preamble, just pure commentary
- CRITICAL: Do NOT use apostrophes, quotes, backticks, or special characters in the commentary text. Use plain ASCII only so the TTS pipe does not break.

Then do two things:
(a) Append to transcript.txt: a blank line, then a line "━━━ <current datetime> (Update #N) ━━━" where N increments each update starting from 1, then your commentary on the next line.
(b) Speak it aloud by running: echo "<your commentary here>" | kokoro-tts - --stream --voice af_heart

STEP 2B — IF THE RACE IS OVER: final wrap-up
Do these steps in order:
1. Get the final leaderboard: grep "task has finished" logs/eval.log | sed 's/.*] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
2. Get provider finish times: grep "all tasks in all configurations have finished on this provider" logs/eval.log
3. Write a final sign-off to transcript.txt: blank line, "━━━ <datetime> — FINAL ━━━", then 2-3 sentences of Vin Scully farewell commentary announcing the winner and final standings. Plain ASCII only.
4. Speak the sign-off: echo "<sign-off text>" | kokoro-tts - --stream --voice af_heart
5. Write results_summary.md with: heading "MindTrial Race Results — <datetime>", the config used, status COMPLETE, final leaderboard as a markdown table, provider finish order, notable moments (errors, close calls, speed records), and the full transcript.txt contents.
6. Speak: echo "Results summary written. What a race folks. The loop is closing. Until next time!" | kokoro-tts - --stream --voice af_heart
7. Stop this loop — the race is complete.

---

Remind the user:
- Default voice is `af_heart`. They can swap to `am_michael` (male) or `bf_emma` (British) by editing the /loop prompt.
- Default interval is 3 minutes. They can change it in the /loop command.
- For long runs, use tmux to keep the session alive: `tmux new -s mindtrial-race`
