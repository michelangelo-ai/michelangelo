#!/bin/bash
# Helper script to start port-forward for Michelangelo CLI
# Run this after creating/starting the sandbox

echo "Starting port-forward for Michelangelo API server..."
echo "This will forward localhost:14567 -> apiserver:14566"
echo ""
echo "Keep this terminal open while using 'ma' commands."
echo "Press Ctrl+C to stop the port-forward."
echo ""

kubectl --context k3d-michelangelo-sandbox port-forward svc/michelangelo-apiserver 14567:14566
