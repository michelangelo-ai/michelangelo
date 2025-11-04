#!/usr/bin/env python3
"""
Michelangelo Multi-Cluster Sandbox

This script creates a multi-cluster sandbox environment with:
1. Control Plane Cluster: Runs Michelangelo services (API, UI, Worker, etc.)
2. Job Cluster 1: Dedicated Ray and Spark job execution
3. Job Cluster 2: Dedicated Ray and Spark job execution

This setup enables testing multi-cluster job submission and resource isolation.
"""

import json
import os
import sys
import argparse
import shutil
import subprocess
import tempfile
import time
import uuid
import yaml
from pathlib import Path
import base64

short_description = "Manage the multi-cluster sandbox environment."

description = """
Michelangelo Multi-Cluster Sandbox creates a distributed environment with:
- 1 Control Plane cluster (Michelangelo services)
- 2 Job clusters (Ray + Spark execution)

This enables testing multi-cluster job submission, resource isolation, and cross-cluster communication.
"""

_dir = Path(__file__).parent

# Cluster configurations
_control_plane_cluster_name = "michelangelo-control-plane"
_job_cluster1_name = "michelangelo-job-cluster1"
_job_cluster2_name = "michelangelo-job-cluster2"

# Control plane ports (API, UI, etc.)
_control_plane_ports = [
    "3306:30001",  # MySQL
    "9091:30007",  # MinIO
    "9090:30008",  # MinIO Console
    "14566:30009",  # Michelangelo API Server
    "8081:30010",  # Envoy gRPC --> gRPC-web proxy
    "8090:30011",  # Michelangelo UI
    "3000:30012",  # Grafana
    "9092:30015",  # Prometheus
]

# Workflow engine ports
_cadence_ports = [
    "7833:30002",  # Cadence gRPC
    "7933:30003",  # Cadence TChannel
    "8088:30004",  # Cadence Web
]

# Job cluster 1 ports (Ray)
_job_cluster1_ports = [
    "10001:10001",  # Ray client port
    "8265:8265",    # Ray dashboard
    "9093:9093",    # Spark UI
]

# Job cluster 2 ports (Ray - different ports to avoid conflicts)
_job_cluster2_ports = [
    "10002:10001",  # Ray client port (mapped to different host port)
    "8266:8265",    # Ray dashboard (mapped to different host port)
    "9094:9093",    # Spark UI (mapped to different host port)
]

_cadence_domain = "default"


def init_arguments(p: argparse.ArgumentParser):
    sp = p.add_subparsers(dest="action", required=True)

    create_p = sp.add_parser("create", help="Create and start all clusters.")
    create_p.add_argument(
        "--exclude",
        help="Excludes specified services from control plane. Available options: apiserver, controllermgr, ui, worker",
        nargs="+",
        default=[],
    )
    create_p.add_argument(
        "--workflow",
        choices=["cadence", "temporal"],
        default="cadence",
        help="Choose workflow engine: cadence or temporal (default: cadence).",
    )
    create_p.add_argument(
        "--include-experimental",
        help="Include experimental services in control plane.",
        nargs="+",
        default=[],
    )
    create_p.add_argument(
        "--job-cluster1-name",
        default=_job_cluster1_name,
        help=f"Name of the first job cluster (default: {_job_cluster1_name}).",
    )
    create_p.add_argument(
        "--job-cluster2-name",
        default=_job_cluster2_name,
        help=f"Name of the second job cluster (default: {_job_cluster2_name}).",
    )

    _ = sp.add_parser(
        "demo", help="Create demo projects and pipelines in the sandbox clusters."
    )

    delete_p = sp.add_parser("delete", help="Delete all clusters.")
    delete_p.add_argument(
        "--job-cluster1-name",
        default=_job_cluster1_name,
        help=f"Name of the first job cluster to delete (default: {_job_cluster1_name}).",
    )
    delete_p.add_argument(
        "--job-cluster2-name",
        default=_job_cluster2_name,
        help=f"Name of the second job cluster to delete (default: {_job_cluster2_name}).",
    )

    _ = sp.add_parser("start", help="Start all clusters.")
    _ = sp.add_parser("stop", help="Stop all clusters.")

    status_p = sp.add_parser("status", help="Show status of all clusters.")


