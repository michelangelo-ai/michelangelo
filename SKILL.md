# Michelangelo Pipeline Debugging Skill

## Description
This skill helps debug Michelangelo pipeline runs, especially focusing on retry logic, actor failures, and condition engine issues. It provides structured approaches to investigate pipeline failures and understand the retry mechanism.

## Key Capabilities
- Analyze pipeline run retry logic and actor failure patterns
- Debug condition engine behavior and terminal states
- Investigate infinite requeuing issues
- Trace error propagation through pipeline steps
- Examine Bazel build issues related to pipeline components

## Common Debugging Workflows

### 1. Pipeline Retry Logic Investigation
When investigating retry behavior:
1. Check the `maxRetryAttempts` setting in `go/base/conditions/engine/defaultengine.go`
2. Look for retry attempt logs: `"running actor" attempt=X maxAttempts=Y`
3. Verify error propagation from actor to engine to controller
4. Check if terminal conditions are being set properly

### 2. Infinite Requeuing Issues
When pipelines keep retrying indefinitely:
1. Look for terminal condition checks in actors (e.g., `"pipeline run has a terminal condition, skipping"`)
2. Verify controller sets proper terminal conditions for failed actors
3. Check that `ctrl.Result{Requeue: false}` is being returned
4. Ensure pipeline state changes are persisted correctly

### 3. Error Message Propagation
When error messages aren't showing in pipeline steps:
1. Check if actor updates step state and message on failure
2. Verify controller propagates errors to step status
3. Look for step state updates: `executeWorkflowStep.State = FAILED`
4. Ensure error messages are set: `executeWorkflowStep.Message = "error details"`

### 4. Actor-Specific Debugging

#### ExecuteWorkflowActor
- Check blob store fetch errors in `StartWorkflow()`
- Verify workflow client configuration and task list setup
- Look for terminal condition checks in `Run()` method
- Monitor workflow execution status updates

#### Engine Retry Logic
- Located in `go/base/conditions/engine/defaultengine.go`
- Key method: `runActorWithRetry()`
- Check retry metadata creation and retrieval
- Verify error vs condition handling logic

### 5. Build and Deployment Issues
When changes aren't taking effect:
1. Check Bazel BUILD.bazel files for missing dependencies
2. Verify imports match dependency declarations
3. Ensure controller restarts with new code (`bazel run controllermgr`)
4. Look for compilation errors in build output

## Key Files to Examine
- `go/components/pipelinerun/actors/executeworkflow.go` - Main workflow execution logic
- `go/components/pipelinerun/controller.go` - Pipeline reconciliation and state management
- `go/base/conditions/engine/defaultengine.go` - Retry logic and condition engine
- `go/components/pipelinerun/BUILD.bazel` - Build dependencies
- `go/components/pipelinerun/actors/utils/` - Utility functions for pipeline runs

## Debugging Commands
Useful commands for investigation:
- `bazel run //go/cmd/controllermgr:controllermgr` - Run controller locally
- `kubectl logs -f <controller-pod>` - Monitor controller logs
- `kubectl get pipelinerun <name> -o yaml` - Inspect pipeline run status
- `bazel clean && bazel run controllermgr` - Clean rebuild and restart

## Log Patterns to Look For
- `"running actor" actor="Execute Workflow" attempt=X` - Retry attempts
- `"actor execution failed"` - Actor failures
- `"error running actor with retry"` - Retry exhaustion
- `"pipeline run has a terminal condition, skipping"` - Terminal condition checks
- `"Failed to run engine"` - Engine-level failures
- `"UPDATED CONTROLLER"` - Custom debug messages (if added)

## Common Issues and Solutions
1. **Infinite retrying**: Add terminal condition for failed actors
2. **Missing error messages**: Update step state and message in controller
3. **Build failures**: Check BUILD.bazel dependencies match imports
4. **Old code running**: Restart controller with `bazel run`

## Best Practices
- Add debug log messages when debugging complex issues
- Use structured approach: actor → engine → controller → step status
- Check both condition status and pipeline state
- Verify error propagation at each level
- Test with invalid configurations to trigger failure paths