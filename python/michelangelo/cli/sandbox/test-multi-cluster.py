#!/usr/bin/env python3
"""
Test script for Multi-Cluster Sandbox

This script validates that the multi-cluster setup is working correctly by:
1. Checking cluster status
2. Verifying cross-cluster connectivity
3. Testing multi-tenant storage configuration
4. Validating job cluster operators
"""

import subprocess
import sys
import yaml
from pathlib import Path

def run_command(cmd, capture_output=True, check=True):
    """Run a command and return the result."""
    try:
        result = subprocess.run(
            cmd, shell=True, capture_output=capture_output,
            text=True, check=check
        )
        return result.stdout.strip() if capture_output else None
    except subprocess.CalledProcessError as e:
        if capture_output:
            print(f"❌ Command failed: {cmd}")
            print(f"   Error: {e.stderr}")
        return None

def test_cluster_status():
    """Test that all clusters are running."""
    print("🔍 Testing cluster status...")

    clusters = [
        "michelangelo-control-plane",
        "michelangelo-job-cluster1",
        "michelangelo-job-cluster2"
    ]

    all_running = True
    for cluster in clusters:
        result = run_command(f"k3d cluster list {cluster}", check=False)
        if result and cluster in result and "running" in result.lower():
            print(f"✅ {cluster}: RUNNING")
        else:
            print(f"❌ {cluster}: NOT RUNNING")
            all_running = False

    return all_running

def test_kubectl_contexts():
    """Test that kubectl contexts are properly configured."""
    print("\n🔍 Testing kubectl contexts...")

    contexts = [
        "k3d-michelangelo-control-plane",
        "k3d-michelangelo-job-cluster1",
        "k3d-michelangelo-job-cluster2"
    ]

    result = run_command("kubectl config get-contexts")
    if not result:
        print("❌ Failed to get kubectl contexts")
        return False

    all_contexts = True
    for context in contexts:
        if context in result:
            print(f"✅ Context available: {context}")
        else:
            print(f"❌ Context missing: {context}")
            all_contexts = False

    return all_contexts

def test_control_plane_services():
    """Test that control plane services are running."""
    print("\n🔍 Testing control plane services...")

    # Switch to control plane context
    run_command("kubectl config use-context k3d-michelangelo-control-plane", capture_output=False)

    # Check key services
    services = [
        ("mysql", "MySQL database"),
        ("minio", "MinIO storage"),
        ("michelangelo-apiserver", "Michelangelo API server"),
    ]

    all_services = True
    for service, description in services:
        result = run_command(f"kubectl get pods -l app={service} --no-headers", check=False)
        if result and "Running" in result:
            print(f"✅ {description}: RUNNING")
        else:
            print(f"❌ {description}: NOT RUNNING")
            all_services = False

    return all_services

def test_job_cluster_operators():
    """Test that Ray and Spark operators are installed on job clusters."""
    print("\n🔍 Testing job cluster operators...")

    job_clusters = [
        ("michelangelo-job-cluster1", "Job Cluster 1"),
        ("michelangelo-job-cluster2", "Job Cluster 2")
    ]

    all_operators = True
    for cluster, name in job_clusters:
        print(f"\n  Testing {name} ({cluster}):")

        # Switch context
        run_command(f"kubectl config use-context k3d-{cluster}", capture_output=False)

        # Check KubeRay operator
        result = run_command("kubectl get pods -n ray-system --no-headers", check=False)
        if result and "kuberay-operator" in result and "Running" in result:
            print(f"    ✅ KubeRay operator: RUNNING")
        else:
            print(f"    ❌ KubeRay operator: NOT RUNNING")
            all_operators = False

        # Check Spark operator
        result = run_command("kubectl get pods -n spark-operator --no-headers", check=False)
        if result and "spark-operator" in result and "Running" in result:
            print(f"    ✅ Spark operator: RUNNING")
        else:
            print(f"    ❌ Spark operator: NOT RUNNING")
            all_operators = False

    return all_operators

