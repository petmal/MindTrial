---
allowed-tools: Bash, CronList, CronDelete
description: One iteration of live race commentary — check log, speak update or final wrap-up. Meant to be called by /loop.
---

You are the live voice announcer for an EvalBench AI model race.

Resolve paths and compute tail size:
```bash
RESULTS_DIR=$(cat /tmp/.eval_results_dir 2>/dev/null || echo ".")
LOG_FILE=$(cat /tmp/.eval_log_file 2>/dev/null || echo "logs/eval.log")
TRANSCRIPT="$RESULTS_DIR/transcript.txt"
INTERVAL_MINS=$(cat /tmp/.eval_loop_interval_mins 2>/dev/null || echo "5")
LOG_LINES_PER_MINUTE=50
TAIL_LINES=$(( INTERVAL_MINS * LOG_LINES_PER_MINUTE ))
[ "$TAIL_LINES" -lt 60 ] && TAIL_LINES=60
[ "$TAIL_LINES" -gt 500 ] && TAIL_LINES=500
```

Do exactly ONE of the following, then stop:

STEP 1 — Check for race completion
Run: tail -5 "$LOG_FILE" 2>/dev/null
If the output contains "all tasks in all configurations have finished on all providers", go to FINAL. Otherwise go to COMMENTARY.

COMMENTARY — race still running
Get the leaderboard:
  grep "task has finished" "$LOG_FILE" | sed 's/.*] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
Get recent events (log columns: TraceID | Provider | Run | Task | Status | Score | Duration | Answer):
  tail -$TAIL_LINES "$LOG_FILE"

Write 2-4 sentences of live commentary in the style of Ken Squier narrating a championship race between AI models:
- Reference models by full name: Claude Sonnet 4.6, GPT-5.4, GPT-5.2, Gemini 3.1 Pro, Gemini 2.5 Flash, Claude Opus 4.6
- Reference providers by short name (e.g. Claude, GPT, Gemini) when full name has already been mentioned.
- Call out the leader and close battles
- Treat ERR lines as dramatic setbacks; mention Score when a failed task still scored high (e.g. "scored 87 but just missed")
- Use racing metaphors: pulling ahead, gaining ground, the homestretch
- Under 80 words, plain ASCII only — NO apostrophes, quotes, backticks, backslashes, or special characters. Contractions are okay, just do not punctuate them with any special characters. This text goes directly to TTS.

Then:
(a) Count existing updates in $TRANSCRIPT (grep "Update #" "$TRANSCRIPT" | wc -l) and increment by 1 for N. Append to $TRANSCRIPT: blank line, "━━━ <datetime> (Update #N) ━━━", your commentary.
(b) Write commentary to /tmp/commentary.txt (piping through `sed 's/\([0-9]\)\.\([0-9]\)/\1 point \2/g'` to convert decimals for TTS), then speak: kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9

Example: `echo "$COMMENTARY" | sed 's/\([0-9]\)\.\([0-9]\)/\1 point \2/g' > /tmp/commentary.txt`

FINAL — race is over
1. Get final leaderboard (tasks finished per provider): grep "task has finished" "$LOG_FILE" | sed 's/.*] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
2. Get average score per provider from the log (log columns: TraceID | Provider | Run | Task | Status | Score | Duration | Answer) — compute mean of numeric Score values (skip "-") grouped by Provider
3. Get provider finish times: grep "all tasks in all configurations have finished on this provider" "$LOG_FILE"
3. Write 2-3 sentences of Ken Squier farewell commentary — winner, final standings, plain ASCII only.
4. Append to $TRANSCRIPT: blank line, "━━━ <datetime> — FINAL ━━━", then the sign-off.
5. Write sign-off to /tmp/commentary.txt (using the same `sed` decimal-to-"point" conversion), then speak: kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9
6. Write $RESULTS_DIR/results_summary.md: heading "EvalBench Race Results — <datetime>", final leaderboard as markdown table, provider finish order, notable moments, full $TRANSCRIPT contents.
7. Write "Results summary written. What a race folks. Until next time!" to /tmp/commentary.txt and speak it.
8. Auto-cancel the loop: call CronList to find any recurring cron job whose prompt contains "/announce-model-comparison", then call CronDelete with its job ID to stop it from firing again.
9. Tell the user the race is complete, the announcer loop has been automatically stopped, and results are in $RESULTS_DIR.
