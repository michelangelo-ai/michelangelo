"""Sandbox CLI for Michelangelo."""

import argparse
import base64
import contextlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
import uuid
from pathlib import Path
from typing import Optional

import yaml

short_description = "Manage the sandbox cluster."

description = """
Michelangelo Sandbox is a lightweight version of the Michelangelo platform,
tailored for local development and testing.
This tool helps you create and manage a sandbox cluster directly on your machine.
"""

_dir = Path(__file__).parent

_michelangelo_sandbox_kube_cluster_name = "michelangelo-sandbox"
_kube_ports = [
    "3306:30001",  # MySQL
    "9091:30007",  # MinIO
    "9090:30008",  # MinIO Console
    "14566:30009",  # Michelangelo API Server
    "8081:30010",  # Envoy gRPC --> gRPC-web proxy
    "8090:30011",  # Michelangelo UI
    "3000:30012",  # Grafana
    "9092:30015",  # Prometheus
    "5001:30013",  # MLflow Tracking Server
]

# Workflow engine ports
_cadence_ports = [
    "7833:30002",  # Cadence gRPC
    "7933:30003",  # Cadence TChannel
    "8088:30004",  # Cadence Web
]

# Ray framework ports
_ray_ports = [
    "10001:10001",  # Ray client port
    "8265:8265",  # Ray dashboard
]

_cadence_domain = "default"
_default_compute_kube_cluster_name = "michelangelo-compute-0"

# GKE cluster context name
_gke_context = "gke_michelanglo-oss-196506_us-east1_kubernetes-gke-dev01"

# Global flag to track if we're using GKE/external cluster mode
_use_gke_mode = False


def _get_kubectl_context_args() -> list[str]:
    """Get kubectl context arguments based on current mode.
    
    Returns ["--context", gke_context] for GKE mode,
    or ["--context", "k3d-{cluster_name}"] for k3d mode.
    """
    if _use_gke_mode:
        return ["--context", _gke_context]
    return ["--context", f"k3d-{_michelangelo_sandbox_kube_cluster_name}"]


def _sanitize_k8s_name(name: str) -> str:
    """Sanitize a name to be RFC 1123 compliant for Kubernetes resources."""
    # Replace underscores with dashes, lowercase
    return name.replace("_", "-").lower()


def _get_current_cluster_name() -> str:
    """Get the cluster name for use as cluster identifier."""
    if _use_gke_mode:
        return _sanitize_k8s_name(_gke_context)
    return _michelangelo_sandbox_kube_cluster_name


def init_arguments(p: argparse.ArgumentParser):
    """Initialize command-line arguments for the sandbox CLI."""
    sp = p.add_subparsers(dest="action", required=True)

    create_p = sp.add_parser("create", help="Create and start the cluster.")
    create_p.add_argument(
        "--exclude",
        help=(
            "Excludes specified services. "
            "Available options: apiserver, controllermgr, ui, worker, mysql, minio, prometheus, grafana, cadence, ray, spark"
        ),
        nargs="+",
        default=[],
    )
    create_p.add_argument(
        "--skip-cluster-creation",
        action="store_true",
        help="Skip k3d cluster creation and deploy to the current kubectl context (e.g., for GKE).",
    )
    create_p.add_argument(
        "--gke",
        action="store_true",
        help="Deploy to current GKE/external cluster context. Equivalent to --skip-cluster-creation with GHCR secret setup.",
    )
    create_p.add_argument(
        "--workflow",
        choices=["cadence", "temporal"],
        default="cadence",
        help="Choose workflow engine: cadence or temporal (default: cadence).",
    )
    create_p.add_argument(
        "--create-compute-cluster",
        action="store_true",
        help="Create an additional cluster for Ray jobs.",
    )
    create_p.add_argument(
        "--include-experimental",
        help="Include experimental services.",
        nargs="+",
        default=[],
    )
    create_p.add_argument(
        "--compute-cluster-name",
        default=_default_compute_kube_cluster_name,
        help=(
            f"Name of the compute cluster to create when "
            f"--create-compute-cluster is used "
            f"(default: {_default_compute_kube_cluster_name})."
        ),
    )

    demo_p = sp.add_parser(
        "demo", help="Create demo project and pipelines in the sandbox cluster."
    )
    demo_p.add_argument(
        "--gke",
        action="store_true",
        help="Run demo on GKE/external cluster instead of k3d.",
    )
    demo_sp = demo_p.add_subparsers(
        dest="demo_action", required=True, help="Demo type to create"
    )
    _ = demo_sp.add_parser("pipeline", help="Create pipeline demo resources")
    _ = demo_sp.add_parser("inference", help="Create inference server demo resources")
    _ = demo_sp.add_parser(
        "inference-dynamo",
        help="Create NVIDIA Dynamo inference server demo resources",
    )

    delete_p = sp.add_parser("delete", help="Delete the cluster.")
    delete_p.add_argument(
        "--compute-cluster-name",
        default=_default_compute_kube_cluster_name,
        help=(
            f"Name of the compute cluster to delete when "
            f"--create-compute-cluster is used "
            f"(default: {_default_compute_kube_cluster_name})."
        ),
    )
    delete_p.add_argument(
        "--gke",
        action="store_true",
        help="Delete sandbox resources from current GKE/external cluster context instead of k3d.",
    )
    _ = sp.add_parser("start", help="Start the cluster.")
    _ = sp.add_parser("stop", help="Stop the cluster.")


def main(args=None):
    """Main entry point for the sandbox CLI."""
    p = argparse.ArgumentParser(description=description)
    init_arguments(p)
    ns = p.parse_args(args=args)
    return run(ns)


def run(ns: argparse.Namespace):
    """Run the sandbox command based on the parsed namespace."""
    # Assert prerequisites. Sandbox depends on the following tools:
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

    raise ValueError(f"Unsupported action: {ns.action}")


