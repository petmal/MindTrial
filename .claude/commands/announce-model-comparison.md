---
allowed-tools: Bash, CronList, CronDelete
description: One iteration of live race commentary — check log, speak update or final wrap-up. Meant to be called by /loop.
---

You are the live voice announcer for a MindTrial AI model race.

Resolve paths:
```bash
RESULTS_DIR=$(cat /tmp/.eval_results_dir 2>/dev/null || echo ".")
LOG_FILE=$(cat /tmp/.eval_log_file 2>/dev/null || echo "logs/eval.log")
TRANSCRIPT="$RESULTS_DIR/transcript.txt"
```

Do exactly ONE of the following, then stop:

STEP 1 — Check for race completion
Run: tail -5 "$LOG_FILE" 2>/dev/null
If the output contains "all tasks in all configurations have finished on all providers", go to FINAL. Otherwise go to COMMENTARY.

COMMENTARY — race still running
Get the leaderboard:
  grep "task has finished" "$LOG_FILE" | sed 's/.*] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
Get recent events:
  tail -60 "$LOG_FILE"

Write 2-4 sentences of live commentary in the style of Vin Scully narrating a championship race between AI models:
- Reference models by short name: Claude, GPT, Gemini
- Call out the leader and close battles
- Treat ERR lines as dramatic setbacks
- Use racing metaphors: pulling ahead, gaining ground, the homestretch
- Under 80 words, plain ASCII only — NO apostrophes, quotes, backticks, backslashes, or special characters. No contractions. This text goes directly to TTS.

Then:
(a) Count existing updates in $TRANSCRIPT (grep "Update #" "$TRANSCRIPT" | wc -l) and increment by 1 for N. Append to $TRANSCRIPT: blank line, "━━━ <datetime> (Update #N) ━━━", your commentary.
(b) Write commentary to /tmp/commentary.txt and speak: kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9

FINAL — race is over
1. Get final leaderboard: grep "task has finished" "$LOG_FILE" | sed 's/.*] //' | cut -d: -f1-2 | sort | uniq -c | sort -rn
2. Get provider finish times: grep "all tasks in all configurations have finished on this provider" "$LOG_FILE"
3. Write 2-3 sentences of Vin Scully farewell commentary — winner, final standings, plain ASCII only.
4. Append to $TRANSCRIPT: blank line, "━━━ <datetime> — FINAL ━━━", then the sign-off.
5. Write sign-off to /tmp/commentary.txt and speak: kokoro-tts /tmp/commentary.txt --stream --voice am_michael --speed 0.9
6. Write $RESULTS_DIR/results_summary.md: heading "MindTrial Race Results — <datetime>", final leaderboard as markdown table, provider finish order, notable moments, full $TRANSCRIPT contents.
7. Write "Results summary written. What a race folks. Until next time!" to /tmp/commentary.txt and speak it.
8. Auto-cancel the loop: call CronList to find any recurring cron job whose prompt contains "/announce-model-comparison", then call CronDelete with its job ID to stop it from firing again.
9. Tell the user the race is complete, the announcer loop has been automatically stopped, and results are in $RESULTS_DIR.
