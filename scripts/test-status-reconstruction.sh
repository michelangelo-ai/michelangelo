#!/usr/bin/env bash
# Manual integration test for PipelineRun status reconstruction
#
# Prerequisites:
# - Sandbox is running (ma sandbox create)
# - Demo pipeline is registered
#
# Usage:
#   ./scripts/test-status-reconstruction.sh [namespace]
#
# Example:
#   ./scripts/test-status-reconstruction.sh ma-dev-test

set -euo pipefail

NAMESPACE="${1:-ma-dev-test}"
PIPELINE_NAME="training-pipeline"
TEST_RUN_NAME="test-reconstruction-$(date +%s)"

echo "🧪 Testing PipelineRun Status Reconstruction"
echo "============================================"
echo "Namespace: $NAMESPACE"
echo "Pipeline: $PIPELINE_NAME"
echo "Test Run: $TEST_RUN_NAME"
echo

# Function to wait for condition with timeout
wait_for_condition() {
    local description="$1"
    local check_command="$2"
    local timeout_seconds="${3:-60}"
    local interval="${4:-2}"

    echo "⏳ Waiting for: $description"
    local elapsed=0
    while [ $elapsed -lt $timeout_seconds ]; do
        if eval "$check_command"; then
            echo "✅ Condition met: $description"
            return 0
        fi
        sleep $interval
        elapsed=$((elapsed + interval))
    done

    echo "❌ Timeout waiting for: $description"
    return 1
}

# Step 1: Create a test pipeline run
echo "📝 Step 1: Creating test PipelineRun"
kubectl create -f - <<EOF
apiVersion: michelangelo.ai/v2
kind: PipelineRun
metadata:
  name: $TEST_RUN_NAME
  namespace: $NAMESPACE
spec:
  pipeline:
    name: $PIPELINE_NAME
    namespace: $NAMESPACE
EOF

# Step 2: Wait for workflow to start
echo "⏳ Step 2: Waiting for workflow to start..."
wait_for_condition \
    "Workflow to start" \
    "kubectl get pipelinerun $TEST_RUN_NAME -n $NAMESPACE -o jsonpath='{.status.workflowId}' | grep -q ." \
    120

# Get original workflow IDs
ORIGINAL_WORKFLOW_ID=$(kubectl get pipelinerun $TEST_RUN_NAME -n $NAMESPACE -o jsonpath='{.status.workflowId}')
ORIGINAL_RUN_ID=$(kubectl get pipelinerun $TEST_RUN_NAME -n $NAMESPACE -o jsonpath='{.status.workflowRunId}')
ORIGINAL_STATE=$(kubectl get pipelinerun $TEST_RUN_NAME -n $NAMESPACE -o jsonpath='{.status.state}')

echo "✅ Workflow started:"
echo "   WorkflowId: $ORIGINAL_WORKFLOW_ID"
echo "   WorkflowRunId: $ORIGINAL_RUN_ID"
echo "   State: $ORIGINAL_STATE"
echo

# Step 3: Simulate status loss
echo "💣 Step 3: Simulating status loss (removing workflowId and workflowRunId)"
kubectl patch pipelinerun $TEST_RUN_NAME -n $NAMESPACE --type=json -p='[
  {"op": "remove", "path": "/status/workflowId"},
  {"op": "remove", "path": "/status/workflowRunId"}
]'

# Verify status was cleared
CLEARED_WORKFLOW_ID=$(kubectl get pipelinerun $TEST_RUN_NAME -n $NAMESPACE -o jsonpath='{.status.workflowId}' || echo "")
CLEARED_RUN_ID=$(kubectl get pipelinerun $TEST_RUN_NAME -n $NAMESPACE -o jsonpath='{.status.workflowRunId}' || echo "")

if [ -z "$CLEARED_WORKFLOW_ID" ] && [ -z "$CLEARED_RUN_ID" ]; then
    echo "✅ Status cleared successfully"
else
    echo "❌ Status not fully cleared!"
    echo "   WorkflowId: $CLEARED_WORKFLOW_ID"
    echo "   WorkflowRunId: $CLEARED_RUN_ID"
    exit 1
fi
echo

# Step 4: Trigger reconciliation
echo "🔄 Step 4: Triggering reconciliation (adding annotation)"
kubectl annotate pipelinerun $TEST_RUN_NAME -n $NAMESPACE test-reconcile="$(date +%s)" --overwrite

# Step 5: Wait for status reconstruction
echo "⏳ Step 5: Waiting for status reconstruction..."
sleep 5

# Check if status was reconstructed
RECONSTRUCTED_WORKFLOW_ID=$(kubectl get pipelinerun $TEST_RUN_NAME -n $NAMESPACE -o jsonpath='{.status.workflowId}')
RECONSTRUCTED_RUN_ID=$(kubectl get pipelinerun $TEST_RUN_NAME -n $NAMESPACE -o jsonpath='{.status.workflowRunId}')
RECONSTRUCTED_STATE=$(kubectl get pipelinerun $TEST_RUN_NAME -n $NAMESPACE -o jsonpath='{.status.state}')

echo "📊 Reconstruction Results:"
echo "   Original WorkflowId:      $ORIGINAL_WORKFLOW_ID"
echo "   Reconstructed WorkflowId: $RECONSTRUCTED_WORKFLOW_ID"
echo
echo "   Original RunId:      $ORIGINAL_RUN_ID"
echo "   Reconstructed RunId: $RECONSTRUCTED_RUN_ID"
echo
echo "   Original State:      $ORIGINAL_STATE"
echo "   Reconstructed State: $RECONSTRUCTED_STATE"
echo

# Step 6: Verify reconstruction
echo "✔️  Step 6: Verifying reconstruction"
SUCCESS=true

if [ "$RECONSTRUCTED_WORKFLOW_ID" != "$ORIGINAL_WORKFLOW_ID" ]; then
    echo "❌ WorkflowId mismatch!"
    SUCCESS=false
fi

if [ "$RECONSTRUCTED_RUN_ID" != "$ORIGINAL_RUN_ID" ]; then
    echo "❌ WorkflowRunId mismatch!"
    SUCCESS=false
fi

if [ -z "$RECONSTRUCTED_STATE" ]; then
    echo "❌ State was not reconstructed!"
    SUCCESS=false
fi

if [ "$SUCCESS" = true ]; then
    echo "✅ Status reconstruction PASSED!"
    echo
    echo "🎉 All checks passed! Status was successfully reconstructed."
else
    echo "❌ Status reconstruction FAILED!"
    exit 1
fi

# Step 7: Check controller logs
echo
echo "📋 Step 7: Checking controller logs for reconstruction messages"
echo "Looking for 'reconstructed status from workflow engine' in logs..."
kubectl logs -n michelangelo-system deployment/michelangelo-controllermgr --tail=100 | \
    grep -i "reconstructed status" || echo "⚠️  No reconstruction log messages found (may be normal if logs rotated)"

# Cleanup
echo
read -p "🗑️  Delete test PipelineRun? [y/N] " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    kubectl delete pipelinerun $TEST_RUN_NAME -n $NAMESPACE --ignore-not-found=true
    echo "✅ Test PipelineRun deleted"
else
    echo "ℹ️  Test PipelineRun left for inspection: $TEST_RUN_NAME"
fi

echo
echo "✅ Test completed successfully!"