def test_cross_cluster_crds():
    """Test that cluster CRDs are created in control plane."""
    print("\n🔍 Testing cross-cluster CRDs...")

    # Switch to control plane
    run_command("kubectl config use-context k3d-michelangelo-control-plane", capture_output=False)

    # Check for cluster CRDs
    result = run_command("kubectl get clusters --no-headers", check=False)
    if not result:
        print("❌ No cluster CRDs found")
        return False

    expected_clusters = ["michelangelo-job-cluster1", "michelangelo-job-cluster2"]
    all_crds = True

    for cluster in expected_clusters:
        if cluster in result:
            print(f"✅ Cluster CRD exists: {cluster}")
        else:
            print(f"❌ Cluster CRD missing: {cluster}")
            all_crds = False

    return all_crds

def test_storage_config():
    """Test that multi-tenant storage configuration is deployed."""
    print("\n🔍 Testing storage configuration...")

    # Switch to control plane
    run_command("kubectl config use-context k3d-michelangelo-control-plane", capture_output=False)

    # Check storage providers configmap
    result = run_command("kubectl get configmap storage-providers-config -o yaml", check=False)
    if not result:
        print("❌ Storage providers config not found")
        return False

    try:
        config_data = yaml.safe_load(result)
        config_yaml = config_data['data']['config.yaml']
        storage_config = yaml.safe_load(config_yaml)

        expected_providers = ["aws-dev", "aws-company1", "aws-company2", "azure-sharezone"]
        minio_providers = storage_config.get('minio', {}).get('storageProviders', {})

        all_providers = True
        for provider in expected_providers:
            if provider in minio_providers:
                print(f"✅ Storage provider configured: {provider}")
            else:
                print(f"❌ Storage provider missing: {provider}")
                all_providers = False

        return all_providers

    except Exception as e:
        print(f"❌ Failed to parse storage config: {e}")
        return False

def test_demo_projects():
    """Test that demo projects are created."""
    print("\n🔍 Testing demo projects...")

    # Switch to control plane
    run_command("kubectl config use-context k3d-michelangelo-control-plane", capture_output=False)

    # Check for demo projects
    result = run_command("kubectl get projects --all-namespaces --no-headers", check=False)
    if not result:
        print("⚠️  No demo projects found (run 'python sandbox-multi-clusters.py demo' to create them)")
        return True  # Not a failure, just not created yet

    expected_projects = ["ma-dev-test", "company1-ml-project", "company2-analytics", "shared-research"]
    all_projects = True

    for project in expected_projects:
        if project in result:
            print(f"✅ Demo project exists: {project}")
        else:
            print(f"⚠️  Demo project missing: {project} (run demo command to create)")
            # Not marking as failure since demo is optional

    return all_projects

def main():
    """Run all tests."""
    print("🧪 Multi-Cluster Sandbox Test Suite")
    print("=" * 50)

    tests = [
        ("Cluster Status", test_cluster_status),
        ("Kubectl Contexts", test_kubectl_contexts),
        ("Control Plane Services", test_control_plane_services),
        ("Job Cluster Operators", test_job_cluster_operators),
        ("Cross-Cluster CRDs", test_cross_cluster_crds),
        ("Storage Configuration", test_storage_config),
        ("Demo Projects", test_demo_projects),
    ]

    results = []
    for test_name, test_func in tests:
        print(f"\n{'=' * 20} {test_name} {'=' * 20}")
        try:
            result = test_func()
            results.append((test_name, result))
        except Exception as e:
            print(f"❌ Test failed with exception: {e}")
            results.append((test_name, False))

    # Summary
    print(f"\n{'=' * 50}")
    print("📊 TEST SUMMARY")
    print(f"{'=' * 50}")

    passed = 0
    total = len(results)

    for test_name, passed_test in results:
        status = "✅ PASS" if passed_test else "❌ FAIL"
        print(f"{status} {test_name}")
        if passed_test:
            passed += 1

    print(f"\nResults: {passed}/{total} tests passed")

    if passed == total:
        print("🎉 All tests passed! Multi-cluster sandbox is working correctly.")
        return 0
    else:
        print("⚠️  Some tests failed. Check the output above for details.")
        return 1

if __name__ == "__main__":
    sys.exit(main())