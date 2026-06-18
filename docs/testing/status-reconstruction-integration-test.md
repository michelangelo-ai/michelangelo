# PipelineRun Status Reconstruction Integration Test

## Overview

This document describes the integration test for verifying that the PipelineRun controller can successfully reconstruct status from Cadence/Temporal when status is lost due to etcd corruption, TTL eviction, or controller restarts.

## Feature Background

The status reconstruction feature was added to handle disaster recovery scenarios where PipelineRun status is lost but the workflow execution is still running in Cadence/Temporal. When the controller reconciles a PipelineRun with missing `WorkflowId` or `WorkflowRunId` fields, it queries the workflow engine using the PipelineRun name as the workflow ID and reconstructs:
- `status.WorkflowId`
- `status.WorkflowRunId`  
- `status.State` (mapped from workflow execution status)

## Known Limitations

### MySQL UID vs Workflow ID Mismatch

**Issue**: If a PipelineRun is deleted and recreated with the same name in a new Kubernetes cluster:
- The new PipelineRun will have a **different K8s UID** (UIDs are unique per cluster/object)
- But the **workflow ID remains the same** (workflow ID = PipelineRun name)
- MySQL uses K8s UID as the primary key
- Workflow engine uses PipelineRun name as workflow ID

**Result**: Status reconstruction will work (finds workflow by name), but MySQL will have orphaned records from the old UID.

**Workaround**: 
- Avoid recreating PipelineRuns with the same name across clusters
- If migration is necessary, use a different name or ensure workflows are terminated first
- Future enhancement: Add finalizers to prevent deletion while workflow is running

## Test Scenarios

### Scenario 1: Status Reconstruction After etcd Data Loss

**Setup:**
1. Create a PipelineRun that successfully starts a workflow
2. Wait for workflow to be running in Cadence/Temporal
3. Simulate status loss by directly patching the PipelineRun to remove status fields
4. Trigger a reconciliation

**Expected Result:**
- Controller reconstructs status from workflow engine
- `WorkflowId` and `WorkflowRunId` are restored
- `State` matches the workflow execution status
- Pipeline continues running normally

**Verification:**
```bash
# Check that status was reconstructed
kubectl get pipelinerun <name> -n <namespace> -o jsonpath='{.status.workflowId}'
kubectl get pipelinerun <name> -n <namespace> -o jsonpath='{.status.workflowRunId}'
kubectl get pipelinerun <name> -n <namespace> -o jsonpath='{.status.state}'
```

### Scenario 2: Status Reconstruction for Completed Workflow

**Setup:**
1. Create a PipelineRun that runs to completion
2. Wait for workflow to complete in Cadence/Temporal
3. Simulate status loss by patching the PipelineRun
4. Trigger a reconciliation

**Expected Result:**
- Controller reconstructs status showing SUCCEEDED state
- `WorkflowId` and `WorkflowRunId` are restored
- `State` is PIPELINE_RUN_STATE_SUCCEEDED

### Scenario 3: No Reconstruction for New PipelineRun

**Setup:**
1. Create a new PipelineRun that hasn't started yet
2. Verify no workflow exists in Cadence/Temporal

**Expected Result:**
- Controller attempts reconstruction but finds no workflow
- Reconstruction fails silently (logged as debug)
- Reconciliation continues normally
- ExecuteWorkflow actor starts the workflow

### Scenario 4: Status Reconstruction with Retry

**Setup:**
1. Create a PipelineRun with RetryInfo pointing to a failed run
2. Wait for workflow reset to complete
3. Simulate status loss
4. Trigger a reconciliation

**Expected Result:**
- Controller reconstructs status showing the NEW run ID (after reset)
- Retry detection works correctly (new run ID ≠ retry target run ID)
- Prevents duplicate retry

### Scenario 5: Recreated PipelineRun with Same Name (Edge Case)

**Setup:**
1. Create a PipelineRun that starts a workflow
2. Note the K8s UID and workflow ID
3. Delete the PipelineRun (workflow still running in Cadence)
4. Recreate a PipelineRun with the **same name**
5. Observe the new UID (different from step 2)
6. Trigger reconciliation