def _create(ns: argparse.Namespace):
    assert ns

    # --gke implies --skip-cluster-creation
    skip_cluster = getattr(ns, "skip_cluster_creation", False) or getattr(ns, "gke", False)

    # Set global GKE mode flag
    global _use_gke_mode
    _use_gke_mode = skip_cluster

    if not skip_cluster:
        ports = _kube_ports + ([] if ns.workflow == "temporal" else _cadence_ports)
        args = [
            "k3d",
            "cluster",
            "create",
            _michelangelo_sandbox_kube_cluster_name,
            "--servers",
            "1",
            "--agents",
            "1",
        ]

        for p in ports:
            args += ["-p", f"{p}@agent:0"]

        # TODO: andrii: Remove the following block once Michelangelo is publicly accessible.
        # BLOCK START ----------------------------------------------------------------------
        # Handle the GitHub Container Registry authentication.
        env_cr_pat = "CR_PAT"
        cr_pat = os.environ.get(env_cr_pat)
        if not cr_pat:
            _err_exit(
                """
CR_PAT environment variable is not set. To pull Michelangelo's containers
from the GitHub Container Registry, please create a GitHub personal access
token (classic) with the "read:packages" scope. Then, save this token to the
CR_PAT environment variable, e.g.: `export CR_PAT=ghp_...`.

For a detailed guide, check:
https://docs.github.com/en/packages/working-with-a-github-packages-registry/
working-with-the-container-registry#authenticating-with-a-personal-access-token-classic.

Be aware that CR_PAT environment variable is required while Michelangelo is NOT
publicly accessible. Once we become public, the token will no longer be
necessary, and this assertion will be removed.
"""
            )

        # Create a temporary registry file with the GitHub Container Registry
        # authentication.
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

        with tempfile.NamedTemporaryFile(mode="wt", delete=False) as registry_file:
            json.dump(registry, registry_file)
            registry_file.flush()
            args += ["--registry-config", registry_file.name]

        _exec(*args)
    else:
        print("Skipping k3d cluster creation, using current kubectl context...")

        # For existing clusters (GKE, etc.), we need to create the GHCR image pull secret
        env_cr_pat = "CR_PAT"
        cr_pat = os.environ.get(env_cr_pat)
        if not cr_pat:
            _err_exit(
                """
CR_PAT environment variable is not set. To pull Michelangelo's containers
from the GitHub Container Registry, please create a GitHub personal access
token (classic) with the "read:packages" scope. Then, save this token to the
CR_PAT environment variable, e.g.: `export CR_PAT=ghp_...`.
"""
            )

        # Create GHCR image pull secret in default namespace
        print("Creating GHCR image pull secret...")
        subprocess.run(
            ["kubectl", "delete", "secret", "ghcr-secret", "--ignore-not-found"],
            check=False,
            capture_output=True,
        )
        _exec(
            "kubectl",
            "create",
            "secret",
            "docker-registry",
            "ghcr-secret",
            "--docker-server=ghcr.io",
            "--docker-username=ghcr-user",
            f"--docker-password={cr_pat}",
        )

        # Patch default service account to use the secret
        print("Patching default service account to use GHCR secret...")
        _exec(
            "kubectl",
            "patch",
            "serviceaccount",
            "default",
            "-p",
            '{"imagePullSecrets": [{"name": "ghcr-secret"}]}',
        )

    resources = [
        "boot.yaml",
        "michelangelo-config.yaml",
        "aws-credentials.yaml",
    ]
    links = []

    # MySQL (required for Cadence/Temporal unless excluded)
    if "mysql" not in ns.exclude:
        resources.append("mysql.yaml")

    # Cadence (requires mysql)

    if ns.workflow == "cadence" and "cadence" not in ns.exclude and "mysql" not in ns.exclude:
        resources.append("cadence.yaml")
        links.append(
            (
                "Cadence Web UI",
                "http://localhost:8088/domains/default/workflows",
                "",
            )
        )
    elif ns.workflow == "cadence" and ("cadence" in ns.exclude or "mysql" in ns.exclude):
        print("Note: Cadence excluded (explicitly or because mysql is excluded)")

    # MinIO

    if "minio" not in ns.exclude:
        resources.append("minio.yaml")
        links.append(
            (
                "MinIO Console",
                "http://localhost:9090",
                "[Username: minioadmin; Password: minioadmin]",
            )
        )

    # Prometheus & Grafana

    if "prometheus" not in ns.exclude:
        resources.append("prometheus.yaml")
        links.append(
            (
                "Prometheus",
                "http://localhost:9092",
                "",
            )
        )
    if "grafana" not in ns.exclude:
        resources.append("grafana.yaml")
        links.append(
            (
                "Grafana Dashboard",
                "http://localhost:3000",
                "[Username: admin; Password: admin]",
            )
        )

    if "apiserver" not in ns.exclude:
        resources.append("michelangelo-apiserver.yaml")
    if "controllermgr" not in ns.exclude:
        resources.append("michelangelo-controllermgr.yaml")
    if "ui" not in ns.exclude:
        resources.append("envoy.yaml")
        resources.append("michelangelo-ui.yaml")
        links.append(
            (
                "Michelangelo UI",
                "http://localhost:8090",
                "",
            )
        )

    if "fluent-bit" in ns.include_experimental:
        # Provision a ServiceAccount for fluent-bit DaemonSet execution.
        _exec(
            "kubectl",
            "create",
            "serviceaccount",
            "fluent-bit",
        )
        resources.extend(
            [
                "fluent-bit.yaml",
                "fluent-bit-config.yaml",
            ]
        )

    if "mlflow" in ns.include_experimental:
        resources.append("mlflow.yaml")
        links.append(
            (
                "MLflow Tracking Server",
                "http://localhost:5001",
                "",
            )
        )

    # Determine buckets to create based on enabled services
    # Only create bucket setup if minio is deployed
    if "minio" not in ns.exclude:
        bucket_names = ["logs", "default"]
        if "mlflow" in ns.include_experimental:
            bucket_names.append("mlflow")
            print("🪣 Adding MLflow bucket to S3 setup")

        # Create bucket setup with dynamic bucket list
        _create_bucket_setup(bucket_names)
    for r in resources:
        _kube_create(_dir / "resources" / r)

    _assert_command(
        "helm", "Helm not found, please install it: https://helm.sh/docs/intro/install/"
    )

    # Handle the case when helm repo list returns non-zero exit status (no repositories)
    try:
        helm_existing_repos = subprocess.check_output(["helm", "repo", "list"]).decode()
    except subprocess.CalledProcessError:
        # helm repo list returns non-zero exit status when no repositories
        # are configured
        helm_existing_repos = ""

    if "ray" not in ns.exclude:
        _create_kuberay_operator(helm_existing_repos)

    if "spark" not in ns.exclude:
        _create_spark_operator(helm_existing_repos)

    _kube_wait()

    if ns.workflow == "temporal":
        _setup_temporal(links, helm_existing_repos)
        if "worker" not in ns.exclude:
            _kube_create(_dir / "resources/michelangelo-temporal-worker.yaml")
    elif ns.workflow == "cadence":
        # Only create cadence domain if cadence is actually deployed
        if "cadence" not in ns.exclude and "mysql" not in ns.exclude:
            _create_cadence_domain(links)
            if "worker" not in ns.exclude:
                _kube_create(_dir / "resources/michelangelo-worker.yaml")
    else:
        raise ValueError(f"Unsupported workflow engine: {ns.workflow}")

    # Create separate compute cluster if requested
    if ns.create_compute_cluster:
        if _use_gke_mode:
            print("Warning: --create-compute-cluster is not supported in GKE mode.")
        else:
            _create_compute_cluster(ns.compute_cluster_name)
            _create_compute_cluster_crd(ns.compute_cluster_name)
            _apply_compute_cluster_rbac(ns.compute_cluster_name)
            _create_compute_cluster_secrets(ns.compute_cluster_name)
    else:
        # Use the control plane cluster as the default compute cluster if a
        # dedicated compute cluster is not requested
        if _use_gke_mode:
            cluster_name = _get_current_cluster_name()
        else:
            cluster_name = _michelangelo_sandbox_kube_cluster_name
        _create_compute_cluster_crd(cluster_name)
        _apply_compute_cluster_rbac(cluster_name)
        _create_compute_cluster_secrets(cluster_name)

    _kube_wait()

    print(
        "\n🚀 Sandbox created successfully. "
        "To access the services, please use the following links:\n"
    )
    for title, url, comment in links:
        print(f"  - {title}: {url} {comment}")

    print()


def _create_bucket_setup(bucket_names):
    """Create S3 bucket setup job with the provided bucket list."""
    bucket_names_str = ",".join(bucket_names)

    # Read the original bucket setup YAML
    original_bucket_setup_path = _dir / "resources" / "sandbox-bucket-setup.yaml"

    with open(original_bucket_setup_path) as f:
        content = f.read()

    # Replace the hardcoded bucket names with our dynamic list
    modified_content = content.replace(
        'value: "logs,default"', f'value: "{bucket_names_str}"'
    )

    # Create temporary file with modified content
    with tempfile.NamedTemporaryFile(
        mode="w", suffix=".yaml", delete=False
    ) as temp_file:
        temp_file.write(modified_content)
        temp_file.flush()

        # Apply the modified bucket setup
        _exec("kubectl", "create", "-f", temp_file.name)

    print(f"📦 Created bucket setup job with buckets: {bucket_names_str}")


def _create_spark_operator(helm_existing_repos):
    if "spark-operator" not in helm_existing_repos:
        _exec(
            "helm",
            "repo",
            "add",
            "spark-operator",
            "https://kubeflow.github.io/spark-operator",
        )
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
        "--timeout",
        "20m",
    )


