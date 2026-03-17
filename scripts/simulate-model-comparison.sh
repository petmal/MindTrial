#!/usr/bin/env python3
"""
Simulates a MindTrial eval race by writing realistic zerolog output.
Same log format as the real runner — the voice announcer can't tell the difference.

Usage: python3 scripts/simulate-race.sh [log_file] [num_tasks] [speed]
  log_file  — output path (default: logs/eval.log)
  num_tasks — tasks per model config (default: 15)
  speed     — simulation speed multiplier (default: 1, higher = faster)
"""

import sys
import os
import time
import random
import string
from datetime import datetime

LOG_FILE = sys.argv[1] if len(sys.argv) > 1 else "logs/eval.log"
NUM_TASKS = int(sys.argv[2]) if len(sys.argv) > 2 else 15
SPEED = float(sys.argv[3]) if len(sys.argv) > 3 else 1.0

os.makedirs(os.path.dirname(LOG_FILE) or ".", exist_ok=True)

RUNS = [
    ("openai",    "GPT-5.4 (high reasoning)",              "gpt-5.4",          10, (20, 35)),
    ("openai",    "GPT-5.2 (high reasoning)",              "gpt-5.2",          20, (15, 27)),
    ("google",    "Gemini 3.1 Pro (high thinking)",        "gemini-3.1-pro",    3, (10, 20)),
    ("google",    "Gemini 2.5 Flash",                      "gemini-2.5-flash", 10, (5, 13)),
    ("anthropic", "Claude Opus 4.6 (extended thinking)",   "claude-opus-4-6",  10, (18, 32)),
    ("anthropic", "Claude Sonnet 4.6 (extended thinking)", "claude-sonnet-4-6", 10, (12, 22)),
]

TASKS = [
    "reasoning - section, color and number - v1",
    "reasoning - bridge crossing - v1",
    "reasoning - logic grid puzzle - v1",
    "shell - pipeline filtering - v1",
    "shell - process management - v1",
    "docker - multi-stage build - v1",
    "circleci - orb usage - v1",
    "circleci - workflow dependencies - v1",
    "semver - version bump - v1",
    "yaml - config parsing - v1",
    "git - merge conflict - v1",
    "k8s - deployment rollout - v1",
    "terraform - state management - v1",
    "ansible - playbook structure - v1",
    "monitoring - alert rules - v1",
    "networking - dns resolution - v1",
    "security - certificate chain - v1",
    "cicd - artifact caching - v1",
    "scripting - error handling - v1",
    "debugging - log analysis - v1",
    "circleci - parallelism config - v1",
    "docker - compose networking - v1",
    "git - rebase strategy - v1",
    "k8s - resource limits - v1",
    "terraform - module structure - v1",
    "shell - cron scheduling - v1",
    "cicd - environment variables - v1",
    "monitoring - dashboard config - v1",
    "networking - load balancing - v1",
    "security - secret rotation - v1",
]

task_count = min(NUM_TASKS, len(TASKS))
providers = sorted(set(r[0] for r in RUNS))

def ts():
    return datetime.now().strftime("%Y-%m-%d %H:%M:%S")

def ulid():
    return ''.join(random.choices(string.ascii_uppercase + string.digits, k=26))

def log(line):
    with open(LOG_FILE, "a") as f:
        f.write(line + "\n")
        f.flush()

# Clear log
with open(LOG_FILE, "w") as f:
    pass

log(f"{ts()} INF starting {task_count} tasks on {len(providers)} providers...")

for provider in providers:
    configs = sum(1 for r in RUNS if r[0] == provider)
    log(f"{ts()} INF {provider}: starting {task_count} tasks on this provider in {configs} configurations...")

for provider, run_name, _, rpm, _ in RUNS:
    log(f"{ts()} INF {provider}: {run_name}: request rate limited to {rpm} requests/min.")

print(f"=== Simulation: {task_count} tasks x {len(RUNS)} configs, speed={SPEED}x ===", file=sys.stderr)

race_start = time.time()

run_state = {run_name: 0 for _, run_name, *_ in RUNS}
provider_done = {p: False for p in providers}

while any(idx < task_count for idx in run_state.values()):
    for provider, run_name, model, rpm, (lo, hi) in RUNS:
        idx = run_state[run_name]
        if idx >= task_count:
            continue

        task = TASKS[idx]
        trace = ulid()

        log(f"{ts()} INF [{trace}] {provider}: {run_name}: {task}: starting task...")

        base_time = random.uniform(lo, hi)
        wait = base_time / (10 * SPEED)
        time.sleep(wait)

        duration = f"{base_time:.6f}s"

        if random.random() < 0.08:
            log(f"{ts()} ERR [{trace}] {provider}: {run_name}: {task}: task finished with error error=\"API rate limit exceeded (429)\"")

        log(f"{ts()} INF [{trace}] {provider}: {run_name}: {task}: task has finished in {duration}.")

        run_state[run_name] = idx + 1

        if idx + 1 >= task_count:
            all_done_for_provider = all(
                run_state[rn] >= task_count
                for p, rn, *_ in RUNS if p == provider
            )
            if all_done_for_provider and not provider_done[provider]:
                elapsed = time.time() - race_start
                log(f"{ts()} INF {provider}: all tasks in all configurations have finished on this provider in {elapsed:.0f}s.")
                provider_done[provider] = True

total = time.time() - race_start
log(f"{ts()} INF all tasks in all configurations have finished on all providers in {total:.0f}s.")
print(f"=== Simulation complete. Log written to {LOG_FILE} ===", file=sys.stderr)