def main(args=None):
    p = argparse.ArgumentParser(description=description)
    init_arguments(p)
    ns = p.parse_args(args=args)
    return run(ns)


def run(ns: argparse.Namespace):
    # Assert prerequisites
    _assert_command("k3d", "k3d not found, please install it: https://k3d.io")
    _assert_command(
        "kubectl",
        "kubectl not found, please install it: https://kubernetes.io/docs/tasks/tools/#kubectl",
    )

    if ns.action == "create":
        return _create(ns)
    if ns.action == "delete":
        return _delete(ns)
    if ns.action == "start":
        return _start(ns)
    if ns.action == "stop":
        return _stop(ns)
    if ns.action == "demo":
        return _create_demo_crs(ns)
    if ns.action == "status":
        return _status(ns)

    raise ValueError(f"Unsupported action: {ns.action}")


def _create(ns: argparse.Namespace):
    """Create all three clusters: control plane + 2 job clusters."""
    print("🚀 Creating multi-cluster sandbox environment...")
    print(f"  - Control Plane: {_control_plane_cluster_name}")
    print(f"  - Job Cluster 1: {ns.job_cluster1_name}")
    print(f"  - Job Cluster 2: {ns.job_cluster2_name}")

    # Create clusters in parallel where possible
    _create_control_plane_cluster(ns)
    _create_job_cluster(ns.job_cluster1_name, _job_cluster1_ports, "job-cluster1")
    _create_job_cluster(ns.job_cluster2_name, _job_cluster2_ports, "job-cluster2")

    # Setup cross-cluster communication
    _setup_cross_cluster_communication(ns)

    # Display access information
    _display_cluster_info(ns)


def _create_control_plane_cluster(ns: argparse.Namespace):
    """Create the control plane cluster with Michelangelo services."""
    print(f"\n📋 Creating control plane cluster: {_control_plane_cluster_name}")

    ports = _control_plane_ports + ([] if ns.workflow == "temporal" else _cadence_ports)
    args = [
        "k3d",
        "cluster",
        "create",
        _control_plane_cluster_name,
        "--servers",
        "1",
        "--agents",
        "2",  # More agents for control plane services
    ]

    for p in ports:
        args += ["-p", f"{p}@agent:0"]

    # TODO: GitHub Container Registry authentication (same as original sandbox)
    env_cr_pat = "CR_PAT"
    cr_pat = os.environ.get(env_cr_pat)
    if not cr_pat:
        _err_exit(
            """
CR_PAT environment variable is not set. To pull Michelangelo's containers from the GitHub Container Registry, please create a GitHub personal access token (classic) with the "read:packages" scope. Then, save this token to the CR_PAT environment variable, e.g.: `export CR_PAT=ghp_...`.

For a detailed guide, check https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry#authenticating-with-a-personal-access-token-classic.
"""
        )

    # Create temporary registry file
    registry = {
        "mirrors": {
            "ghcr.io": {
                "endpoint": ["https://ghcr.io"],
            },
        },
        "configs": {
            "ghcr.io": {
                "auth": {
                    "username": "USERNAME",
                    "password": cr_pat,
                },
            },
        },
    }

    registry_file = tempfile.NamedTemporaryFile(mode="wt")
    json.dump(registry, registry_file)
    registry_file.flush()
    args += ["--registry-config", registry_file.name]

    _exec(*args)

    # Switch to control plane context
    _exec("kubectl", "config", "use-context", f"k3d-{_control_plane_cluster_name}")

    # Deploy Michelangelo services
    _deploy_control_plane_services(ns)