def _create_kuberay_operator(helm_existing_repos):
    """Create the KubeRay operator using Helm.

    Reference:
    https://docs.ray.io/en/releases-2.49.1/cluster/kubernetes/getting-started/
    kuberay-operator-installation.html#method-1-helm-recommended.
    """
    if "kuberay" not in helm_existing_repos:
        _exec(
            "helm",
            "repo",
            "add",
            "kuberay",
            "https://ray-project.github.io/kuberay-helm",
        )
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
        "--timeout",
        "20m",
    )


def _setup_temporal(links, helm_existing_repos):
    if "temporal" not in helm_existing_repos:
        _exec(
            "helm",
            "repo",
            "add",
            "temporal",
            "https://temporalio.github.io/helm-charts",
        )
        _exec("helm", "repo", "update")

    # Wait for MySQL to be ready before installing Temporal
    print("Waiting for MySQL to be ready...")
    _exec(
        "kubectl",
        "wait",
        "--for=condition=ready",
        "pod",
        "mysql",
        "--timeout=300s",
    )

    # Wait for MySQL to accept connections
    print("Waiting for MySQL to accept connections...")
    _exec(
        "kubectl",
        "exec",
        "mysql",
        "--",
        "mysqladmin",
        "ping",
        "-u",
        "root",
        "-proot",
        "--silent",
        "--wait",
    )

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
        "-l",
        "app",
        "--timeout=600s",
    )

    print("Waiting for Temporal admin tools to be ready...")
    _exec(
        "kubectl",
        "wait",
        "--for=condition=ready",
        "pod",
        "-l",
        "app.kubernetes.io/component=admintools,app.kubernetes.io/instance=temporaltest",
        "--timeout=300s",
    )

    print("Creating database schemas via Temporal admin tools...")

    # Create both temporal databases explicitly
    print("Creating temporal and temporal_visibility databases...")
    _exec(
        "kubectl",
        "exec",
        "mysql",
        "--",
        "mysql",
        "-u",
        "root",
        "-proot",
        "-e",
        "CREATE DATABASE IF NOT EXISTS temporal;",
    )
    _exec(
        "kubectl",
        "exec",
        "mysql",
        "--",
        "mysql",
        "-u",
        "root",
        "-proot",
        "-e",
        "CREATE DATABASE IF NOT EXISTS temporal_visibility;",
    )

    # Setup temporal database schema
    print("Setting up temporal database schema...")
    _exec(
        "kubectl",
        "exec",
        "deployment/temporaltest-admintools",
        "--",
        "env",
        "MYSQL_HOST=mysql",
        "MYSQL_PORT=3306",
        "MYSQL_USER=root",
        "MYSQL_PWD=root",
        "temporal-sql-tool",
        "--endpoint",
        "mysql",
        "--port",
        "3306",
        "--user",
        "root",
        "--password",
        "root",
        "--database",
        "temporal",
        "setup-schema",
        "-v",
        "0.0",
    )
    _exec(
        "kubectl",
        "exec",
        "deployment/temporaltest-admintools",
        "--",
        "env",
        "MYSQL_HOST=mysql",
        "MYSQL_PORT=3306",
        "MYSQL_USER=root",
        "MYSQL_PWD=root",
        "temporal-sql-tool",
        "--endpoint",
        "mysql",
        "--port",
        "3306",
        "--user",
        "root",
        "--password",
        "root",
        "--database",
        "temporal",
        "update-schema",
        "-d",
        "/etc/temporal/schema/mysql/v8/temporal/versioned",
    )

    # Setup temporal visibility database schema
    print("Setting up temporal_visibility database schema...")
    _exec(
        "kubectl",
        "exec",
        "deployment/temporaltest-admintools",
        "--",
        "env",
        "MYSQL_HOST=mysql",
        "MYSQL_PORT=3306",
        "MYSQL_USER=root",
        "MYSQL_PWD=root",
        "temporal-sql-tool",
        "--endpoint",
        "mysql",
        "--port",
        "3306",
        "--user",
        "root",
        "--password",
        "root",
        "--database",
        "temporal_visibility",
        "setup-schema",
        "-v",
        "0.0",
    )
    _exec(
        "kubectl",
        "exec",
        "deployment/temporaltest-admintools",
        "--",
        "env",
        "MYSQL_HOST=mysql",
        "MYSQL_PORT=3306",
        "MYSQL_USER=root",
        "MYSQL_PWD=root",
        "temporal-sql-tool",
        "--endpoint",
        "mysql",
        "--port",
        "3306",
        "--user",
        "root",
        "--password",
        "root",
        "--database",
        "temporal_visibility",
        "update-schema",
        "-d",
        "/etc/temporal/schema/mysql/v8/visibility/versioned",
    )

    print("Database schemas created. Restarting Temporal...")
    # Restart Temporal to apply the schemas
    _exec("helm", "uninstall", "temporaltest")
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
        "-l",
        "app",
        "--timeout=600s",
    )

    # Wait for admin tools to be fully ready and get specific pod name
    print("Waiting for admin tools to be ready for commands...")
    time.sleep(10)  # Increased wait time

    # Get the specific admin tools pod name for more reliable exec
    admin_pod_result = subprocess.check_output(
        [
            "kubectl",
            "get",
            "pod",
            "-l",
            "app.kubernetes.io/component=admintools,app.kubernetes.io/instance=temporaltest",
            "-o",
            "jsonpath={.items[0].metadata.name}",
        ],
        text=True,
    ).strip()

    # Register the default namespace in Temporal using specific pod name
    _exec(
        "kubectl",
        "exec",
        admin_pod_result,
        "-c",
        "admin-tools",
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
    # Automatically port-forward Temporal Web UI in the background
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
        env={
            "CADENCE_CLI_ADDRESS": "cadence:7933",
        },
        retry_attempts=3,
    )


def _create_demo_crs(ns: argparse.Namespace):
    """Create demo Custom Resources (CRs) for the sandbox environment."""
    assert ns
    if ns.demo_action not in ("pipeline", "inference", "inference-dynamo"):
        raise ValueError(f"Unsupported demo action: {ns.demo_action}")

    # Set GKE mode if --gke flag is specified
    global _use_gke_mode
    _use_gke_mode = getattr(ns, "gke", False)

    # Check if cluster exists
    if _use_gke_mode:
        # For GKE, just check if kubectl can connect
        try:
            _exec("kubectl", "--context", _gke_context, "cluster-info", raise_error=True)
        except subprocess.CalledProcessError:
            _err_exit(
                f"Cannot connect to GKE cluster {_gke_context}. "
                "Please check your kubectl configuration."
            )
    else:
        try:
            _exec(
                "k3d",
                "cluster",
                "get",
                _michelangelo_sandbox_kube_cluster_name,
                raise_error=True,
            )
        except subprocess.CalledProcessError:
            _err_exit(
                f"Cluster {_michelangelo_sandbox_kube_cluster_name} not found. "
                "Please run 'ma sandbox create' first."
            )

        # Check if cluster is running
        try:
            _exec("kubectl", "cluster-info", raise_error=True)
        except subprocess.CalledProcessError:
            _err_exit(
                f"Cluster {_michelangelo_sandbox_kube_cluster_name} is not running. "
                "Please run 'ma sandbox start' first."
            )

    # Create CRs used by all demo resources
    demo_dir = _dir / "demo"
    project_yaml_path = demo_dir / "project.yaml"

    # Extract namespace from project.yaml
    with open(project_yaml_path) as f:
        project_yaml = yaml.safe_load(f)
    namespace = project_yaml.get("metadata", {}).get("namespace", "default")

    # Ensure namespace exists
    _ensure_namespace_exists(namespace)

    # Create Project CR
    # Note: The Project CRD is essentially the "parent" of other CRDs. Under
    # normal circumstances, users must create a project CR before creating other CRs.
    if project_yaml_path.exists():
        _kube_apply(project_yaml_path)
    else:
        _err_exit(f"❌ Project CR not found at {project_yaml_path}, exiting...")

    if ns.demo_action == "pipeline":
        _create_pipeline_demo_crs()
    elif ns.demo_action == "inference":
        _create_inference_demo_crs()
    elif ns.demo_action == "inference-dynamo":
        _create_inference_dynamo_demo_crs()
    else:
        raise ValueError(f"Unsupported demo action: {ns.demo_action}")


