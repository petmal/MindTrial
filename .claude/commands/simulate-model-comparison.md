---
allowed-tools: Bash
description: Simulate a MindTrial eval race (no API tokens) with live voice commentary via /loop
---

The user wants to simulate a MindTrial eval race without making real API calls.
This uses scripts/simulate-model-comparison.sh to generate realistic log output that the voice announcer can narrate.
Arguments: $ARGUMENTS (optional: num_tasks, speed multiplier, loop interval)

Parse $ARGUMENTS:
- First arg: number of tasks per model (default: 20)
- Second arg: speed multiplier (default: 1, real-time pacing)
- Third arg: loop interval (default: 1m — shorter since simulation is compressed)

Use the Bash tool to do the following steps:

**1. Initialize transcript**
```bash
LOG_FILE="logs/eval.log"
mkdir -p logs
TIMESTAMP=$(date '+%Y-%m-%d %H:%M:%S')
cat > transcript.txt << HEADER
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  MINDTRIAL RACE — LIVE COMMENTARY TRANSCRIPT
  Started: $TIMESTAMP
  Mode:    SIMULATION (no API calls)
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

HEADER
echo "Transcript initialized at transcript.txt"
```

**2. Start the simulation in the background**
```bash
NUM_TASKS="${1:-20}"
SPEED="${2:-1}"
nohup python3 scripts/simulate-model-comparison.sh logs/eval.log "$NUM_TASKS" "$SPEED" > /dev/null 2>&1 &
SIM_PID=$!
echo "$SIM_PID" > /tmp/.eval_pid
echo "Simulation started (PID: $SIM_PID)"
echo "Tasks per model: $NUM_TASKS"
echo "Speed: ${SPEED}x"
echo "Log: logs/eval.log"
```

**3. Wait a moment for the simulation to begin, then show opening lines**
```bash
sleep 2
head -15 logs/eval.log 2>/dev/null || echo "(log not yet available)"
```

**4. Speak the race start announcement**
```bash
echo "Ladies and gentlemen, welcome to a MindTrial simulation! Six model configurations are about to go head to head. No real tokens, all the drama. Let the race begin!" | kokoro-tts - --stream --voice af_heart 2>/dev/null || echo "(kokoro-tts not available)"
```

**5. Confirm setup and give the user the /loop command**

Tell the user:
- The simulation is running — generating realistic MindTrial log output
- 6 model configs racing: GPT-5.4, GPT-5.2, Gemini 3.1 Pro, Gemini 2.5 Flash, Claude Opus 4.6, Claude Sonnet 4.6
- Now they should start the voice commentator with the /loop command below
- Watch live: `tail -f transcript.txt` in a split pane
- Stop: `/run stop-model-comparison`
- Since simulation runs faster than real evals, the loop interval is 1 minute by default

Give the user this exact /loop command to copy-paste (adjust interval from args):

---

/loop 1m Do the following steps in order using Bash. The MindTrial eval log is at logs/eval.log.

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
5. Write results_summary.md with: heading "MindTrial Race Results — <datetime>", mode SIMULATION, final leaderboard as a markdown table, provider finish order, notable moments (errors, close calls, speed records), and the full transcript.txt contents.
6. Speak: echo "Results summary written. What a race folks. The loop is closing. Until next time!" | kokoro-tts - --stream --voice af_heart
7. Stop this loop — the race is complete.

---

Remind the user:
- This is a simulation — no API tokens are spent
- Default is 20 tasks at 1x speed (~3-4 minutes), giving the announcer several updates
- They can speed it up: `/run simulate-model-comparison 10 10` for a quick sprint
