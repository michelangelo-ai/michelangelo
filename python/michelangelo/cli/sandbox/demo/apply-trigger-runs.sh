#!/bin/bash

# Script to apply TriggerRun instances to the michelangelo-sandbox cluster

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CLUSTER_NAME="k3d-michelangelo-sandbox"

echo "Applying TriggerRun instances to cluster: $CLUSTER_NAME"

# Switch to the sandbox cluster context
echo "Setting kubectl context to $CLUSTER_NAME..."
kubectl config use-context $CLUSTER_NAME

# Verify cluster is accessible
echo "Verifying cluster connectivity..."
kubectl cluster-info

# Apply the TriggerRun resources
echo "Applying daily cron trigger run..."
kubectl apply -f "$SCRIPT_DIR/trigger-run-cron.yaml"

echo "Applying batch rerun trigger run..."
kubectl apply -f "$SCRIPT_DIR/trigger-run-batch-rerun.yaml"

# Verify the resources were created
echo ""
echo "Verifying TriggerRun resources were created:"
kubectl get triggerruns -n ma-dev-test

echo ""
echo "TriggerRun details:"
kubectl describe triggerrun daily-training-trigger -n ma-dev-test
echo ""
kubectl describe triggerrun batch-rerun-trigger -n ma-dev-test

echo ""
echo "TriggerRun instances successfully applied to cluster!"
echo ""
echo "To monitor status:"
echo "  kubectl get triggerruns -n ma-dev-test -w"
echo ""
echo "To view logs:"
echo "  kubectl logs -l app=michelangelo-controllermgr -n michelangelo-system"