**Expected Result:**
- Controller reconstructs status (workflow found by name)
- Status fields are populated correctly
- Controller logs include warning about potential MySQL UID mismatch
- **MySQL caveat**: Old UID record remains orphaned; new UID record created

**Verification:**
```bash
# Check for orphaned MySQL records (if metadata storage enabled)
kubectl exec -n michelangelo-system mysql-pod -- mysql -e \
  "SELECT uid, name, namespace FROM pipeline_runs WHERE name='<name>'"
# Should show TWO records with different UIDs if recreated
```

**Recommendation**: Avoid this scenario in production by using unique PipelineRun names or ensuring workflows are terminated before deletion.

## Test Implementation

### Manual Testing in Sandbox

```bash
# 1. Create sandbox with Cadence
poetry run ma sandbox create --workflow cadence

# 2. Create a test pipeline and run
poetry run ma pipeline apply -f examples/demo/pipeline/training-pipeline.yaml
poetry run ma pipeline run -n ma-dev-test --name training-pipeline

# 3. Wait for workflow to start
kubectl get pipelinerun -n ma-dev-test --watch

# 4. Get the PipelineRun name and verify workflow is running
PIPELINE_RUN=$(kubectl get pipelinerun -n ma-dev-test -o jsonpath='{.items[0].metadata.name}')
echo "PipelineRun: $PIPELINE_RUN"

# Verify workflow ID in status
kubectl get pipelinerun $PIPELINE_RUN -n ma-dev-test -o jsonpath='{.status.workflowId}'
kubectl get pipelinerun $PIPELINE_RUN -n ma-dev-test -o jsonpath='{.status.workflowRunId}'

# 5. Simulate status loss by patching
kubectl patch pipelinerun $PIPELINE_RUN -n ma-dev-test --type=json -p='[
  {"op": "remove", "path": "/status/workflowId"},
  {"op": "remove", "path": "/status/workflowRunId"}
]'

# 6. Verify status was cleared
kubectl get pipelinerun $PIPELINE_RUN -n ma-dev-test -o jsonpath='{.status.workflowId}' # Should be empty
kubectl get pipelinerun $PIPELINE_RUN -n ma-dev-test -o jsonpath='{.status.workflowRunId}' # Should be empty

# 7. Trigger reconciliation by adding an annotation
kubectl annotate pipelinerun $PIPELINE_RUN -n ma-dev-test test-reconcile="$(date +%s)"

# 8. Wait a few seconds for reconciliation, then verify status was reconstructed
sleep 5
kubectl get pipelinerun $PIPELINE_RUN -n ma-dev-test -o jsonpath='{.status.workflowId}'  # Should be restored
kubectl get pipelinerun $PIPELINE_RUN -n ma-dev-test -o jsonpath='{.status.workflowRunId}' # Should be restored
kubectl get pipelinerun $PIPELINE_RUN -n ma-dev-test -o jsonpath='{.status.state}' # Should be RUNNING

# 9. Check controller logs for reconstruction messages
kubectl logs -n michelangelo-system deployment/michelangelo-controllermgr --tail=50 | grep "reconstructed status"
```

### Automated Integration Test Script

Location: `python/tests/integration/test_status_reconstruction.py`

