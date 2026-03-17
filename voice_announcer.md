# 🎙️ `/run-eval-suite` + `/loop` — Live Voice Announcer

## Architecture

```
/run-eval-suite
      │
      ├──► Bash: starts your eval process → logs/eval.log
      │
      └──► /loop 5m  ← native Claude Code scheduler (v2.1.71+)
                │
                Every 5 minutes, Claude wakes up and:
                ├── reads: tail logs/eval.log
                ├── calls: Anthropic API → sports commentator text
                ├── appends: transcript.txt (timestamped block)
                └── Bash: kokoro "<commentary>" --voice af_heart
```

**Key insight:** `/loop` IS the commentator. No background script, no daemon, no bash timing logic. Claude Code's native scheduler handles the tick — Claude does the reasoning and tool calls on each wake.

---

## Step 1: Install Kokoro TTS (nazdridoy/kokoro-tts)

```bash
pip install kokoro-tts

# Download the two required model files (one-time, ~350MB total)
# Run these in a stable location — e.g. ~/.config/kokoro-tts/
mkdir -p ~/.config/kokoro-tts && cd ~/.config/kokoro-tts
wget https://github.com/nazdridoy/kokoro-tts/releases/download/v1.0.0/voices-v1.0.bin
wget https://github.com/nazdridoy/kokoro-tts/releases/download/v1.0.0/kokoro-v1.0.onnx
cd -

# kokoro-tts looks for model files in the CWD by default.
# Set this env var to point it at your permanent model location:
export KOKORO_MODEL_DIR=~/.config/kokoro-tts

# Verify it works — plays audio directly to speakers
echo "We are LIVE! The eval suite has begun!" | kokoro-tts - --stream --voice af_heart
```

> **Why nazdridoy/kokoro-tts and not hexgrad/kokoro (the original)?**
> `hexgrad/kokoro` is the model inference library — Python API only, requires PyTorch.
> `nazdridoy/kokoro-tts` is a CLI wrapper that runs on ONNX Runtime (no PyTorch needed),
> with native stdin piping and `--stream` for direct speaker output. It's purpose-built
> for exactly the `echo "text" | kokoro-tts - --stream` pattern this setup relies on.

> **Recommended voices:**
> - `af_heart` — warm American female (good default)
> - `am_michael` — confident American male
> - `bf_emma` — British female (maximum BBC Sports energy)

---

## Step 2: The `/run-eval-suite` slash command

**`.claude/commands/run-eval-suite.md`**

```markdown
---
allowed-tools: Bash
description: Run the eval suite with live voice commentary via /loop every 5 minutes
---

The user wants to start the eval suite with live voice commentary.
Arguments: $ARGUMENTS (optional: custom log path, custom interval like "2m" or "10m")

Parse $ARGUMENTS:
- First arg: log file path (default: logs/eval.log)
- Second arg: loop interval (default: 5m)

Use the Bash tool to do the following:

**1. Ensure log directory and transcript exist**
```bash
mkdir -p logs
LOG_FILE="${1:-logs/eval.log}"
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
echo "━━━ Eval Suite Started: $TIMESTAMP ━━━" > transcript.txt
echo "" >> transcript.txt
echo "Transcript initialized at transcript.txt"
```

**2. Start the eval suite in the background**
```bash
# ⚠️ Replace this with your actual eval command
your-eval-command > "$LOG_FILE" 2>&1 &
EVAL_PID=$!
echo "$EVAL_PID" > /tmp/.eval_pid
echo "Eval suite started (PID: $EVAL_PID) → logging to $LOG_FILE"
```

**3. Confirm setup to the user, then instruct them:**

Tell the user:
- Eval is running (PID shown)
- Now you will set up the live voice commentator using /loop
- They should run the following command themselves to start the loop:

```
/loop 5m Read the last 80 lines of logs/eval.log. Based on what you see, write 2-3 sentences of energetic live sports commentator commentary about the eval progress — be dramatic, reference specific numbers or errors if visible, use sports metaphors. Keep it under 60 words. Then append this timestamped block to transcript.txt: a separator line with the current datetime, followed by your commentary. Then run this bash command to speak it aloud: echo "<your commentary here>" | kokoro --voice af_heart
```

Explain that /loop will wake every 5 minutes, read the log, write to transcript.txt, and speak the update live via Kokoro TTS.
```

> **Why the user runs `/loop` themselves:** Claude Code's `/loop` is an interactive command — it's invoked in the session, not spawned from a script. The slash command sets up the eval process and hands the `/loop` invocation to the user as the final step, which also makes the demo moment more explicit and intentional.

---

## The `/loop` command to run

After `/run-eval-suite` starts the eval, run this in your Claude Code session:

```
/loop 5m Do the following steps in order:

STEP 1 — CHECK FOR COMPLETION
Check if the eval suite is still running:
- Run: cat /tmp/.eval_pid | xargs ps -p 2>/dev/null
- Also check the last 10 lines of logs/eval.log for completion signals
  (e.g. "PASSED", "FAILED", "exit code", "total:", "finished", "complete")

STEP 2A — IF EVAL IS STILL RUNNING: give live commentary
Read the last 80 lines of logs/eval.log. Write 2-3 sentences of energetic
live sports commentator commentary — dramatic, reference specific numbers,
errors, or module names if visible, sports metaphors. Under 60 words, no
preamble. Append to transcript.txt: blank line, "━━━ <timestamp> (Update
#N) ━━━", then your commentary. Then run: echo "<commentary>" | kokoro
--voice af_heart

STEP 2B — IF EVAL IS DONE: wrap up and shut down the loop
1. Read the FULL logs/eval.log to understand the final results
2. Write a final closing commentary line to transcript.txt:
   "━━━ <timestamp> — FINAL ━━━" followed by a 2-sentence victory (or
   commiseration) sign-off from the commentator
3. Speak the sign-off: echo "<sign-off>" | kokoro --voice af_heart
4. Write a results_summary.md file with:
   - ## Eval Suite Results — <timestamp>
   - **Status:** PASSED / FAILED / PARTIAL
   - **Duration:** (infer from transcript.txt timestamps if possible)
   - **Key metrics:** test counts, pass rate, error summary — pull specific
     numbers from the log
   - **Notable events:** any errors, retries, or anomalies from the run
   - **Full commentary transcript:** paste the contents of transcript.txt
5. Speak: echo "Results summary written. The loop is closing. Great run!" | kokoro-tts - --stream --voice af_heart
6. Use the CronDelete tool to cancel this loop — the eval is complete
```

Adjust the voice (`af_heart`, `am_michael`, `bf_emma`) and interval (`2m`, `10m`) to taste.

---

## Step 3 (optional): `/stop-eval-suite`

**`.claude/commands/stop-eval-suite.md`**

```markdown
---
allowed-tools: Bash
description: Stop the running eval suite and finalize the transcript
---

Stop the eval suite.

Use the Bash tool to run:
```bash
if [ -f /tmp/.eval_pid ]; then
  PID=$(cat /tmp/.eval_pid)
  kill "$PID" 2>/dev/null && echo "Eval suite stopped (PID $PID)." || echo "Process not found."
  rm /tmp/.eval_pid
  TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
  echo "" >> transcript.txt
  echo "━━━ Eval Suite Stopped: $TIMESTAMP ━━━" >> transcript.txt
  echo "Transcript finalized at transcript.txt"
else
  echo "No eval PID file found."
fi
```

Tell the user to also cancel the /loop by running /loop-cancel or closing the loop in their session.
```

---

## Full session flow

```bash
# 1. Start everything
/run-eval-suite

# 2. Start the voice commentator loop (as prompted by the command output)
/loop 5m [commentator prompt — see above]

# 3. Watch the transcript live in a split pane
tail -f transcript.txt

# 4. Stop when done
/stop-eval-suite
# Then cancel /loop in your session
```

---

## Transcript output format

```
━━━ Eval Suite Started: 2026-03-16 14:30:00 ━━━

━━━ 2026-03-16 14:35:00 (Update #1) ━━━
And we are OFF! 847 test cases down with a 94.2% pass rate — this pipeline
is FLYING through fixture generation! Textbook opening from the eval suite,
absolutely no signs of slowing down!

━━━ 2026-03-16 14:40:00 (Update #2) ━━━
Uh oh — three consecutive failures on serialization. The retry logic stepped
in FAST but this could cost us critical time. The agent is holding its
composure. Every millisecond counts now!

━━━ Eval Suite Stopped: 2026-03-16 16:12:44 ━━━
```

---

## Important: `/loop` session behavior

Tasks are session-scoped — they stop when you exit Claude Code. For a multi-hour eval run:

```bash
# Keep the session alive across disconnects using tmux
tmux new -s eval-run
claude  # start Claude Code inside tmux
# Now /run-eval-suite and /loop run safely even if you close your terminal
# Reattach later with: tmux attach -t eval-run
```

---

## Dependencies

| Dependency | Install | Notes |
|---|---|---|
| `kokoro-tts` | `pip install kokoro-tts` | ONNX-based CLI wrapper — no PyTorch needed |
| Model files | `wget` (see Step 1) | `voices-v1.0.bin` + `kokoro-v1.0.onnx`, ~350MB total |
| `KOKORO_MODEL_DIR` | `export KOKORO_MODEL_DIR=~/.config/kokoro-tts` | Points kokoro-tts at your model files |
| `tmux` | `brew install tmux` / usually pre-installed | Keep session alive for long evals |
| Claude Code | v2.1.71+ | `/loop` introduced in this version |

---

## Persona swap

Change the commentator voice by editing the `/loop` prompt inline:

| Persona | Replace "energetic live sports commentator" with... |
|---|---|
| 🚀 NASA mission control | "calm NASA flight director narrating a critical mission" |
| 🍳 Gordon Ramsay | "Gordon Ramsay judging a software eval with brutal honesty" |
| 🌿 David Attenborough | "David Attenborough narrating a nature documentary about AI agents" |
| 📻 Vin Scully | "legendary baseball announcer Vin Scully calling the play-by-play" |