def _delete(ns: argparse.Namespace):
    assert ns

    # If --gke flag is set, delete resources from current kubectl context
    if getattr(ns, "gke", False):
        _delete_gke_resources()
        return

    # Determine which compute cluster to check for
    compute_cluster = (
        ns.compute_cluster_name
        if ns.compute_cluster_name
        else _default_compute_kube_cluster_name
    )

    # Check if compute cluster exists before attempting to delete
    try:
        subprocess.check_output(
            ["k3d", "cluster", "get", compute_cluster], stderr=subprocess.DEVNULL
        )
        # Cluster exists, delete it
        _exec("k3d", "cluster", "delete", compute_cluster)
    except subprocess.CalledProcessError:
        # Cluster doesn't exist, skip deletion
        print(f"Compute cluster '{compute_cluster}' not found, skipping deletion.")

    # Always try to delete the main sandbox cluster
    _exec("k3d", "cluster", "delete", _michelangelo_sandbox_kube_cluster_name)


def _delete_gke_resources():
    """Delete sandbox resources from current GKE/external cluster context."""
    print("Deleting sandbox resources from current kubectl context...")

    context_args = ["--context", _gke_context]

    # Delete self-provisioned Dynamo resources and Michelangelo CRs
    # Use timeout to prevent hanging on finalizers
    print("Cleaning up Dynamo self-provisioned resources...")

    # Helper to delete with timeout and finalizer removal fallback
    def _delete_with_timeout(args: list, timeout: int = 10):
        with contextlib.suppress(subprocess.TimeoutExpired):
            subprocess.run(args, check=False, capture_output=True, timeout=timeout)

    def _remove_finalizers_and_delete(resource_type: str, namespace: str = None):
        """Remove finalizers from resources then delete them."""
        ns_args = ["-n", namespace] if namespace else ["-A"]
        # Get all resources of this type
        result = subprocess.run(
            ["kubectl", *context_args, "get", resource_type, *ns_args, "-o", "jsonpath={range .items[*]}{.metadata.name}{' '}{.metadata.namespace}{'\\n'}{end}"],
            check=False,
            capture_output=True,
            text=True,
            timeout=10,
        )
        if result.returncode == 0 and result.stdout.strip():
            for line in result.stdout.strip().split("\n"):
                parts = line.split()
                if len(parts) >= 1:
                    name = parts[0]
                    ns = parts[1] if len(parts) > 1 else namespace or "default"
                    # Patch to remove finalizers
                    subprocess.run(
                        ["kubectl", *context_args, "patch", resource_type, name, "-n", ns,
                         "--type=merge", "-p", '{"metadata":{"finalizers":null}}'],
                        check=False,
                        capture_output=True,
                        timeout=10,
                    )
                    # Delete the resource
                    subprocess.run(
                        ["kubectl", *context_args, "delete", resource_type, name, "-n", ns,
                         "--force", "--grace-period=0"],
                        check=False,
                        capture_output=True,
                        timeout=10,
                    )

    # Try normal delete first with timeout, then remove finalizers if needed
    # Delete Michelangelo Deployment CRs first (they reference InferenceServers)
    print("Cleaning up Michelangelo Deployment resources...")
    with contextlib.suppress(Exception):
        _delete_with_timeout([
            "kubectl", *context_args, "delete", "deployment.michelangelo.api",
            "--all", "-n", "default", "--force", "--grace-period=0"
        ])
    _remove_finalizers_and_delete("deployment.michelangelo.api", "default")

    # Delete Michelangelo InferenceServer CRs
    print("Cleaning up Michelangelo InferenceServer resources...")
    with contextlib.suppress(Exception):
        _delete_with_timeout([
            "kubectl", *context_args, "delete", "inferenceserver.michelangelo.api",
            "--all", "-n", "default", "--force", "--grace-period=0"
        ])
    _remove_finalizers_and_delete("inferenceserver.michelangelo.api", "default")

    # Delete self-provisioned Dynamo deployments (created by our controller)
    # Label: app.kubernetes.io/managed-by=michelangelo-self-provision
    print("Cleaning up self-provisioned Dynamo deployments...")
    _delete_with_timeout(
        ["kubectl", *context_args, "delete", "deployment", "-n", "default",
         "-l", "app.kubernetes.io/managed-by=michelangelo-self-provision",
         "--force", "--grace-period=0"]
    )

    # Delete self-provisioned Dynamo services
    print("Cleaning up self-provisioned Dynamo services...")
    _delete_with_timeout(
        ["kubectl", *context_args, "delete", "svc", "-n", "default",
         "-l", "app.kubernetes.io/managed-by=michelangelo-self-provision",
         "--force", "--grace-period=0"]
    )

    # Also delete by name pattern for any orphaned dynamo-sp deployments
    try:
        result = subprocess.run(
            ["kubectl", *context_args, "get", "deployment", "-n", "default", "-o", "name"],
            check=False,
            capture_output=True,
            text=True,
            timeout=10,
        )
        if result.returncode == 0:
            for line in result.stdout.strip().split("\n"):
                if "dynamo-sp-" in line:
                    _delete_with_timeout(
                        ["kubectl", *context_args, "delete", line, "-n", "default",
                         "--force", "--grace-period=0"]
                    )
    except subprocess.TimeoutExpired:
        pass

    # Delete orphaned dynamo-sp services by name pattern
    try:
        result = subprocess.run(
            ["kubectl", *context_args, "get", "svc", "-n", "default", "-o", "name"],
            check=False,
            capture_output=True,
            text=True,
            timeout=10,
        )
        if result.returncode == 0:
            for line in result.stdout.strip().split("\n"):
                if "dynamo-sp-" in line:
                    _delete_with_timeout(
                        ["kubectl", *context_args, "delete", line, "-n", "default",
                         "--force", "--grace-period=0"]
                    )
    except subprocess.TimeoutExpired:
        pass

    # Delete any blocking CRD instances first (like RayClusters)
    print("Cleaning up RayCluster instances...")
    subprocess.run(
        ["kubectl", *context_args, "delete", "raycluster.michelangelo.api", "--all", "-n", "default"],
        check=False,
        capture_output=True,
    )

    # Delete pods
    pods_to_delete = [
        "michelangelo-apiserver",
        "michelangelo-controllermgr",
        "envoy",
        "cadence",
        "cadence-web",
        "mysql",
        "minio",
        "prometheus",
        "grafana",
    ]
    for pod in pods_to_delete:
        subprocess.run(
            ["kubectl", *context_args, "delete", "pod", pod, "--ignore-not-found"],
            check=False,
            capture_output=True,
        )

    # Delete Gateway CR first (triggers Istio to clean up ma-gateway-istio deployment)
    print("Cleaning up Istio Gateway...")
    subprocess.run(
        ["kubectl", *context_args, "delete", "gateway.gateway.networking.k8s.io", "ma-gateway", "-n", "default", "--ignore-not-found"],
        check=False,
        capture_output=True,
    )

    # Delete deployments
    subprocess.run(
        ["kubectl", *context_args, "delete", "deployment", "michelangelo-ui", "--ignore-not-found"],
        check=False,
        capture_output=True,
    )
    subprocess.run(
        ["kubectl", *context_args, "delete", "deployment", "ma-gateway-istio", "-n", "default", "--ignore-not-found"],
        check=False,
        capture_output=True,
    )

    # Delete services
    services_to_delete = [
        "michelangelo-apiserver",
        "michelangelo-controllermgr",
        "envoy",
        "michelangelo-ui",
        "cadence",
        "cadence-web",
        "mysql",
        "minio",
        "prometheus",
        "grafana",
        "ma-gateway-istio",
    ]
    for svc in services_to_delete:
        subprocess.run(
            ["kubectl", *context_args, "delete", "svc", svc, "--ignore-not-found"],
            check=False,
            capture_output=True,
        )

    # Delete configmaps
    configmaps_to_delete = [
        "michelangelo-config",
        "michelangelo-apiserver-config",
        "michelangelo-controllermgr-config",
        "envoy-config",
        "public-config",
        "sandbox-bucket-setup",
    ]
    for cm in configmaps_to_delete:
        subprocess.run(
            ["kubectl", *context_args, "delete", "configmap", cm, "--ignore-not-found"],
            check=False,
            capture_output=True,
        )

    # Delete secrets
    subprocess.run(
        ["kubectl", *context_args, "delete", "secret", "aws-credentials", "ghcr-secret", "--ignore-not-found"],
        check=False,
        capture_output=True,
    )

    # Delete jobs
    subprocess.run(
        ["kubectl", *context_args, "delete", "job", "sandbox-bucket-setup", "--ignore-not-found"],
        check=False,
        capture_output=True,
    )

    # Delete clusterrolebinding
    subprocess.run(
        ["kubectl", *context_args, "delete", "clusterrolebinding", "admin-default-default", "--ignore-not-found"],
        check=False,
        capture_output=True,
    )

    # Uninstall helm releases
    print("Uninstalling helm releases...")
    subprocess.run(
        ["helm", "uninstall", "kuberay-operator", "-n", "ray-system", "--kube-context", _gke_context],
        check=False,
        capture_output=True,
    )
    subprocess.run(
        ["helm", "uninstall", "spark-operator", "-n", "spark-operator", "--kube-context", _gke_context],
        check=False,
        capture_output=True,
    )

    # Clean up Dynamo discovery CRD and RBAC
    print("  Cleaning up Dynamo discovery CRD and RBAC...")
    subprocess.run(
        ["kubectl", *context_args, "delete", "clusterrolebinding",
         "dynamo-discovery-default", "--ignore-not-found"],
        check=False,
        capture_output=True,
    )
    subprocess.run(
        ["kubectl", *context_args, "delete", "clusterrole",
         "dynamo-discovery", "--ignore-not-found"],
        check=False,
        capture_output=True,
    )
    subprocess.run(
        ["kubectl", *context_args, "delete", "crd",
         "dynamoworkermetadatas.nvidia.com", "--ignore-not-found"],
        check=False,
        capture_output=True,
    )

    # Clean up any orphaned Dynamo replicasets in default namespace
    result = subprocess.run(
        ["kubectl", *context_args, "get", "replicaset", "-n", "default", "-o", "name"],
        check=False,
        capture_output=True,
        text=True,
    )
    if result.returncode == 0:
        for line in result.stdout.strip().split("\n"):
            if "dynamo-sp-" in line:
                subprocess.run(
                    ["kubectl", *context_args, "delete", line, "-n", "default",
                     "--force", "--grace-period=0"],
                    check=False,
                    capture_output=True,
                )

    print("GKE sandbox resources deleted.")


