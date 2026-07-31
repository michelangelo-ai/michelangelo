---
name: ma-sandbox-test-plan
description: >-
  Build, test, and verify a sandbox change across Go, JS, and Python.
  Use when the user asks to verify a change or generate evidence that a feature works.
argument-hint: <description of what changed>
user-invocable: true
---

A pipeline that gates on failure at every stage. The output of each stage IS the evidence —
don't summarize it away. See `.claude/skills/ma-sandbox-references/evidence-quality.md` for what makes evidence good vs. hollow.

## Stage 0: Detect stack

```bash
CHANGED_FILES=$(git diff --name-only origin/main...HEAD)
```

Categorize by path prefix: `go/` → Go, `javascript/` → JS, `python/` → Python. Mixed changes run every
applicable stage below, once per stack, each gated independently — a Go build failure doesn't block
the JS pipeline from also reporting its own result.

## Stage 1: Build + lint (gate)

| Stack | Commands |
|-------|----------|
| Go | `go build ./cmd/<service>/...` for each changed `go/cmd/<service>/`; `go vet ./go/components/...` |
| JS | `yarn typecheck`, `yarn lint` |
| Python | `poetry check` |

**On any failure: stop that stack's pipeline, report the command and full error output, and do not
proceed to Stage 2 for that stack.** Other stacks (in a mixed change) still run independently.

## Stage 2: Unit tests (gate)

Scope to the packages/dirs actually touched — derive this from Stage 0's `$CHANGED_FILES` directly,
there's no fixed mapping table to maintain:

| Stack | Command |
|-------|---------|
| Go | `go test ./<dir>/...` for each unique directory containing a changed `.go` file under `go/components/` or `go/cmd/` |
| JS | `yarn test <changed-dir-glob>` (this repo runs `vitest --run`, which accepts path filters) |
| Python | `pytest <changed-dir>` |

**On failure: stop, report the failing test output, do not proceed to Stage 3 for that stack.**

## Stage 3: Integration setup

Only reached for stacks whose Stage 1–2 passed.

**Go backend:**
```bash
bash $(git rev-parse --show-toplevel)/.claude/skills/ma-sandbox-scripts/check-backend.sh
```
| Output | Action |
|--------|--------|
| `clean` | No local Go changes deployed — skip Go integration scenarios (Stage 4 Go section), note this in "Known gaps" |
| `deployed <service>` | Enable debug logging before capturing any evidence: `bash $(git rev-parse --show-toplevel)/.claude/skills/ma-sandbox-scripts/set_log_level.sh <service> debug` |
| `needs-deploy <service>` | **Stop and respond**: "You have local changes to `<service>` that aren't deployed. Run `/ma-sandbox-deploy` first, then re-run `/ma-sandbox-test-plan`." Do not proceed for this stack. |

**JavaScript UI:**
```bash
VITE_INFO=$(bash $(git rev-parse --show-toplevel)/.claude/skills/ma-sandbox-scripts/find-vite-port.sh)
VITE_PORT=$(echo "$VITE_INFO" | awk '{print $1}')
VITE_PID=$(echo "$VITE_INFO" | awk '{print $2}')
VITE_MODE=$(echo "$VITE_INFO" | awk '{print $3}')  # "existing" or "started"
```
- Exit 0 → use `$VITE_PORT` in place of `5173` in all URLs below. Record `$VITE_MODE`/`$VITE_PID` for cleanup.
- Non-zero exit → **stop immediately and report the failure.** Do not fall back to guessing a port via curl/lsof — a successful response doesn't confirm which branch is being served.
- If the Go backend is *also* deployed (Stage 3 said `deployed <service>`), use the full sandbox at `http://localhost:8090` instead of Vite, and skip straight to Stage 4.

**Python:**
```bash
poetry run ma sandbox health
```
Confirm the services relevant to the change are running.

## Stage 4: Integration scenarios

Before running scenarios, list the code paths identified from the diff and note any paths skipped — this list becomes the "Known gaps" input for Stage 5.

### Go

For each controller/reconcile path touched by the diff:

1. Apply a named test CR (`test-<feature>-<scenario>` — see `.claude/skills/ma-sandbox-references/sandbox-data.md` for demo-data
   conventions; prefer creating a new entity over disturbing shared demo state, per evidence principle 7).
2. Capture logs scoped to that resource:
   ```bash
   bash $(git rev-parse --show-toplevel)/.claude/skills/ma-sandbox-scripts/capture_service_logs.sh <service> --resource <test-name>
   ```
3. Verify state independently via `kubectl get`/`kubectl describe` — don't rely on logs alone.
4. Show the full lifecycle: creation, intermediate phase transitions, terminal state, and (if the diff
   touches them) immutability enforcement or garbage collection. See `.claude/skills/ma-sandbox-references/evidence-quality.md` #8–9.

### JavaScript

1. **Playwright setup.** Use `playwright-1`. Clean up stale screenshots: `rm -f .claude/ma-sandbox-test-plan-*.png`
2. **Derive the route.** Read `.claude/skills/ma-sandbox-references/routes.md` for the phase/entity keyword tables and resolution
   algorithm. Resolve phase and entity as independent signals, then combine. No matching row → stop and
   report, don't guess.
3. **Navigate and check preconditions.**
   - `browser_snapshot` to confirm the page loaded (no error boundary, no blank state) — stop and report if it didn't.
   - Read `.claude/skills/ma-sandbox-references/sandbox-data.md` for what state each feature needs. If the required entity/state is
     missing, create it (prefer the UI over a full re-seed) and document every setup step taken.
   - Do not use yab/curl/direct API calls for setup — `kubectl` for inspection, UI or `ma` CLI for creation.
4. **Exercise the feature** — click, fill, trigger the interactions the description calls for.
5. **Screenshot after each meaningful state:** `browser_take_screenshot` → `.claude/ma-sandbox-test-plan-{state-name}.png`.
6. **Check console errors:** `browser_console_messages(level: "error")`. Classify:

   | Classification | Matches |
   |---------------|---------|
   | **Blocking** | `Uncaught`, `TypeError`, `ReferenceError`, `Failed to fetch`, `Cannot read properties`, React render error |
   | **Non-blocking** | `deprecated`, `404 /favicon`, `CORS preflight`, `OPTIONS`, `sourceMap`, `net::ERR_ABORTED` for known-missing mock data |

   Anything matching neither list: treat as **Blocking** (unknown = assume blocking).

### Python

1. Run the relevant CLI commands or `kubectl apply` the relevant resources.
2. Capture service logs (`capture_service_logs.sh`) and CLI stdout/stderr.
3. Verify state via `kubectl get`.

## Stage 5: Report

Print the integration evidence following `.claude/skills/ma-sandbox-references/evidence-quality.md`'s guidance.
Build/lint/unit-test results from stages 1–2 are gates, not evidence — don't include them (CI covers that).

## Cleanup

- **If Vite was started by this run** (`$VITE_MODE` = `started`): ask "I started a Vite dev server (PID
  `$VITE_PID`) for this session. Kill it now?" If yes: `kill "$VITE_PID"`. If `$VITE_MODE` = `existing`,
  leave it alone.
- **If log level was changed in Stage 3:** ask whether to revert — `bash $(git rev-parse --show-toplevel)/.claude/skills/ma-sandbox-scripts/set_log_level.sh <service> info`.
- **Test CRs left in the cluster:** list them in the report. Don't auto-delete — the user may want to
  inspect them further or reuse them for a follow-up run.
