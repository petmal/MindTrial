---
allowed-tools: Bash, AskUserQuestion
description: Build and run MindTrial eval suite with live voice race commentary
---

The user wants to start a MindTrial eval race with live voice commentary.
Arguments: $ARGUMENTS (optional: config file, loop interval)

Parse $ARGUMENTS:
- First arg: config file path (optional — if omitted, prompt interactively)
- Second arg: loop interval in minutes (default: 5)

**Step 0 — Pick a config (interactive if no first arg)**

If the first arg is NOT provided, use AskUserQuestion to ask:
"Which eval config do you want to race?"
With options:
1. `config-eval-top3-cicd.yaml` — CI/CD & DevOps tasks (CircleCI, Docker, Kubernetes, secrets, pipelines) — 15 model configs across 3 providers
2. `config-eval-top3.yaml` — General software engineering tasks (coding, debugging, architecture, APIs) — 15 model configs across 3 providers

Use their answer as the config file path before continuing.

If the first arg IS provided, use it directly as the config file path.

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
echo "${2:-5}" > /tmp/.eval_loop_interval_mins
cat > "$RESULTS_DIR/transcript.txt" << HEADER
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  MODEL COMPARISON — LIVE COMMENTARY TRANSCRIPT
  Started: $TIMESTAMP
  Config:  $CONFIG_FILE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

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
LOG_FILE=$(cat /tmp/.eval_log_file)
RESULTS_DIR=$(cat /tmp/.eval_results_dir)
nohup ./mindtrial -config "$CONFIG_FILE" -log "$LOG_FILE" -output-dir "$RESULTS_DIR" -output-basename "results" -verbose run > /dev/null 2>&1 &
EVAL_PID=$!
echo "$EVAL_PID" > /tmp/.eval_pid
echo "MindTrial eval started (PID: $EVAL_PID)"
echo "Config: $CONFIG_FILE"
echo "Log:    $LOG_FILE"
```

**4. Wait for providers to initialize, then show opening lines**
```bash
sleep 5
head -20 "$(cat /tmp/.eval_log_file)" 2>/dev/null || echo "(log not yet available)"
```

**5. Speak the race start announcement**
```bash
LOG_FILE=$(cat /tmp/.eval_log_file)
NUM_PROVIDERS=$(grep -c "starting [0-9]* tasks on this provider" "$LOG_FILE" 2>/dev/null || echo 3)
NUM_CONFIGS=$(grep "starting [0-9]* tasks on this provider" "$LOG_FILE" 2>/dev/null | grep -o "in [0-9]* configurations" | awk '{sum+=$2} END {print sum}')
NUM_CONFIGS=${NUM_CONFIGS:-6}
echo "Ladies and gentlemen, welcome to MindTrial! ${NUM_CONFIGS} model configurations across ${NUM_PROVIDERS} providers are lined up, the tasks are loaded, and we are LIVE. Let the race begin!" > /tmp/commentary.txt && kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9 2>/dev/null || echo "(kokoro-tts not available)"
```

**6. Tell the user to start the announcer loop**

Tell the user to start the live voice announcer by running:

```
/loop 5m /announce-model-comparison
```

(They can adjust the interval — e.g. `2m` or `10m`.)

**7. Tell the user the race is running**

Tell the user:
- The eval is running (show PID, config, log path)
- Run `/loop 5m /announce-model-comparison` to start the live voice announcer
- Watch live: `tail -f <RESULTS_DIR>/transcript.txt`
- Stop everything: `/run stop-model-comparison`
