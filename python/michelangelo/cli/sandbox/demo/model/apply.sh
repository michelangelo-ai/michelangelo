#!/usr/bin/env bash
# Applies a demo ModelFamily and two Models so the Create deployment form's
# Model family -> Model dropdowns have data to show in the ma-dev-test namespace.

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

kubectl apply -f "$SCRIPT_DIR/test-model-family.yaml"
kubectl apply -f "$SCRIPT_DIR/bert-cola.yaml"
kubectl apply -f "$SCRIPT_DIR/sentiment.yaml"

echo "test-model-family, bert-cola, and sentiment-model applied."