def _start(ns: argparse.Namespace):
    assert ns
    _exec("k3d", "cluster", "start", _michelangelo_sandbox_kube_cluster_name)


def _stop(ns: argparse.Namespace):
    assert ns
    _exec("k3d", "cluster", "stop", _michelangelo_sandbox_kube_cluster_name)


def _kube_create(path: Path):
    context_args = _get_kubectl_context_args()
    _exec("kubectl", *context_args, "create", "-f", str(path))


def _kube_apply(path: Path):
    context_args = _get_kubectl_context_args()
    _exec("kubectl", *context_args, "apply", "-f", str(path))


def _kube_wait(pods: bool = True, jobs: bool = True):
    context_args = _get_kubectl_context_args()
    if pods:
        # Wait for all non-job pods to be ready
        _exec(
            "kubectl",
            *context_args,
            "wait",
            "--for=condition=ready",
            "pod",
            "-l",
            "app",
            "--timeout=600s",
        )
    if jobs:
        _exec(
            "kubectl",
            *context_args,
            "wait",
            "--all",
            "jobs",
            "--for=condition=complete",
            "--timeout=600s",
        )


def _apply_compute_cluster_rbac(cluster_name: str):
    """Apply RBAC for Ray management in the compute cluster.

    This creates the ServiceAccount `ray-manager`, a namespaced Role with permissions on
    Ray resources, and a RoleBinding to bind them, in the `default` namespace of the
    jobs cluster.
    """
    rbac_path = _dir / "resources" / "rbac-ray.yaml"
    # For GKE mode, use GKE context; for k3d, use the specific cluster context
    if _use_gke_mode:
        context_args = ["--context", _gke_context]
    else:
        context_args = ["--context", f"k3d-{cluster_name}"]
    _exec(
        "kubectl",
        *context_args,
        "apply",
        "-f",
        str(rbac_path),
    )


def _kube_run(
    image: str,
    command: list[str],
    env: Optional[dict[str, str]] = None,
    retry_attempts: int = 0,
):
    assert image
    assert command

    args = [
        "kubectl",
        "run",
        uuid.uuid4().hex,  # Pod's name.
        "--restart=Never",  # The restart policy for the Pod.
        "--rm",  # Delete the pod after it exits.
        "--stdin",  # Keep stdin open on the container in the pod,
        # allowing the command to block until completion.
        "--image",
        image,
    ]
    if env:
        args += [f"--env={k}={v}" for k, v in env.items()]

    args += [
        "--command",
        "--",
        *command,
    ]
    return _exec(*args, retry_attempts=retry_attempts)


def _exec(
    *args,
    retry_attempts: int = 0,
    retry_delay_seconds: int = 5,
    raise_error: bool = False,
):
    """Execute a shell command with optional retries.

    If the command exits with a non-zero code, it will be retried up to
    retry_attempts times, waiting retry_delay_seconds between attempts.

    Parameters:
        *args: Variable-length argument list representing the command to run
            and its arguments.
        retry_attempts: Number of times to retry the command on failure.
            Defaults to 0 (no retry).
        retry_delay_seconds: Number of seconds to wait between retries.
            Defaults to 5.
        raise_error: Determines how to handle errors after the final retry.
            If True, the function will raise a subprocess.CalledProcessError.
            If False, the function will terminate the program with the exit
            code of the failed command. Defaults to False.

    Returns:
        None.

    Raises:
        subprocess.CalledProcessError: If the command fails after all retries
            and raise_error is True.

    Examples:
        - Basic usage with a single command: _exec("ls", "-l", "~/bin")
        - Run a script with retries: _exec("bash", "my_script.sh",
          retry_attempts=3, retry_delay_seconds=2)

    Side Effects:
        - Prints the command being executed and retry messages if any.
        - Terminates the program if raise_error is False and retries are
          exhausted.
    """
    for i in range(retry_attempts + 1):
        try:
            print("[+]", " ".join(args))
            subprocess.check_call(args)
            return
        except subprocess.CalledProcessError as e:
            if i == retry_attempts:
                # This was the last attempt, either re-raise or exit.
                if raise_error:
                    raise e
                else:
                    _err_exit("command failed", code=e.returncode)

            # Wait before the next attempt.
            print("retrying after", retry_delay_seconds, "seconds...")
            time.sleep(retry_delay_seconds)


def _assert_command(command: str, err_message: str):
    if shutil.which(command) is None:
        _err_exit(err_message)


def _err_exit(err_message: str, code: int = 1):
    # Print the error message in red and bold.
    print(f"\033[91m\033[1mERROR: {err_message}\nexit {code}\033[0m")
    sys.exit(code)


