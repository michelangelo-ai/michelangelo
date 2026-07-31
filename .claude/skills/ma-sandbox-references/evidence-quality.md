# Test Evidence

A test plan's value is evidence that CI can't provide: the feature working in a running environment. A reviewer should be able to reconstruct what you actually observed — not just trust your conclusion.

## Example

```markdown
## Test plan

### Integration on k3d sandbox
Image: `michelangelo-controllermgr:local` (branch `craig/revision-manager`, sha `d1046e10`)
Log level: `debug`

#### 1. PipelineRun reconciles through full lifecycle
Applied `test-revision-happy` → watched phase transitions:

​```
{"level":"info","msg":"transitioned phase","name":"test-revision-happy","from":"Pending","to":"Running"}
{"level":"info","msg":"transitioned phase","name":"test-revision-happy","from":"Running","to":"Succeeded"}
​```

Verified terminal state:
​```
$ kubectl get pipelinerun test-revision-happy -n ma-dev-test -o jsonpath='{.status.phase}'
Succeeded
​```

#### 2. Immutability enforced on completed run
Attempted spec mutation on the succeeded run — rejected as expected.

### Known gaps
- Did not test the Temporal workflow-engine path — sandbox was seeded with Cadence only.
```

This is one shape, not the only shape. Match the evidence to what the diff actually changes.

## What makes evidence useful vs hollow

**Positive evidence beats negative.** "No errors in the logs" proves the log capture worked, not that the feature worked — a silent no-op produces identical evidence. Show the feature working: phase transitions in logs, screenshots of UI interaction, state confirmed via `kubectl get`.

**One section per code path.** A single "it works" paragraph covering three branches touched by the diff tells a reviewer nothing about which were actually exercised.

**Action → evidence → verify.** Show the exact command, its raw output, and an independent check confirming the state. "Ran X, worked fine" with no output is not evidence.

**Filter to signal.** Don't paste 500 lines of raw logs. Use `capture_service_logs.sh` to show only lines relevant to the resource under test.

**Verify your environment.** Cross-check that the pod is running your build (image labels, branch/sha) before trusting results from it. A stale pod producing "passing" evidence for the wrong binary is worse than no evidence.

**Enable observability first.** `set_log_level.sh <service> debug` before exercising the feature, so the evidence includes the detail you'll need if something goes wrong.

**Named test resources.** Create resources named for the scenario (`test-<feature>-<scenario>`) rather than testing against shared demo data where failures could be leftover state.

**Show time behavior.** A single terminal-state snapshot tells you nothing about whether intermediate states were correct. Show the transitions.

**Show the full lifecycle.** Exercise every path the diff touches — not just the happy-path create, but delete, error handling, immutability enforcement if those are in the diff.

**Flag gaps honestly.** An untested path noted is a known risk. An untested path left silent is a surprise later.
