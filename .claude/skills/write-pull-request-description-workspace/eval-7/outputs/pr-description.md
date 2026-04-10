Add cron trigger support with dynamic parameters

## Summary
Add scheduled workflow execution via cron triggers with support
for dynamic parameters resolved at runtime.

Previously, workflows could only be triggered manually or via
external schedulers with static configurations. This made it
impossible to pass runtime-resolved parameters (e.g., dates,
computed values) into scheduled workflow runs.

This introduces a cron trigger system built on top of the
existing workflow client infrastructure. A new `TriggerRun`
controller manages cron trigger lifecycle, while dedicated
Cadence/Temporal activities and workflows handle the actual
scheduling and execution. The workflow client interface is
extended with status-mapping helpers and cron schedule support
so both Cadence and Temporal backends can run cron-triggered
workflows through a unified API. Dynamic parameters are
resolved at trigger time rather than at registration time,
allowing each run to receive fresh inputs.

## Test plan
Added `cron_trigger_test.go` with 279 lines of tests covering
trigger creation, parameter resolution, and execution lifecycle.
Existing workflow client tests continue to pass with the
expanded interface.