```python
"""Integration test for PipelineRun status reconstruction feature."""

import time
import subprocess
import json
from typing import Dict, Any

def run_kubectl(*args: str) -> str:
    """Run kubectl command and return output."""
    result = subprocess.run(
        ["kubectl"] + list(args),
        capture_output=True,
        text=True,
        check=True
    )
    return result.stdout.strip()

def get_pipelinerun_status(name: str, namespace: str) -> Dict[str, Any]:
    """Get PipelineRun status."""
    output = run_kubectl(
        "get", "pipelinerun", name,
        "-n", namespace,
        "-o", "json"
    )
    pr = json.loads(output)
    return pr.get("status", {})

def test_status_reconstruction_running_workflow():
    """Test status reconstruction for a running workflow."""
    namespace = "ma-dev-test"
    
    # 1. Create a test pipeline run (assumes demo pipeline exists)
    run_kubectl("create", "pipelinerun", "test-reconstruction-run",
                "-n", namespace,
                "--image=<example-image>",
                "--pipeline=training-pipeline")
    
    # 2. Wait for workflow to start
    for _ in range(30):
        status = get_pipelinerun_status("test-reconstruction-run", namespace)
        if status.get("workflowId") and status.get("state") == "RUNNING":
            break
        time.sleep(2)
    else:
        raise AssertionError("Workflow did not start in time")
    
    original_workflow_id = status["workflowId"]
    original_run_id = status["workflowRunId"]
    
    # 3. Simulate status loss
    run_kubectl(
        "patch", "pipelinerun", "test-reconstruction-run",
        "-n", namespace,
        "--type=json",
        "-p=[{\"op\": \"remove\", \"path\": \"/status/workflowId\"},"
           "{\"op\": \"remove\", \"path\": \"/status/workflowRunId\"}]"
    )
    
    # 4. Verify status was cleared
    status = get_pipelinerun_status("test-reconstruction-run", namespace)
    assert status.get("workflowId") == "", "WorkflowId should be empty"
    assert status.get("workflowRunId") == "", "WorkflowRunId should be empty"
    
    # 5. Trigger reconciliation
    run_kubectl(
        "annotate", "pipelinerun", "test-reconstruction-run",
        "-n", namespace,
        f"test-reconcile={int(time.time())}"
    )
    
    # 6. Wait for reconstruction
    time.sleep(5)
    
    # 7. Verify status was reconstructed
    status = get_pipelinerun_status("test-reconstruction-run", namespace)
    assert status.get("workflowId") == original_workflow_id, \
        f"WorkflowId should be reconstructed to {original_workflow_id}"
    assert status.get("workflowRunId") == original_run_id, \
        f"WorkflowRunId should be reconstructed to {original_run_id}"
    assert status.get("state") in ["RUNNING", "SUCCEEDED"], \
        "State should be RUNNING or SUCCEEDED"
    
    print("✅ Status reconstruction test passed!")
    
    # Cleanup
    run_kubectl("delete", "pipelinerun", "test-reconstruction-run",
                "-n", namespace, "--ignore-not-found=true")

if __name__ == "__main__":
    test_status_reconstruction_running_workflow()
```

## Success Criteria

- ✅ Status reconstruction works for RUNNING workflows
- ✅ Status reconstruction works for COMPLETED workflows  
- ✅ Status reconstruction works for FAILED workflows
- ✅ Reconstruction fails gracefully for new PipelineRuns (no workflow yet)
- ✅ Reconstruction works correctly with retry scenarios
- ✅ Controller logs show appropriate INFO messages for successful reconstruction
- ✅ Controller logs show DEBUG messages for failed reconstruction attempts
- ✅ No errors or panics in controller logs during reconstruction

## CI Integration

This test should be added to the nightly integration test suite (`integration-test-sandbox.yaml`) as a new step:

```yaml
- name: Test Status Reconstruction
  run: |
    cd python
    poetry run pytest tests/integration/test_status_reconstruction.py -v
```

## Rollout Plan

1. **Phase 1 (Current)**: Manual testing in sandbox environment
2. **Phase 2**: Add automated integration test script
3. **Phase 3**: Integrate into nightly CI test suite
4. **Phase 4**: Add status reconstruction metrics and alerting

## Monitoring

After deployment, monitor these metrics:
- `pipelinerun_status_reconstruction_total` - Counter of reconstruction attempts
- `pipelinerun_status_reconstruction_success` - Counter of successful reconstructions
- `pipelinerun_status_reconstruction_failure` - Counter of failed reconstructions

## Related Documentation

- [Feature PR](https://github.com/michelangelo-ai/michelangelo/pull/TBD)
- [Integration Test Infrastructure](../../integration_test.md)
- [Sandbox Setup](../getting-started/sandbox-setup.md)