def _deploy_control_plane_services(ns: argparse.Namespace):
    """Deploy Michelangelo services to the control plane cluster."""
    print("📦 Deploying Michelangelo services to control plane...")

    resources = [
        "boot.yaml",
        "mysql.yaml",
        "michelangelo-config.yaml",
        "storage-providers-config.yaml",  # Multi-tenant storage config
        "aws-credentials.yaml",
        "yscope-log-viewer-deployment.yaml",
        "sandbox-bucket-setup.yaml",
    ]
    links = []

    # Workflow engine
    if ns.workflow == "cadence":
        resources.append("cadence.yaml")
        links.append(
            (
                "Cadence Web UI",
                "http://localhost:8088/domains/default/workflows",
                "",
            )
        )

    # MinIO
    resources.append("minio.yaml")
    links.append(
        (
            "MinIO Console",
            "http://localhost:9090",
            "[Username: minioadmin; Password: minioadmin]",
        )
    )

    # Prometheus & Grafana
    resources.append("prometheus.yaml")
    resources.append("grafana.yaml")
    links.append(("Prometheus", "http://localhost:9092", ""))
    links.append(
        ("Grafana Dashboard", "http://localhost:3000", "[Username: admin; Password: admin]")
    )

    # Michelangelo core services
    if "apiserver" not in ns.exclude:
        resources.append("michelangelo-apiserver.yaml")
    if "controllermgr" not in ns.exclude:
        resources.append("michelangelo-controllermgr.yaml")
    if "ui" not in ns.exclude:
        resources.append("envoy.yaml")
        resources.append("michelangelo-ui.yaml")
        links.append(("Michelangelo UI", "http://localhost:8090", ""))

    # Experimental services
    if "fluent-bit" in ns.include_experimental:
        _exec("kubectl", "create", "serviceaccount", "fluent-bit")
        resources.extend(["fluent-bit.yaml", "fluent-bit-config.yaml"])

    # Apply all resources
    for r in resources:
        _kube_create(_dir / "resources" / r)

    # Install Helm charts
    _assert_command("helm", "Helm not found, please install it: https://helm.sh/docs/intro/install/")

    try:
        helm_existing_repos = subprocess.check_output(["helm", "repo", "list"]).decode()
    except subprocess.CalledProcessError:
        helm_existing_repos = ""

    # Note: We don't install Ray/Spark on control plane - only on job clusters
    _kube_wait()

    # Setup workflow engine
    if ns.workflow == "temporal":
        _setup_temporal(links, helm_existing_repos)
        if "worker" not in ns.exclude:
            _kube_create(_dir / "resources/michelangelo-temporal-worker.yaml")
    elif ns.workflow == "cadence":
        _create_cadence_domain(links)
        if "worker" not in ns.exclude:
            _kube_create(_dir / "resources/michelangelo-worker.yaml")

    _kube_wait()

    # Store links for later display
    ns._control_plane_links = links


def _create_job_cluster(cluster_name: str, ports: list, cluster_type: str):
    """Create a job cluster with Ray and Spark operators."""
    print(f"\n⚡ Creating job cluster: {cluster_name} ({cluster_type})")

    args = [
        "k3d",
        "cluster",
        "create",
        cluster_name,
        "--servers",
        "1",
        "--agents",
        "3",  # More worker nodes for jobs
        "--kubeconfig-switch-context=false",  # Don't switch context
    ]

    # Add port mappings
    for p in ports:
        args += ["-p", f"{p}@agent:0"]

    _exec(*args)

    # Switch to job cluster context temporarily
    original_context = subprocess.check_output(["kubectl", "config", "current-context"]).decode().strip()
    _exec("kubectl", "config", "use-context", f"k3d-{cluster_name}")

    try:
        # Install operators on job cluster
        _install_job_cluster_operators(cluster_name)

        # Apply RBAC
        _apply_jobs_rbac(cluster_name)

    finally:
        # Switch back to original context
        _exec("kubectl", "config", "use-context", original_context)

    print(f"✅ Job cluster '{cluster_name}' created successfully.")


def _install_job_cluster_operators(cluster_name: str):
    """Install Ray and Spark operators on a job cluster."""
    print(f"📦 Installing operators on {cluster_name}...")

    # Get existing helm repos
    try:
        helm_existing_repos = subprocess.check_output(["helm", "repo", "list"]).decode()
    except subprocess.CalledProcessError:
        helm_existing_repos = ""

    # Install KubeRay operator
    _create_kuberay_operator(helm_existing_repos, cluster_name)

    # Install Spark operator
    _create_spark_operator(helm_existing_repos, cluster_name)


