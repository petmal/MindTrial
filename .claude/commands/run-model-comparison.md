---
allowed-tools: Bash, AskUserQuestion, CronCreate
description: Build and run EvalBench eval suite, with optional live voice race commentary
---

The user wants to start an EvalBench eval race.
Arguments: $ARGUMENTS (optional: config file, loop interval, `with-announcer`)

Parse $ARGUMENTS:
- First arg that ends in `.yaml`: config file path (optional — if omitted, prompt interactively)
- Any arg equal to `with-announcer`: enable voice announcer
- A numeric arg immediately following `with-announcer` in the arg list: announcer loop interval in minutes (default: 5 if not specified)

**Step 0 — Pick a config (interactive if no first arg)**

If the first arg is NOT provided, use AskUserQuestion to ask:
"Which eval config do you want to race?"
With options:
1. `config-eval-top3-cicd.yaml` — CI/CD & DevOps tasks (CircleCI, Docker, Kubernetes, secrets, pipelines) — 15 model configs across 3 providers
2. `config-eval-top3.yaml` — General software engineering tasks (coding, debugging, architecture, APIs) — 15 model configs across 3 providers

Use their answer as the config file path before continuing.

If the first arg IS provided, use it directly as the config file path.

**Step 0b — Determine announcer mode**

If `with-announcer` is NOT present in $ARGUMENTS, use AskUserQuestion to ask:
"Want live voice commentary for this race?"
With options:
1. Yes — enable the voice announcer (requires kokoro-tts)
2. No — run silently (no TTS, no announcer loop)

Set `ANNOUNCER_ENABLED` to `true` or `false` based on the flag or the user's answer before continuing.

If `ANNOUNCER_ENABLED` is `true` AND no interval was specified in $ARGUMENTS, use AskUserQuestion to ask:
"How often should the announcer check in?"
With options:
1. Every 1 minute
2. Every 2 minutes
3. Every 5 minutes (default)
4. Every 10 minutes

Set `ANNOUNCER_INTERVAL` to the chosen number of minutes (default: 5 if skipped or unspecified).

Use the Bash tool to do the following steps:

**1. Ensure log directory and transcript exist**
```bash
CONFIG_FILE="${1:-config-eval-top3-cicd.yaml}"
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
RESULTS_DIR="results/eval/$(date '+%Y-%m-%d')/$(date '+%H-%M-%S')"
LOG_FILE="$RESULTS_DIR/eval.log"
mkdir -p "$RESULTS_DIR"
echo "$RESULTS_DIR" > /tmp/.eval_results_dir
echo "$LOG_FILE" > /tmp/.eval_log_file
echo "${ANNOUNCER_INTERVAL:-5}" > /tmp/.eval_loop_interval_mins
cat > "$RESULTS_DIR/transcript.txt" << HEADER
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  MODEL COMPARISON — LIVE COMMENTARY TRANSCRIPT
  Started: $TIMESTAMP
  Config:  $CONFIG_FILE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

HEADER
echo "Transcript initialized at $RESULTS_DIR/transcript.txt"
```

**2. Build EvalBench**
```bash
echo "Building EvalBench..."
go build -o evalbench ./cmd/evalbench/
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
LOG_FILE=$(cat /tmp/.eval_log_file)
RESULTS_DIR=$(cat /tmp/.eval_results_dir)
nohup ./evalbench -config "$CONFIG_FILE" -log "$LOG_FILE" -output-dir "$RESULTS_DIR" -output-basename "results" -verbose run > /dev/null 2>&1 &
EVAL_PID=$!
echo "$EVAL_PID" > /tmp/.eval_pid
echo "EvalBench eval started (PID: $EVAL_PID)"
echo "Config: $CONFIG_FILE"
echo "Log:    $LOG_FILE"
```

**4. Wait for providers to initialize, then show opening lines**
```bash
sleep 5
head -20 "$(cat /tmp/.eval_log_file)" 2>/dev/null || echo "(log not yet available)"
```

**5. Open tail windows via watch script**
```bash
bash scripts/open-dashboard.sh
```

**6. (ANNOUNCER ONLY) Speak the race start announcement**

Skip this step entirely if `ANNOUNCER_ENABLED` is `false`.

```bash
LOG_FILE=$(cat /tmp/.eval_log_file)
NUM_PROVIDERS=$(grep -c "starting [0-9]* tasks on this provider" "$LOG_FILE" 2>/dev/null || echo 3)
NUM_CONFIGS=$(grep "starting [0-9]* tasks on this provider" "$LOG_FILE" 2>/dev/null | grep -o "in [0-9]* configurations" | awk '{sum+=$2} END {print sum}')
NUM_CONFIGS=${NUM_CONFIGS:-6}
echo "Ladies and gentlemen, welcome to EvalBench! ${NUM_CONFIGS} model configurations across ${NUM_PROVIDERS} providers are lined up, the tasks are loaded, and we are LIVE. Let the race begin!" > /tmp/commentary.txt && kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9 2>/dev/null || echo "(kokoro-tts not available)"
```

**7. (ANNOUNCER ONLY) Auto-start the announcer loop**

Skip this step entirely if `ANNOUNCER_ENABLED` is `false`.

Use the CronCreate tool to schedule the announcer automatically:
- `cron`: `*/${ANNOUNCER_INTERVAL:-5} * * * *` (every N minutes, using the parsed interval)
- `prompt`: `/announce-model-comparison`
- `recurring`: `true`

Save the returned job ID to show the user.

**8. Tell the user the race is running**

Tell the user:
- The eval is running (show PID, config, log path)
- If announcer is enabled: the announcer loop is running (show job ID, cancel with CronDelete if needed)
- If announcer is disabled: mention they can re-run with `with-announcer` to enable voice commentary, or manually kick off commentary at any time with `/loop 5m /announce-model-comparison`
- Watch live:
  - Transcript: `tail -f <RESULTS_DIR>/transcript.txt`
  - Log: `tail -f <LOG_FILE>`
- Stop everything: `/stop-model-comparison`