def _create_compute_cluster(cluster_name: str):
    """Create a dedicated compute cluster for running Ray jobs.

    This function sets up a separate Kubernetes cluster specifically for executing
    Ray workloads. The compute cluster includes:

    Infrastructure Components:
    - k3d cluster with 1 server and 2 agent nodes
    - KubeRay operator for managing Ray clusters
    - RBAC permissions for ray-manager service account

    Storage Configuration (required for Ray jobs):
    - michelangelo-config ConfigMap (S3 endpoint and credentials)
    - aws-credentials Secret (for AWS CLI access)

    Network Configuration:
    - Ray client port: 10001
    - Ray dashboard: 8265

    Note: Ray pods reference the michelangelo-config ConfigMap via envFrom,
    which is why storage must be set up in the compute cluster.

    Args:
        cluster_name: Name of the k3d cluster to create
    """
    args = [
        "k3d",
        "cluster",
        "create",
        cluster_name,
        "--servers",
        "1",
        "--agents",
        "2",  # More worker nodes for Ray
        "--kubeconfig-switch-context=false",  # Don't switch kubectl context
        "--network",
        f"k3d-{_michelangelo_sandbox_kube_cluster_name}",
        # Use the same network as the control plane
    ]

    # Add port mappings for Ray
    for p in _ray_ports:
        args += ["-p", f"{p}@agent:0"]

    _exec(*args)

    # Add kuberay operator to the jobs cluster
    _exec(
        "helm",
        "install",
        "--kube-context",
        f"k3d-{cluster_name}",
        "kuberay-operator",
        "kuberay/kuberay-operator",
        "--version",
        "1.4.2",
        "--namespace",
        "ray-system",
        "--create-namespace",
        "--wait",
        "--timeout",
        "20m",
    )

    # Create michelangelo-config ConfigMap pointing to control plane's MinIO
    _create_config_in_compute_cluster(cluster_name)

    # Create aws-credentials Secret
    _create_aws_credentials_in_cluster(cluster_name)

    print(
        f"\nJobs cluster '{cluster_name}' created successfully "
        "configured to use control plane storage."
    )


def _create_config_in_compute_cluster(cluster_name: str):
    """Create michelangelo-config ConfigMap in compute cluster."""
    config_path = _dir / "resources" / "michelangelo-config.yaml"

    with open(config_path) as f:
        config_data = yaml.safe_load(f)

    # Update MinIO endpoint to point to the control plane's MinIO within the shared
    # network k3d-michelangelo-sandbox-agent-0 is the hostname of the control plane's
    # agent node. 30007 is the NodePort for MinIO API service.
    if "data" in config_data:
        config_data["data"]["AWS_ENDPOINT_URL"] = (
            f"http://k3d-{_michelangelo_sandbox_kube_cluster_name}-agent-0:30007"
        )

    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as temp_config:
        yaml.dump(config_data, temp_config)
        temp_config.flush()

        _exec(
            "kubectl",
            "--context",
            f"k3d-{cluster_name}",
            "apply",
            "-f",
            temp_config.name,
        )

    print(f"Created michelangelo-config ConfigMap in cluster '{cluster_name}'")


def _create_aws_credentials_in_cluster(cluster_name: str):
    """Create aws-credentials Secret in compute cluster."""
    _exec(
        "kubectl",
        "--context",
        f"k3d-{cluster_name}",
        "apply",
        "-f",
        str(_dir / "resources" / "aws-credentials.yaml"),
    )
    print(f"Created aws-credentials Secret in cluster '{cluster_name}'")


def _ensure_namespace_exists(namespace: str):
    """Ensure the namespace exists in the sandbox cluster."""
    context_args = _get_kubectl_context_args()
    try:
        # Check if namespace already exists
        subprocess.check_output(
            ["kubectl", *context_args, "get", "namespace", namespace],
            stderr=subprocess.DEVNULL,
        )
        print(f"Namespace '{namespace}' already exists.")
    except subprocess.CalledProcessError:
        # Namespace doesn't exist, create it
        _exec(
            "kubectl",
            *context_args,
            "create",
            "namespace",
            namespace,
        )
        print(f"Created namespace '{namespace}' in the sandbox cluster.")


def _get_kubeconfig(cluster_name: str) -> str:
    """Get kubeconfig for the specified cluster.
    
    In GKE mode, returns the GKE context's kubeconfig.
    In k3d mode, returns the k3d cluster's kubeconfig.
    """
    if _use_gke_mode:
        return subprocess.check_output(
            ["kubectl", "config", "view", "--minify", "--raw",
             "--context", _gke_context]
        ).decode()
    else:
        return subprocess.check_output(
            ["k3d", "kubeconfig", "get", cluster_name]
        ).decode()