def _create_kuberay_operator(helm_existing_repos: str, cluster_name: str):
    """Install KubeRay operator on the specified cluster."""
    print(f"🔄 Installing KubeRay operator on {cluster_name}...")

    if "kuberay" not in helm_existing_repos:
        _exec("helm", "repo", "add", "kuberay", "https://ray-project.github.io/kuberay-helm")
        _exec("helm", "repo", "update")

    _exec(
        "helm",
        "install",
        "kuberay-operator",
        "kuberay/kuberay-operator",
        "--version",
        "1.4.2",
        "--namespace",
        "ray-system",
        "--create-namespace",
        "--wait",
        "--kube-context",
        f"k3d-{cluster_name}",
    )


def _create_spark_operator(helm_existing_repos: str, cluster_name: str):
    """Install Spark operator on the specified cluster."""
    print(f"⚡ Installing Spark operator on {cluster_name}...")

    if "spark-operator" not in helm_existing_repos:
        _exec("helm", "repo", "add", "spark-operator", "https://kubeflow.github.io/spark-operator")
        _exec("helm", "repo", "update")

    _exec(
        "helm",
        "install",
        "spark-operator",
        "spark-operator/spark-operator",
        "--namespace",
        "spark-operator",
        "--create-namespace",
        "--wait",
        "--kube-context",
        f"k3d-{cluster_name}",
        "--set",
        "controller.namespaces=",  # Empty string means watch all namespaces
    )


def _setup_cross_cluster_communication(ns: argparse.Namespace):
    """Setup communication between control plane and job clusters."""
    print("\n🔗 Setting up cross-cluster communication...")

    # Switch back to control plane context
    _exec("kubectl", "config", "use-context", f"k3d-{_control_plane_cluster_name}")

    # Create cluster CRDs and secrets for each job cluster
    for cluster_name in [ns.job_cluster1_name, ns.job_cluster2_name]:
        _create_cluster_crd(cluster_name)
        _create_cluster_secrets(cluster_name)

    print("✅ Cross-cluster communication configured.")