# Given a cluster name, create a Cluster CRD in the sandbox cluster
def _create_compute_cluster_crd(cluster_name: str):
    """Create a Cluster CRD for the Ray jobs cluster in the sandbox cluster."""
    # Ensure ma-system namespace exists
    _ensure_namespace_exists("ma-system")

    # Get kubeconfig for the Ray jobs cluster
    kubeconfig = _get_kubeconfig(cluster_name)

    # Parse the kubeconfig YAML
    kubeconfig_data = yaml.safe_load(kubeconfig)

    # Extract server URL from clusters[0].cluster.server
    server_url = kubeconfig_data["clusters"][0]["cluster"]["server"]

    # Extract host and port from server URL
    # Example: "https://host.docker.internal:52910" or "https://34.26.98.208"
    import re

    # Try with explicit port first
    match = re.search(r"(https://[^:/]+):(\d+)", server_url)
    if match:
        host, port = match.groups()
    else:
        # No port specified, use default HTTPS port 443
        match = re.search(r"(https://[^:/]+)", server_url)
        if not match:
            raise ValueError(
                f"Could not extract cluster host from server URL: {server_url}"
            )
        host = match.group(1)
        port = "443"

    # Create Cluster CRD manifest
    cluster_crd = {
        "apiVersion": "michelangelo.api/v2",
        "kind": "Cluster",
        "metadata": {"name": cluster_name, "namespace": "ma-system"},
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

    # Create a temporary file for the Cluster CRD
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as crd_file:
        yaml.dump(cluster_crd, crd_file)
        crd_file.flush()

        # Apply the Cluster CRD to the sandbox cluster
        context_args = _get_kubectl_context_args()
        _exec(
            "kubectl",
            *context_args,
            "apply",
            "-f",
            crd_file.name,
        )

        print(f"\nCreated Cluster CRD '{cluster_name}' in the sandbox cluster")
        print(f"Cluster host: {host}")
        print(f"Cluster port: {port}")
        print(f"Server URL: {server_url}")


def _create_compute_cluster_secrets(cluster_name: str):
    """Create Kubernetes secrets for the kubeconfig of the given cluster name."""
    # Get kubeconfig for the cluster
    kubeconfig = _get_kubeconfig(cluster_name)

    # Parse the kubeconfig YAML
    kubeconfig_data = yaml.safe_load(kubeconfig)

    # Extract certificate-authority-data from clusters[0].cluster
    ca_data = kubeconfig_data["clusters"][0]["cluster"].get(
        "certificate-authority-data"
    )
    if not ca_data:
        raise ValueError("certificate-authority-data not found in kubeconfig")
    ca_data_decoded = base64.b64decode(ca_data).decode()

    # Create a secret for the certificate-authority-data
    ca_secret = {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {"name": f"cluster-{cluster_name}-ca-data", "namespace": "default"},
        "stringData": {"cadata": ca_data_decoded},
    }

    # Create a temporary file for the CA secret
    context_args = _get_kubectl_context_args()
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as ca_file:
        yaml.dump(ca_secret, ca_file)
        ca_file.flush()

        # Apply the CA secret to the sandbox cluster
        _exec(
            "kubectl",
            *context_args,
            "apply",
            "-f",
            ca_file.name,
        )

    # Create a new token for the ray-manager service account in the jobs cluster
    # For GKE mode, use GKE context; for k3d, use the specific cluster context
    if _use_gke_mode:
        token_cmd = ["kubectl", "--context", _gke_context, "-n", "default",
                     "create", "token", "ray-manager", "--duration=87600h"]
    else:
        token_cmd = ["kubectl", "--context", f"k3d-{cluster_name}", "-n", "default",
                     "create", "token", "ray-manager", "--duration=87600h"]
    token_decoded = subprocess.check_output(token_cmd).decode().strip()

    # Create a secret for the user token
    token_secret = {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {
            "name": f"cluster-{cluster_name}-client-token",
            "namespace": "default",
        },
        "stringData": {"token": token_decoded},
    }

    # Create a temporary file for the token secret
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as token_file:
        yaml.dump(token_secret, token_file)
        token_file.flush()

        # Apply the token secret to the sandbox cluster
        _exec(
            "kubectl",
            *context_args,
            "apply",
            "-f",
            token_file.name,
        )

    print(f"\nCreated secrets for cluster '{cluster_name}' in the sandbox cluster")


def _create_inference_demo_crs():
    """Create an inference server for the sandbox cluster for demo purposes."""
    print("🚀 Setting up Michelangelo AI Inference Demo...")

    # Setup istio with Gateway API
    # This allows usage of HTTPRoutes to route traffic to the inference server.
    _setup_istio_with_gateway_api()

    inference_demo_dir = _dir / "demo" / "inference"
    # Create inference server CR
    inference_server_path = inference_demo_dir / "inferenceserver.yaml"
    if not inference_server_path.exists():
        _err_exit(
            f"❌ Inference server CR not found at {inference_server_path}, exiting..."
        )

    print("✅ Creating Triton Inference Server...")
    _kube_apply(inference_server_path)

    # Wait for inference server to reach SERVING state (image pull may take time)
    with open(inference_server_path) as f:
        inference_server_yaml = yaml.safe_load(f)
    inference_server_name = inference_server_yaml["metadata"]["name"]
    inference_server_namespace = inference_server_yaml["metadata"].get(
        "namespace", "default"
    )

    print(f"⏳ Waiting for inference server '{inference_server_name}' to be ready...")
    print("   (This may take 5-10 minutes for first-time Triton image pull)")

    context_args = _get_kubectl_context_args()
    try:
        _exec(
            "kubectl",
            *context_args,
            "wait",
            "--for=jsonpath=.status.state=INFERENCE_SERVER_STATE_SERVING",
            f"inferenceservers.michelangelo.api/{inference_server_name}",
            "-n",
            inference_server_namespace,
            "--timeout=720s",
            raise_error=True,
        )
        print("✅ Inference server is ready!")
    except subprocess.CalledProcessError:
        _err_exit(
            f"Inference server '{inference_server_name}'\
                failed to become ready after 720s.\n"
            f"Check status with:\n"
            f"kubectl get inferenceservers.michelangelo.api\
                {inference_server_name} -n {inference_server_namespace} -o yaml\n"
            f"Check logs with:\
                kubectl logs -l app=inference-server -n {inference_server_namespace}"
        )

    # Deploy model-sync Deployment
    model_sync_deployment_path = _dir / "resources" / "model-sync.yaml"
    if not model_sync_deployment_path.exists():
        _err_exit(
            f"❌ Model-sync Deployment not found at {model_sync_deployment_path},\
                exiting..."
        )

    print("✅ Deploying model-sync Deployment...")
    _kube_apply(model_sync_deployment_path)

    # Wait for Deployment to be ready
    print("⏳ Waiting for model-sync Deployment to be ready...")
    try:
        _exec(
            "kubectl",
            *context_args,
            "rollout",
            "status",
            "deployment/model-sync",
            "-n",
            "default",
            "--timeout=60s",
            raise_error=True,
        )
        print("✅ Model-sync Deployment is ready!")
    except subprocess.CalledProcessError:
        _err_exit(
            "Model-sync Deployment failed to become ready after 60s.\n"
            "Check status with:\n"
            "kubectl get deployments model-sync -n default -o yaml\n"
            "Check logs with: kubectl logs deployment/model-sync -n default"
        )

    print("✅ Inference demo resources created successfully")

    print("🎉 Inference demo deployment created successfully!")
    print("📋 What was set up:")
    print("  • Gateway API with Istio integration")
    print("  • HTTPRoute for traffic routing")
    print("  • Triton Inference Server")
    print("  • Model-sync Deployment (handles S3 sync and model loading)")

    print(
        "🌐 Deployment-agnostic endpoint:\
            Use the following URL to test the inference server"
    )
    print("  http://localhost:8080/inference-server-example")
    print(
        "  For example,\
            to test inference of a model deployed to the above inference server:\n"
    )
    print(
        "  curl -X POST http://localhost:8080/inference-server-example/<deployment-name>/infer \\"  # noqa: E501
    )
    print('  -H "Content-Type: application/json" \\')
    print("  -d '{")
    print('  "inputs": [')
    print("    {")
    print('      "name": "input_ids",')
    print('      "shape": [1, 10],')
    print('      "datatype": "INT64",')
    print('      "data": [101, 7592, 999, 102, 0, 0, 0, 0, 0, 0]')
    print("    },")
    print("    {")
    print('      "name": "attention_mask",')
    print('      "shape": [1, 10],')
    print('      "datatype": "INT64",')
    print('      "data": [1, 1, 1, 1, 0, 0, 0, 0, 0, 0]')
    print("    }")
    print("  ]")
    print("}'")


def _install_dynamo_discovery_rbac():
    """Install Dynamo discovery CRD and RBAC for Kubernetes-native service discovery.

    This enables Dynamo's Kubernetes discovery mode without requiring the full operator:
    - DynamoWorkerMetadata CRD: Schema for worker registration
    - ClusterRole: Permissions to manage worker metadata
    - ClusterRoleBinding: Grants permissions to pods in default namespace
    """
    print("📦 Installing Dynamo discovery CRD and RBAC...")

    context_args = _get_kubectl_context_args()
    rbac_path = _dir / "resources" / "dynamo-discovery-rbac.yaml"

    if not rbac_path.exists():
        print(f"  ⚠️  RBAC file not found at {rbac_path}, skipping...")
        return

    # Apply the CRD and RBAC (idempotent - will update if exists)
    _exec("kubectl", *context_args, "apply", "-f", str(rbac_path), raise_error=True)
    print("✅ Dynamo discovery CRD and RBAC installed successfully!")


def _create_inference_dynamo_demo_crs():
    """Create an NVIDIA Dynamo inference server for the sandbox cluster.

    This function uses self-provisioning (no Dynamo operator needed):
    1. Sets up Istio with Gateway API for routing
    2. Creates InferenceServer CR with BACKEND_TYPE_DYNAMO
    3. The Michelangelo controller directly provisions Frontend and Worker pods
    """
    print("🚀 Setting up Dynamo Inference Demo (self-provisioned)...")

    # Setup Istio with Gateway API for routing
    _setup_istio_with_gateway_api()

    # Install Dynamo discovery CRD and RBAC (no operator needed)
    # This allows workers to register themselves via K8s API for frontend discovery
    _install_dynamo_discovery_rbac()

    # Create the InferenceServer CR with Dynamo backend
    inference_demo_dir = _dir / "demo" / "inference"
    inference_server_path = inference_demo_dir / "inferenceserver_dynamo.yaml"
    if not inference_server_path.exists():
        _err_exit(
            f"❌ Dynamo inference server CR not found at {inference_server_path}, "
            "exiting..."
        )

    print("✅ Creating Dynamo Inference Server CR (self-provisioned)...")
    _kube_apply(inference_server_path)

    # Wait for inference server to reach SERVING state
    with open(inference_server_path) as f:
        inference_server_yaml = yaml.safe_load(f)
    inference_server_name = inference_server_yaml["metadata"]["name"]
    inference_server_namespace = inference_server_yaml["metadata"].get(
        "namespace", "default"
    )

    print(
        f"⏳ Waiting for Dynamo inference server "
        f"'{inference_server_name}' to be ready..."
    )
    print("   (This may take several minutes for first-time image pull)")

    context_args = _get_kubectl_context_args()
    try:
        _exec(
            "kubectl",
            *context_args,
            "wait",
            "--for=jsonpath=.status.state=INFERENCE_SERVER_STATE_SERVING",
            f"inferenceservers.michelangelo.api/{inference_server_name}",
            "-n",
            inference_server_namespace,
            "--timeout=900s",  # Longer timeout for model loading
            raise_error=True,
        )
        print("✅ Dynamo Inference server is ready!")

        # Start port-forward in background for easy access
        print("🔌 Starting port-forward to frontend (localhost:8000)...")
        try:
            # Start port-forward as a detached background process
            # start_new_session=True ensures it survives when parent exits
            # Self-provisioned naming: "dynamo-sp-{inference_server_name}-frontend"
            frontend_deployment = f"dynamo-sp-{inference_server_name}-frontend"
            port_forward_cmd = [
                "kubectl",
                *context_args,
                "port-forward",
                f"deployment/{frontend_deployment}",
                "8000:8000",
                "-n",
                inference_server_namespace,
            ]
            subprocess.Popen(
                port_forward_cmd,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.DEVNULL,
                stdin=subprocess.DEVNULL,
                start_new_session=True,
            )
            print("✅ Port-forward started! Access the API at http://localhost:8000")
        except Exception as e:
            print(f"⚠️  Could not start port-forward: {e}")

    except subprocess.CalledProcessError:
        # Check if it's still creating (not a failure)
        print(
            f"⚠️  Inference server '{inference_server_name}' "
            f"not ready after 15 minutes.\n"
            f"This may be normal if images are still being pulled.\n"
            f"Check status with:\n"
            f"  kubectl get inferenceservers.michelangelo.api "
            f"{inference_server_name} -n {inference_server_namespace} -o yaml\n"
            f"Check self-provisioned resources with:\n"
            f"  kubectl get deployments "
            f"-l michelangelo.ai/inference-server={inference_server_name}\n"
            f"  kubectl get pods -n {inference_server_namespace}"
        )

    print("🎉 Dynamo Inference demo setup completed (self-provisioned)!")
    print("📋 What was set up:")
    print("  • Gateway API with Istio integration")
    print("  • Michelangelo InferenceServer CR (with Dynamo backend)")
    print("  • Frontend and Worker pods (self-provisioned by controller)")
    print()
    print("🔍 To check self-provisioned resources:")
    print(
        f"  kubectl get deployments "
        f"-l michelangelo.ai/inference-server={inference_server_name}"
    )
    print(f"  kubectl get pods -n {inference_server_namespace}")
    print()
    print("🌐 Once ready, the Dynamo frontend will be available at:")
    print(
        f"  http://dynamo-sp-{inference_server_name}-frontend."
        f"{inference_server_namespace}.svc.cluster.local:8000"
    )
    print()
    print("📡 Test with OpenAI-compatible API:")
    print("  curl http://localhost:8000/v1/models")
    print(
        '  curl -X POST http://localhost:8000/v1/chat/completions '
        '-H "Content-Type: application/json" '
        '-d \'{"model": "Qwen/Qwen3-0.6B", "messages": [{"role": "user", '
        '"content": "Hello!"}], "max_tokens": 50}\''
    )

def _setup_istio_with_gateway_api():
    """Install Istio service mesh with Kubernetes Gateway API support.

    This function:
    1. Installs Istio base CRDs and cluster roles
    2. Installs Kubernetes Gateway API CRDs
    3. Installs Istio control plane (istiod)
    4. Creates the Gateway CR which triggers Istio to auto-provision the gateway
    """
    print("Setting up Istio service mesh with Gateway API...")

    # Fetch existing Helm repositories
    try:
        helm_existing_repos = subprocess.check_output(["helm", "repo", "list"]).decode()
    except subprocess.CalledProcessError:
        helm_existing_repos = ""

    # Add Istio Helm repository if not already present
    if "istio" not in helm_existing_repos:
        _exec(
            "helm",
            "repo",
            "add",
            "istio",
            "https://istio-release.storage.googleapis.com/charts",
        )
        _exec("helm", "repo", "update")

    # Install or upgrade Istio base (CRDs and cluster roles)
    print("Installing/upgrading Istio base...")
    _exec(
        "helm",
        "upgrade",
        "--install",
        "istio-base",
        "istio/base",
        "--namespace",
        "istio-system",
        "--create-namespace",
        "--wait",
    )

    # Install Gateway API CRDs (required for HTTPRoute support)
    # kubectl apply is idempotent by default
    context_args = _get_kubectl_context_args()
    print("Installing Gateway API CRDs...")
    _exec(
        "kubectl",
        *context_args,
        "apply",
        "-f",
        "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.1.0/standard-install.yaml",
    )
    _exec(
        "kubectl",
        *context_args,
        "wait",
        "--for=condition=Established",
        "crd/gateways.gateway.networking.k8s.io",
        "crd/httproutes.gateway.networking.k8s.io",
        "crd/gatewayclasses.gateway.networking.k8s.io",
        "--timeout=60s",
    )

    # Install or upgrade Istio control plane (istiod)
    print("Installing/upgrading Istio control plane...")
    _exec(
        "helm",
        "upgrade",
        "--install",
        "istiod",
        "istio/istiod",
        "--namespace",
        "istio-system",
        "--wait",
    )

    # Wait for Istio control plane to be ready
    _exec(
        "kubectl",
        *context_args,
        "wait",
        "--for=condition=available",
        "deployment",
        "--namespace=istio-system",
        "--all",
        "--timeout=600s",
    )

    print("✅ Istio control plane installed successfully")

    # Create Gateway CR (triggers Istio to auto-provision gateway deployment/service)
    gateway_setup_path = _dir / "resources" / "gateway-api-setup.yaml"
    if not gateway_setup_path.exists():
        _err_exit(f"❌ Gateway API setup not found at {gateway_setup_path}")

    print("Creating Gateway API Gateway CR...")
    _kube_apply(gateway_setup_path)

    # Wait for Gateway to be programmed (Istio provisions the gateway)
    _exec(
        "kubectl",
        *context_args,
        "wait",
        "--for=condition=Programmed",
        "gateway/ma-gateway",
        "-n",
        "default",
        "--timeout=300s",
    )

    # Print status for visibility
    _exec(
        "kubectl",
        *context_args,
        "get",
        "gateway",
        "ma-gateway",
        "-n",
        "default",
        "-o",
        "wide",
    )

    # automatically perform port-forwarding in the background
    subprocess.Popen(
        ["kubectl", *context_args, "-n", "default", "port-forward",
         "svc/ma-gateway-istio", "8080:80"],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    print("✅ Istio with Gateway API setup complete")


def _create_pipeline_demo_crs():
    """Create a pipeline demo for the sandbox cluster for demo purposes."""
    pipeline_demo_dir = _dir / "demo" / "pipeline"
    for yaml_file in pipeline_demo_dir.glob("*.yaml"):
        _kube_apply(yaml_file)

    print("✅ Pipeline demo resources created successfully")
    print("📋 What was set up:")
    print("  • Training pipelines")
    print("  • Pipeline triggers (cron and backfill)")
    print("  • Evaluation pipeline")
    print("  • Pipeline resources")
    print("  • Pipeline triggers")
    print("  • Pipeline evaluation")
    print(
        'The above pipelines can be verified in the Cadence Web UI at "http://localhost:8088/domains/default/workflows"'
    )


if __name__ == "__main__":
    sys.exit(main())