def _setup_temporal(links, helm_existing_repos):
    """Setup Temporal workflow engine."""
    if "temporal" not in helm_existing_repos:
        _exec("helm", "repo", "add", "temporal", "https://temporalio.github.io/helm-charts")
        _exec("helm", "repo", "update")

    values_file = _dir / "resources" / "temporal.mysql.yaml"

    _exec(
        "helm",
        "install",
        "temporaltest",
        "temporal",
        "--repo",
        "https://go.temporal.io/helm-charts",
        "-f",
        str(values_file),
        "--set",
        "elasticsearch.enabled=false",
        "--set",
        "prometheus.enabled=false",
        "--set",
        "grafana.enabled=false",
    )

    _exec(
        "kubectl",
        "-n",
        "default",
        "wait",
        "--for=condition=available",
        "deployment",
        "--selector=!job-name",
        "--all",
        "--timeout=600s",
    )

    # Register default namespace
    _exec(
        "kubectl",
        "exec",
        "deploy/temporaltest-admintools",
        "--",
        "tctl",
        "--address",
        "temporaltest-frontend:7233",
        "namespace",
        "register",
        "default",
        "--retention",
        "72",
    )

    # Port-forward in background
    subprocess.Popen(
        ["kubectl", "port-forward", "svc/temporaltest-web", "8080:8080"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    subprocess.Popen(
        ["kubectl", "port-forward", "svc/temporaltest-frontend", "7233:7233"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )
    links.append(("Temporal Web UI", "http://localhost:8080", ""))


def _create_cadence_domain(links):
    """Create Cadence domain."""
    _kube_run(
        image="ubercadence/cli:v1.2.6",
        command=[
            "cadence",
            "--domain",
            _cadence_domain,
            "domain",
            "register",
            "--rd",
            "1",
        ],
        env={"CADENCE_CLI_ADDRESS": "cadence:7933"},
        retry_attempts=3,
    )


def _create_demo_crs(ns: argparse.Namespace):
    """Create demo Custom Resources across clusters."""
    print("🎯 Creating demo resources...")

    # Switch to control plane
    _exec("kubectl", "config", "use-context", f"k3d-{_control_plane_cluster_name}")

    # Check if control plane cluster exists and is running
    try:
        _exec("k3d", "cluster", "get", _control_plane_cluster_name, raise_error=True)
        _exec("kubectl", "cluster-info", raise_error=True)
    except subprocess.CalledProcessError:
        _err_exit(f"Control plane cluster {_control_plane_cluster_name} not found or not running. Please run 'create' first.")

    demo_dir = _dir / "demo"

    # Create multi-tenant demo projects
    demo_projects = [
        "project.yaml",         # aws-dev
        "project-company1.yaml", # aws-company1
        "project-company2.yaml", # aws-company2
        "project-shared.yaml",   # azure-sharezone
    ]

    # Create namespaces and projects
    namespaces_created = set()
    for project_file in demo_projects:
        project_path = demo_dir / project_file
        if not project_path.exists():
            print(f"⚠️  Demo file {project_file} not found, skipping...")
            continue

        with open(project_path) as f:
            project_yaml = yaml.safe_load(f)
        namespace = project_yaml.get("metadata", {}).get("namespace", "default")

        if namespace not in namespaces_created:
            _exec("kubectl", "create", "namespace", namespace, "--dry-run=client", "-o", "yaml")
            _exec("kubectl", "apply", "-f", "-", input=subprocess.check_output([
                "kubectl", "create", "namespace", namespace, "--dry-run=client", "-o", "yaml"
            ]))
            namespaces_created.add(namespace)

        _kube_create(project_path)
        print(f"✅ Created project from {project_file} in namespace {namespace}")

    # Create other demo resources
    for yaml_file in demo_dir.glob("*.yaml"):
        if yaml_file.name not in demo_projects:
            _kube_create(yaml_file)
            print(f"✅ Created demo resource: {yaml_file.name}")

    print(f"\n🎯 Demo resources created across {len(namespaces_created)} namespaces")
    print("Each project is configured with a different resource provider for multi-tenant testing.")


def _delete(ns: argparse.Namespace):
    """Delete all clusters."""
    print("🗑️  Deleting multi-cluster sandbox environment...")

    clusters = [_control_plane_cluster_name, ns.job_cluster1_name, ns.job_cluster2_name]
    for cluster in clusters:
        try:
            _exec("k3d", "cluster", "delete", cluster)
            print(f"✅ Deleted cluster: {cluster}")
        except:
            print(f"⚠️  Failed to delete cluster: {cluster} (may not exist)")


def _start(ns: argparse.Namespace):
    """Start all clusters."""
    print("▶️  Starting multi-cluster sandbox environment...")

    clusters = [_control_plane_cluster_name, ns.job_cluster1_name, ns.job_cluster2_name]
    for cluster in clusters:
        try:
            _exec("k3d", "cluster", "start", cluster)
            print(f"✅ Started cluster: {cluster}")
        except:
            print(f"⚠️  Failed to start cluster: {cluster}")


def _stop(ns: argparse.Namespace):
    """Stop all clusters."""
    print("⏹️  Stopping multi-cluster sandbox environment...")

    clusters = [_control_plane_cluster_name, ns.job_cluster1_name, ns.job_cluster2_name]
    for cluster in clusters:
        try:
            _exec("k3d", "cluster", "stop", cluster)
            print(f"✅ Stopped cluster: {cluster}")
        except:
            print(f"⚠️  Failed to stop cluster: {cluster}")


def _status(ns: argparse.Namespace):
    """Show status of all clusters."""
    print("📊 Multi-Cluster Sandbox Status\n")

    clusters = [
        (_control_plane_cluster_name, "Control Plane"),
        (getattr(ns, 'job_cluster1_name', _job_cluster1_name), "Job Cluster 1"),
        (getattr(ns, 'job_cluster2_name', _job_cluster2_name), "Job Cluster 2"),
    ]

    for cluster_name, cluster_type in clusters:
        try:
            result = subprocess.check_output(["k3d", "cluster", "list", cluster_name], stderr=subprocess.DEVNULL)
            if cluster_name in result.decode():
                print(f"✅ {cluster_type}: {cluster_name} - RUNNING")
            else:
                print(f"❌ {cluster_type}: {cluster_name} - NOT FOUND")
        except:
            print(f"❌ {cluster_type}: {cluster_name} - NOT FOUND")


def _display_cluster_info(ns: argparse.Namespace):
    """Display access information for all clusters."""
    print("\n🚀 Multi-Cluster Sandbox created successfully!\n")

    print("📋 CONTROL PLANE CLUSTER")
    print(f"   Cluster: {_control_plane_cluster_name}")
    print(f"   Context: k3d-{_control_plane_cluster_name}")
    if hasattr(ns, '_control_plane_links'):
        for title, url, comment in ns._control_plane_links:
            print(f"   - {title}: {url} {comment}")

    print(f"\n⚡ JOB CLUSTER 1")
    print(f"   Cluster: {ns.job_cluster1_name}")
    print(f"   Context: k3d-{ns.job_cluster1_name}")
    print("   - Ray Dashboard: http://localhost:8265")
    print("   - Spark UI: http://localhost:9093")
    print("   - Ray Client Port: 10001")

    print(f"\n⚡ JOB CLUSTER 2")
    print(f"   Cluster: {ns.job_cluster2_name}")
    print(f"   Context: k3d-{ns.job_cluster2_name}")
    print("   - Ray Dashboard: http://localhost:8266")
    print("   - Spark UI: http://localhost:9094")
    print("   - Ray Client Port: 10002")

    print("\n🔧 KUBECTL CONTEXT SWITCHING")
    print(f"   Control Plane: kubectl config use-context k3d-{_control_plane_cluster_name}")
    print(f"   Job Cluster 1: kubectl config use-context k3d-{ns.job_cluster1_name}")
    print(f"   Job Cluster 2: kubectl config use-context k3d-{ns.job_cluster2_name}")

    print("\n🎯 NEXT STEPS")
    print("   1. Run 'python sandbox-multi-clusters.py demo' to create demo projects")
    print("   2. Submit jobs to different clusters using Michelangelo UI")
    print("   3. Monitor jobs across clusters via Ray/Spark dashboards")


# Helper functions (same as original sandbox but with multi-cluster awareness)

def _apply_jobs_rbac(cluster_name: str):
    """Apply RBAC for Ray management in the jobs cluster."""
    rbac_path = _dir / "resources" / "rbac-ray.yaml"
    _exec(
        "kubectl",
        "--context",
        f"k3d-{cluster_name}",
        "apply",
        "-f",
        str(rbac_path),
    )


def _kube_create(path: Path):
    _exec("kubectl", "create", "-f", str(path))


def _kube_wait(pods: bool = True, jobs: bool = True):
    if pods:
        _exec(
            "kubectl",
            "wait",
            "--all",
            "pods",
            "--for=condition=ready",
            "--selector=!job-name",
            "--timeout=600s",
        )
    if jobs:
        _exec(
            "kubectl",
            "wait",
            "--all",
            "jobs",
            "--for=condition=complete",
            "--timeout=600s",
        )


def _kube_run(
    image: str,
    command: list[str],
    env: dict[str, str] = None,
    retry_attempts: int = 0,
):
    args = [
        "kubectl",
        "run",
        uuid.uuid4().hex,
        "--restart=Never",
        "--rm",
        "--stdin",
        "--image",
        image,
    ]
    if env:
        args += [f"--env={k}={v}" for k, v in env.items()]

    args += ["--command", "--", *command]
    return _exec(*args, retry_attempts=retry_attempts)


def _exec(
    *args,
    retry_attempts: int = 0,
    retry_delay_seconds: int = 5,
    raise_error: bool = False,
    input: bytes = None,
):
    """Execute a shell command with optional retries."""
    for i in range(retry_attempts + 1):
        try:
            print("[+]", " ".join(args))
            if input:
                subprocess.check_call(args, input=input)
            else:
                subprocess.check_call(args)
            return
        except subprocess.CalledProcessError as e:
            if i == retry_attempts:
                if raise_error:
                    raise e
                else:
                    _err_exit("command failed", code=e.returncode)
            print("retrying after", retry_delay_seconds, "seconds...")
            time.sleep(retry_delay_seconds)


def _assert_command(command: str, err_message: str):
    if shutil.which(command) is None:
        _err_exit(err_message)


def _err_exit(err_message: str, code: int = 1):
    print(f"\033[91m\033[1mERROR: {err_message}\nexit {code}\033[0m")
    sys.exit(code)


def _create_cluster_crd(cluster_name: str):
    """Create a Cluster CRD for the job cluster in the control plane cluster."""
    kubeconfig = subprocess.check_output(["k3d", "kubeconfig", "get", cluster_name]).decode()
    kubeconfig_data = yaml.safe_load(kubeconfig)
    server_url = kubeconfig_data["clusters"][0]["cluster"]["server"]

    import re
    match = re.search(r"(https://[^:]+):(\d+)", server_url)
    if not match:
        raise ValueError(f"Could not extract cluster host and port from server URL: {server_url}")
    host, port = match.groups()

    cluster_crd = {
        "apiVersion": "michelangelo.api/v2",
        "kind": "Cluster",
        "metadata": {"name": cluster_name, "namespace": "default"},
        "spec": {
            "kubernetes": {
                "rest": {
                    "host": host,
                    "port": port,
                    "tokenTag": f"cluster-{cluster_name}-client-token",
                    "caDataTag": f"cluster-{cluster_name}-ca-data",
                },
                "skus": [],
            }
        },
    }

    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as crd_file:
        yaml.dump(cluster_crd, crd_file)
        crd_file.flush()
        _exec(
            "kubectl",
            "--context",
            f"k3d-{_control_plane_cluster_name}",
            "apply",
            "-f",
            crd_file.name,
        )
    print(f"✅ Created Cluster CRD '{cluster_name}' in control plane")


def _create_cluster_secrets(cluster_name: str):
    """Create Kubernetes secrets for the kubeconfig of the given cluster."""
    kubeconfig = subprocess.check_output(["k3d", "kubeconfig", "get", cluster_name]).decode()
    kubeconfig_data = yaml.safe_load(kubeconfig)

    # CA data
    ca_data = kubeconfig_data["clusters"][0]["cluster"].get("certificate-authority-data")
    if not ca_data:
        raise ValueError("certificate-authority-data not found in kubeconfig")
    ca_data_decoded = base64.b64decode(ca_data).decode()

    ca_secret = {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {"name": f"cluster-{cluster_name}-ca-data", "namespace": "default"},
        "stringData": {"cadata": ca_data_decoded},
    }

    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as ca_file:
        yaml.dump(ca_secret, ca_file)
        ca_file.flush()
        _exec(
            "kubectl",
            "--context",
            f"k3d-{_control_plane_cluster_name}",
            "apply",
            "-f",
            ca_file.name,
        )

    # Token
    token_decoded = (
        subprocess.check_output([
            "kubectl",
            "--context",
            f"k3d-{cluster_name}",
            "-n",
            "default",
            "create",
            "token",
            "ray-manager",
            "--duration=87600h",
        ])
        .decode()
        .strip()
    )

    token_secret = {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {"name": f"cluster-{cluster_name}-client-token", "namespace": "default"},
        "stringData": {"token": token_decoded},
    }

    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as token_file:
        yaml.dump(token_secret, token_file)
        token_file.flush()
        _exec(
            "kubectl",
            "--context",
            f"k3d-{_control_plane_cluster_name}",
            "apply",
            "-f",
            token_file.name,
        )
    print(f"✅ Created secrets for cluster '{cluster_name}' in control plane")


if __name__ == "__main__":
    sys.exit(main())