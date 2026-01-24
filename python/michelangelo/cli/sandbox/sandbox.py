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


def init_arguments(p: argparse.ArgumentParser):
    """Initialize command-line arguments for the sandbox CLI."""
    sp = p.add_subparsers(dest="action", required=True)

    create_p = sp.add_parser("create", help="Create and start the cluster.")
    create_p.add_argument(
        "--exclude",
        help=(
            "Excludes specified services. "
            "Available options: apiserver, controllermgr, ui, worker"
        ),
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
    demo_sp = demo_p.add_subparsers(
        dest="demo_action", required=True, help="Demo type to create"
    )
    _ = demo_sp.add_parser("pipeline", help="Create pipeline demo resources")
    _ = demo_sp.add_parser("inference", help="Create inference server demo resources")

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

    # BLOCK END ----------------------------------------------------------------------

    _exec(*args)

    resources = [
        "boot.yaml",
        "mysql.yaml",
        "michelangelo-config.yaml",
        "aws-credentials.yaml",
    ]
    links = []

    # Cadence

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
    links.append(
        (
            "Prometheus",
            "http://localhost:9092",
            "",
        )
    )
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
        _create_cadence_domain(links)
        if "worker" not in ns.exclude:
            _kube_create(_dir / "resources/michelangelo-worker.yaml")
    else:
        raise ValueError(f"Unsupported workflow engine: {ns.workflow}")

    # Create separate compute cluster if requested
    if ns.create_compute_cluster:
        _create_compute_cluster(ns.compute_cluster_name)
        _create_compute_cluster_crd(ns.compute_cluster_name)
        _apply_compute_cluster_rbac(ns.compute_cluster_name)
        _create_compute_cluster_secrets(ns.compute_cluster_name)
    else:
        # Use the control plane cluster as the default compute cluster if a
        # dedicated compute cluster is not requested
        _create_compute_cluster_crd(_michelangelo_sandbox_kube_cluster_name)
        _apply_compute_cluster_rbac(_michelangelo_sandbox_kube_cluster_name)
        _create_compute_cluster_secrets(_michelangelo_sandbox_kube_cluster_name)

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

    # Register the default namespace in Temporal
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
    if ns.demo_action != "pipeline" and ns.demo_action != "inference":
        raise ValueError(f"Unsupported demo action: {ns.demo_action}")

    # Check if cluster exists
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
    else:
        raise ValueError(f"Unsupported demo action: {ns.demo_action}")


def _delete(ns: argparse.Namespace):
    assert ns
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


def _start(ns: argparse.Namespace):
    assert ns
    _exec("k3d", "cluster", "start", _michelangelo_sandbox_kube_cluster_name)


def _stop(ns: argparse.Namespace):
    assert ns
    _exec("k3d", "cluster", "stop", _michelangelo_sandbox_kube_cluster_name)


def _kube_create(path: Path):
    _exec("kubectl", "create", "-f", str(path))


def _kube_apply(path: Path):
    _exec("kubectl", "apply", "-f", str(path))


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


def _apply_compute_cluster_rbac(cluster_name: str):
    """Apply RBAC for Ray management in the compute cluster.

    This creates the ServiceAccount `ray-manager`, a namespaced Role with permissions on
    Ray resources, and a RoleBinding to bind them, in the `default` namespace of the
    jobs cluster.
    """
    rbac_path = _dir / "resources" / "rbac-ray.yaml"
    _exec(
        "kubectl",
        "--context",
        f"k3d-{cluster_name}",
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
    try:
        # Check if namespace already exists
        subprocess.check_output(
            [
                "kubectl",
                "--context",
                f"k3d-{_michelangelo_sandbox_kube_cluster_name}",
                "get",
                "namespace",
                namespace,
            ],
            stderr=subprocess.DEVNULL,
        )
        print(f"Namespace '{namespace}' already exists.")
    except subprocess.CalledProcessError:
        # Namespace doesn't exist, create it
        _exec(
            "kubectl",
            "--context",
            f"k3d-{_michelangelo_sandbox_kube_cluster_name}",
            "create",
            "namespace",
            namespace,
        )
        print(f"Created namespace '{namespace}' in the sandbox cluster.")


# Given a cluster name, create a Cluster CRD in the sandbox cluster
def _create_compute_cluster_crd(cluster_name: str):
    """Create a Cluster CRD for the Ray jobs cluster in the sandbox cluster."""
    # Ensure ma-system namespace exists
    _ensure_namespace_exists("ma-system")

    # Get kubeconfig for the Ray jobs cluster
    kubeconfig = subprocess.check_output(
        ["k3d", "kubeconfig", "get", cluster_name]
    ).decode()

    # Parse the kubeconfig YAML
    kubeconfig_data = yaml.safe_load(kubeconfig)

    # Extract server URL from clusters[0].cluster.server
    server_url = kubeconfig_data["clusters"][0]["cluster"]["server"]

    # Extract host and port from server URL
    # Example: "https://host.docker.internal:52910"
    import re

    match = re.search(r"(https://[^:]+):(\d+)", server_url)
    if not match:
        raise ValueError(
            f"Could not extract cluster host and port from server URL: {server_url}"
        )
    host, port = match.groups()

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

        # Apply the Cluster CRD to the sandbox cluster (explicitly specify context)
        _exec(
            "kubectl",
            "--context",
            f"k3d-{_michelangelo_sandbox_kube_cluster_name}",
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
    kubeconfig = subprocess.check_output(
        ["k3d", "kubeconfig", "get", cluster_name]
    ).decode()

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
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as ca_file:
        yaml.dump(ca_secret, ca_file)
        ca_file.flush()

        # Apply the CA secret to the sandbox cluster (explicit context)
        _exec(
            "kubectl",
            "--context",
            f"k3d-{_michelangelo_sandbox_kube_cluster_name}",
            "apply",
            "-f",
            ca_file.name,
        )

    # Create a new token for the ray-manager service account in the jobs cluster
    token_decoded = (
        subprocess.check_output(
            [
                "kubectl",
                "--context",
                f"k3d-{cluster_name}",
                "-n",
                "default",
                "create",
                "token",
                "ray-manager",
                # Required to override kubectl's 1h default token TTL;
                # set ~10y to prevent frequent sandbox expirations
                "--duration=87600h",
            ]
        )
        .decode()
        .strip()
    )

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

        # Apply the token secret to the sandbox cluster (explicit context)
        _exec(
            "kubectl",
            "--context",
            f"k3d-{_michelangelo_sandbox_kube_cluster_name}",
            "apply",
            "-f",
            token_file.name,
        )

    print(f"\nCreated secrets for cluster '{cluster_name}' in the sandbox cluster")


def _create_inference_cluster_secrets(cluster_name: str, secret_prefix: str):
    """Create Kubernetes secrets for inference server to access a target cluster.

    This creates:
    1. A service account with cluster-admin permissions in the target cluster
    2. Secrets in the control plane cluster with the CA data and token

    Args:
        cluster_name: The k3d cluster name (e.g., "cluster-1")
        secret_prefix: Prefix for secret names (e.g., "k3d-cluster-1")
    """
    print(
        f"🔐 Creating secrets for inference server access to cluster '{cluster_name}'"
    )

    # Get kubeconfig for the target cluster
    kubeconfig = subprocess.check_output(
        ["k3d", "kubeconfig", "get", cluster_name]
    ).decode()

    # Parse the kubeconfig YAML
    kubeconfig_data = yaml.safe_load(kubeconfig)

    # Extract certificate-authority-data from clusters[0].cluster
    ca_data = kubeconfig_data["clusters"][0]["cluster"].get(
        "certificate-authority-data"
    )
    if not ca_data:
        raise ValueError("certificate-authority-data not found in kubeconfig")
    ca_data_decoded = base64.b64decode(ca_data).decode()

    # Create service account with cluster-admin permissions in the target cluster
    sa_manifest = {
        "apiVersion": "v1",
        "kind": "ServiceAccount",
        "metadata": {
            "name": "michelangelo-inference-access",
            "namespace": "default",
        },
    }

    crb_manifest = {
        "apiVersion": "rbac.authorization.k8s.io/v1",
        "kind": "ClusterRoleBinding",
        "metadata": {"name": "michelangelo-inference-access-admin"},
        "roleRef": {
            "apiGroup": "rbac.authorization.k8s.io",
            "kind": "ClusterRole",
            "name": "cluster-admin",
        },
        "subjects": [
            {
                "kind": "ServiceAccount",
                "name": "michelangelo-inference-access",
                "namespace": "default",
            }
        ],
    }

    # Apply SA and CRB to target cluster
    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as f:
        yaml.dump_all([sa_manifest, crb_manifest], f)
        f.flush()
        _exec(
            "kubectl",
            "--context",
            f"k3d-{cluster_name}",
            "apply",
            "-f",
            f.name,
        )

    # Create a token for the service account in the target cluster
    token_decoded = (
        subprocess.check_output(
            [
                "kubectl",
                "--context",
                f"k3d-{cluster_name}",
                "-n",
                "default",
                "create",
                "token",
                "michelangelo-inference-access",
                "--duration=87600h",  # ~10 years
            ]
        )
        .decode()
        .strip()
    )

    # Create CA secret in the control plane cluster
    ca_secret = {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {"name": f"{secret_prefix}-ca", "namespace": "default"},
        "stringData": {"cadata": ca_data_decoded},
    }

    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as f:
        yaml.dump(ca_secret, f)
        f.flush()
        _exec(
            "kubectl",
            "--context",
            f"k3d-{_michelangelo_sandbox_kube_cluster_name}",
            "apply",
            "-f",
            f.name,
        )

    # Create token secret in the control plane cluster
    token_secret = {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {"name": f"{secret_prefix}-token", "namespace": "default"},
        "stringData": {"token": token_decoded},
    }

    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as f:
        yaml.dump(token_secret, f)
        f.flush()
        _exec(
            "kubectl",
            "--context",
            f"k3d-{_michelangelo_sandbox_kube_cluster_name}",
            "apply",
            "-f",
            f.name,
        )

    # Extract and log the server URL for the user to update their CR if needed
    server_url = kubeconfig_data["clusters"][0]["cluster"].get("server", "")
    print(f"✅ Created secrets '{secret_prefix}-ca' and '{secret_prefix}-token'")
    print(f"   Target cluster API server: {server_url}")


def _create_inference_demo_crs():
    """Create an inference server for the sandbox cluster for demo purposes."""
    print("🚀 Setting up Michelangelo AI Inference Demo...")

    inference_demo_dir = _dir / "demo" / "inference"
    # Create inference server CR
    inference_server_path = inference_demo_dir / "inferenceserver.yaml"
    if not inference_server_path.exists():
        _err_exit(
            f"❌ Inference server CR not found at {inference_server_path}, exiting..."
        )

    # Parse the inference server YAML to extract target clusters
    with open(inference_server_path) as f:
        inference_server_yaml = yaml.safe_load(f)

    # Extract target cluster info from clusterTargets
    cluster_targets = inference_server_yaml.get("spec", {}).get("clusterTargets", [])
    target_cluster_names = []

    for target in cluster_targets:
        cluster_id = target.get("clusterId", "")
        k8s_config = target.get("kubernetes", {})
        token_tag = k8s_config.get("tokenTag", "")
        ca_tag = k8s_config.get("caDataTag", "")

        if cluster_id and token_tag and ca_tag:
            # Derive k3d cluster name from clusterId
            # e.g., "k3d-cluster-1" -> "cluster-1"
            if cluster_id.startswith("k3d-"):
                k3d_cluster_name = cluster_id.replace("k3d-", "")
            else:
                k3d_cluster_name = cluster_id
            # Derive secret prefix from tokenTag
            # e.g., "k3d-cluster-1-token" -> "k3d-cluster-1"
            secret_prefix = token_tag.replace("-token", "")

            # Create secrets for this target cluster
            _create_inference_cluster_secrets(k3d_cluster_name, secret_prefix)
            # Use the k3d cluster name (without k3d- prefix) for Istio setup
            target_cluster_names.append(k3d_cluster_name)

    # Setup Istio with Gateway API on target clusters
    # This allows usage of HTTPRoutes to route traffic to the inference server.
    if not target_cluster_names:
        _err_exit(
            "❌ No valid clusterTargets found in inferenceserver.yaml.\n"
            "Please specify at least one clusterTarget with clusterId, "
            "tokenTag, and caDataTag."
        )

    print(
        f"📋 Found {len(target_cluster_names)} target cluster(s): "
        f"{target_cluster_names}"
    )

    # Setup Istio on control plane cluster and target clusters (for ServiceEntry)
    all_clusters = [_michelangelo_sandbox_kube_cluster_name]
    all_clusters.extend(target_cluster_names)
    _setup_istio_on_clusters(all_clusters)

    print("✅ Creating Triton Inference Server...")
    _kube_apply(inference_server_path)

    # Wait for inference server to reach SERVING state (image pull may take time)
    inference_server_name = inference_server_yaml["metadata"]["name"]
    inference_server_namespace = inference_server_yaml["metadata"].get(
        "namespace", "default"
    )

    print(f"⏳ Waiting for inference server '{inference_server_name}' to be ready...")
    print("   (This may take 5-10 minutes for first-time Triton image pull)")

    try:
        _exec(
            "kubectl",
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

    # Deploy model-sync Deployment to each target cluster
    model_sync_deployment_path = _dir / "resources" / "model-sync.yaml"
    if not model_sync_deployment_path.exists():
        _err_exit(
            f"❌ Model-sync Deployment not found at {model_sync_deployment_path},\
                exiting..."
        )

    # Deploy to each target cluster
    for cluster_name in target_cluster_names:
        ctx = f"k3d-{cluster_name}"

        # Create michelangelo-config ConfigMap (required for AWS/S3 access)
        print(f"📦 Creating michelangelo-config ConfigMap in cluster '{ctx}'...")
        _create_config_in_compute_cluster(cluster_name)

        print(f"✅ Deploying model-sync Deployment to cluster '{ctx}'...")

        kubectl_args = ["kubectl", "--context", ctx]
        _exec(*kubectl_args, "apply", "-f", str(model_sync_deployment_path))

        # Wait for Deployment to be ready
        print(f"⏳ Waiting for model-sync Deployment to be ready on '{ctx}'...")
        try:
            _exec(
                *kubectl_args,
                "rollout",
                "status",
                "deployment/model-sync",
                "-n",
                "default",
                "--timeout=60s",
                raise_error=True,
            )
            print(f"✅ Model-sync Deployment is ready on '{ctx}'!")
        except subprocess.CalledProcessError:
            _err_exit(
                f"Model-sync Deployment failed to become ready after 60s on '{ctx}'.\n"
                f"Check status with:\n"
                f"  kubectl --context {ctx} "
                "get deployments model-sync -n default -o yaml\n"
                f"Check logs with:\n"
                f"  kubectl --context {ctx} logs deployment/model-sync -n default"
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


def _generate_shared_ca_certs(cluster_names: list[str]) -> Path:
    """Generate a shared root CA and per-cluster intermediate CAs.

    For multi-cluster Istio mTLS to work, all clusters must trust each other's
    certificates. This is achieved by having a shared root CA that signs
    intermediate CAs for each cluster.

    Directory structure:
        certs/
        ├── root-cert.pem       # Root CA certificate (shared)
        ├── root-key.pem        # Root CA private key (keep secure)
        ├── <cluster>/
        │   ├── ca-cert.pem     # Intermediate CA certificate
        │   ├── ca-key.pem      # Intermediate CA private key
        │   └── cert-chain.pem  # Certificate chain (intermediate + root)

    Args:
        cluster_names: List of cluster names to generate intermediate CAs for

    Returns:
        Path to the certs directory
    """
    # Create a persistent certs directory in the sandbox resources
    ca_dir = _dir / "certs"
    ca_dir.mkdir(exist_ok=True)

    root_cert = ca_dir / "root-cert.pem"
    root_key = ca_dir / "root-key.pem"

    # Generate root CA if it doesn't exist
    if not root_cert.exists() or not root_key.exists():
        print("   Generating root CA...")
        subprocess.check_call(
            [
                "openssl",
                "req",
                "-x509",
                "-sha256",
                "-nodes",
                "-days",
                "3650",  # 10 years
                "-newkey",
                "rsa:4096",
                "-subj",
                "/O=Michelangelo/CN=Root CA",
                "-keyout",
                str(root_key),
                "-out",
                str(root_cert),
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

    # Generate intermediate CA for each cluster
    for cluster_name in cluster_names:
        cluster_dir = ca_dir / cluster_name
        cluster_dir.mkdir(exist_ok=True)

        ca_cert = cluster_dir / "ca-cert.pem"
        ca_key = cluster_dir / "ca-key.pem"
        ca_csr = cluster_dir / "ca-csr.pem"
        cert_chain = cluster_dir / "cert-chain.pem"

        if ca_cert.exists() and ca_key.exists():
            print(f"   Intermediate CA for {cluster_name} already exists, skipping...")
            continue

        print(f"   Generating intermediate CA for {cluster_name}...")

        # Generate intermediate CA private key and CSR
        subprocess.check_call(
            [
                "openssl",
                "req",
                "-newkey",
                "rsa:4096",
                "-nodes",
                "-subj",
                f"/O=Michelangelo/CN={cluster_name} Intermediate CA",
                "-keyout",
                str(ca_key),
                "-out",
                str(ca_csr),
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

        # Create extensions config file for CA signing
        ext_file = cluster_dir / "ca-ext.cnf"
        ext_file.write_text(
            "[v3_ca]\n"
            "basicConstraints=critical,CA:TRUE\n"
            "keyUsage=critical,keyCertSign,cRLSign\n"
        )

        # Sign the intermediate CA with the root CA
        subprocess.check_call(
            [
                "openssl",
                "x509",
                "-req",
                "-sha256",
                "-days",
                "3650",
                "-CA",
                str(root_cert),
                "-CAkey",
                str(root_key),
                "-CAcreateserial",
                "-in",
                str(ca_csr),
                "-out",
                str(ca_cert),
                "-extfile",
                str(ext_file),
                "-extensions",
                "v3_ca",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

        # Clean up extension file
        ext_file.unlink(missing_ok=True)

        # Create certificate chain (intermediate + root)
        with open(cert_chain, "w") as f:
            f.write(ca_cert.read_text())
            f.write(root_cert.read_text())

        # Clean up CSR
        ca_csr.unlink(missing_ok=True)

    return ca_dir


def _create_istio_ca_secret(kube_context: str, ca_dir: Path, cluster_name: str):
    """Create the 'cacerts' secret in istio-system namespace.

    Istio uses this secret to issue workload certificates. By providing
    a shared root CA hierarchy, all clusters will trust each other's
    workload certificates.

    Args:
        kube_context: kubectl context (e.g., "k3d-cluster-1")
        ca_dir: Path to the CA certificates directory
        cluster_name: Name of the cluster (used to find intermediate CA)
    """
    kubectl_context_args = ["--context", kube_context]

    root_cert = ca_dir / "root-cert.pem"
    cluster_ca_dir = ca_dir / cluster_name
    ca_cert = cluster_ca_dir / "ca-cert.pem"
    ca_key = cluster_ca_dir / "ca-key.pem"
    cert_chain = cluster_ca_dir / "cert-chain.pem"

    # Verify all required files exist
    for f in [root_cert, ca_cert, ca_key, cert_chain]:
        if not f.exists():
            raise FileNotFoundError(f"Required CA file not found: {f}")

    # Ensure istio-system namespace exists
    with contextlib.suppress(subprocess.CalledProcessError):
        subprocess.check_call(
            ["kubectl", *kubectl_context_args, "create", "namespace", "istio-system"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

    # Delete existing secret if present (to update certs)
    with contextlib.suppress(subprocess.CalledProcessError):
        subprocess.check_call(
            [
                "kubectl",
                *kubectl_context_args,
                "delete",
                "secret",
                "cacerts",
                "-n",
                "istio-system",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )

    # Create the cacerts secret
    print(f"   Creating cacerts secret in {kube_context}...")
    subprocess.check_call(
        [
            "kubectl",
            *kubectl_context_args,
            "create",
            "secret",
            "generic",
            "cacerts",
            "-n",
            "istio-system",
            f"--from-file=root-cert.pem={root_cert}",
            f"--from-file=ca-cert.pem={ca_cert}",
            f"--from-file=ca-key.pem={ca_key}",
            f"--from-file=cert-chain.pem={cert_chain}",
        ]
    )


def _create_inference_mtls_destination_rule():
    """Create a wildcard DestinationRule for multi-cluster mTLS on the control plane.

    This DestinationRule tells the control plane's Istio sidecars to use mTLS
    when connecting to ServiceEntry endpoints (east-west gateways in target clusters).

    The rule uses a wildcard host pattern to match all inference service hostnames
    created by the EndpointRegistry (e.g., *-inference-service.*.svc.cluster.local).
    """
    kube_context = f"k3d-{_michelangelo_sandbox_kube_cluster_name}"
    kubectl_context_args = ["--context", kube_context]

    print("🔒 Creating mTLS DestinationRule for multi-cluster inference routing...")

    destination_rule_path = _dir / "demo" / "inference" / "destination-rule-mtls.yaml"
    if not destination_rule_path.exists():
        _err_exit(f"❌ DestinationRule not found at {destination_rule_path}")

    _exec(
        "kubectl",
        *kubectl_context_args,
        "apply",
        "-f",
        str(destination_rule_path),
    )

    print("✅ mTLS DestinationRule created for inference endpoints")


def _setup_istio_on_clusters(target_clusters: list[str]):
    """Install Istio and Gateway API on multiple target clusters.

    This function sets up Istio service mesh with Kubernetes Gateway API support
    on each of the specified target clusters. Use this for multi-cluster
    deployments where inference workloads run on separate clusters.

    For multi-cluster mTLS to work (AUTO_PASSTHROUGH on east-west gateways),
    all clusters must share the same root CA. This function:
    1. Generates a shared root CA
    2. Creates per-cluster intermediate CAs signed by the root
    3. Installs Istio with the shared CA hierarchy

    For target clusters (non-control-plane), an east-west gateway is also
    installed to enable cross-cluster routing via AUTO_PASSTHROUGH.

    Args:
        target_clusters: List of k3d cluster names where Istio should be
            installed. Each name is the k3d cluster name (without k3d- prefix).
    """
    if not target_clusters:
        print("⚠️ No target clusters specified for Istio setup")
        return

    print(f"🚀 Setting up Istio on {len(target_clusters)} target cluster(s)...")

    # Generate shared root CA for multi-cluster mTLS trust
    print("🔐 Generating shared root CA for multi-cluster mTLS...")
    ca_dir = _generate_shared_ca_certs(target_clusters)
    print(f"   CA certificates stored in: {ca_dir}")

    for idx, cluster_name in enumerate(target_clusters):
        print(f"\n📦 Setting up Istio on cluster: {cluster_name}")
        kube_context = f"k3d-{cluster_name}"
        # Network names:
        # network0 for control plane,
        # network1, network2, etc. for target clusters
        network_name = f"network{idx}"

        # Install shared CA secret before Istio installation
        _create_istio_ca_secret(kube_context, ca_dir, cluster_name)

        # Label istio-system namespace with network topology
        # (required for multi-cluster)
        # See: https://istio.io/latest/docs/setup/install/multicluster/multi-primary_multi-network/
        _exec(
            "kubectl",
            "--context",
            kube_context,
            "label",
            "namespace",
            "istio-system",
            f"topology.istio.io/network={network_name}",
            "--overwrite",
        )

        # Label default namespace for sidecar injection and network topology
        # Sidecar injection is required for pods
        # to receive mTLS traffic from east-west gateway
        _exec(
            "kubectl",
            "--context",
            kube_context,
            "label",
            "namespace",
            "default",
            "istio-injection=enabled",
            f"topology.istio.io/network={network_name}",
            "--overwrite",
        )

        _setup_istio_with_gateway_api(
            kube_context=kube_context,
            cluster_name=cluster_name,
            network_name=network_name,
        )

        # Install east-west gateway on target clusters (not control plane)
        is_control_plane = cluster_name == _michelangelo_sandbox_kube_cluster_name
        if not is_control_plane:
            cluster_id = f"k3d-{cluster_name}"  # e.g., "k3d-cluster-1"
            _install_east_west_gateway(
                kube_context=kube_context,
                cluster_id=cluster_id,
                network_name=network_name,
            )

        print(f"✅ Istio setup complete on cluster: {cluster_name}")

    # Create a wildcard DestinationRule on the control plane to enable mTLS
    # for all traffic to ServiceEntry endpoints (east-west gateways)
    _create_inference_mtls_destination_rule()

    # setup port-forwarding for the control plane
    # kubectl --context k3d-michelangelo-sandbox port-forward svc/ma-gateway-istio 8080:80 -n default
    subprocess.Popen(
        [
            "kubectl",
            "--context",
            f"k3d-{_michelangelo_sandbox_kube_cluster_name}",
            "port-forward",
            "svc/ma-gateway-istio",
            "8080:80",
            "-n",
            "default",
        ],
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )

    print(f"\n✅ Istio setup complete on all {len(target_clusters)} cluster(s)")


def _setup_istio_with_gateway_api(
    kube_context: str | None = None,
    cluster_name: str | None = None,
    network_name: str | None = None,
):
    """Install Istio service mesh with Kubernetes Gateway API support.

    This function:
    1. Installs Istio base CRDs and cluster roles
    2. Installs Kubernetes Gateway API CRDs
    3. Installs Istio control plane (istiod) with multi-cluster settings
    4. Creates the Gateway CR which triggers Istio to auto-provision the gateway

    Args:
        kube_context: Optional kubectl context to use. If None, uses current context.
        cluster_name: Optional cluster name for multi-cluster mesh.
            Used for istiod multiCluster.clusterName.
        network_name: Optional network name for multi-cluster mesh.
            Used for istiod global.network.
    """
    # helm uses --kube-context, kubectl uses --context
    helm_context_args = ["--kube-context", kube_context] if kube_context else []
    kubectl_context_args = ["--context", kube_context] if kube_context else []
    context_msg = f" (context: {kube_context})" if kube_context else ""

    print(f"Setting up Istio service mesh with Gateway API{context_msg}...")

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
        *helm_context_args,
        "--namespace",
        "istio-system",
        "--create-namespace",
        "--wait",
    )

    # Install Gateway API CRDs (required for HTTPRoute support)
    # kubectl apply is idempotent by default
    print("Installing Gateway API CRDs...")
    _exec(
        "kubectl",
        *kubectl_context_args,
        "apply",
        "-f",
        "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.1.0/standard-install.yaml",
    )
    _exec(
        "kubectl",
        *kubectl_context_args,
        "wait",
        "--for=condition=Established",
        "crd/gateways.gateway.networking.k8s.io",
        "crd/httproutes.gateway.networking.k8s.io",
        "crd/gatewayclasses.gateway.networking.k8s.io",
        "--timeout=60s",
    )

    # Install or upgrade Istio control plane (istiod)
    print("Installing/upgrading Istio control plane...")
    istiod_args = [
        "helm",
        "upgrade",
        "--install",
        "istiod",
        "istio/istiod",
        *helm_context_args,
        "--namespace",
        "istio-system",
        "--wait",
    ]
    # Add multi-cluster settings if cluster/network names are provided
    # See: https://istio.io/latest/docs/setup/install/multicluster/multi-primary_multi-network/
    if cluster_name or network_name:
        istiod_args.extend(["--set", "global.meshID=mesh1"])
    if cluster_name:
        istiod_args.extend(["--set", f"global.multiCluster.clusterName={cluster_name}"])
    if network_name:
        istiod_args.extend(["--set", f"global.network={network_name}"])
    _exec(*istiod_args)

    # Wait for Istio control plane to be ready
    _exec(
        "kubectl",
        *kubectl_context_args,
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
    _exec(
        "kubectl",
        *kubectl_context_args,
        "apply",
        "-f",
        str(gateway_setup_path),
    )

    # Wait for Gateway to be programmed (Istio provisions the gateway)
    _exec(
        "kubectl",
        *kubectl_context_args,
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
        *kubectl_context_args,
        "get",
        "gateway",
        "ma-gateway",
        "-n",
        "default",
        "-o",
        "wide",
    )

    print("✅ Istio with Gateway API setup complete")


def _install_east_west_gateway(kube_context: str, cluster_id: str, network_name: str):
    """Install an Istio east-west gateway for cross-cluster routing.

    This function:
    1. Installs the istio/gateway Helm chart as an east-west gateway
    2. Labels the gateway Service with discovery labels for EndpointRegistry
    3. Creates a Gateway CR with AUTO_PASSTHROUGH for SNI-based routing

    The EndpointRegistry in the control plane discovers this gateway by looking
    for Services with labels:
      - michelangelo.ai/east-west-gateway: "true"
      - michelangelo.ai/cluster-id: <cluster_id>

    Args:
        kube_context: kubectl context (e.g., "k3d-cluster-1")
        cluster_id: The cluster ID used in InferenceServer.spec.clusterTargets
                    (e.g., "k3d-cluster-1")
        network_name: Network name for multi-cluster topology (e.g., "network1")
                      See: https://istio.io/latest/docs/setup/install/multicluster/multi-primary_multi-network/
    """
    helm_context_args = ["--kube-context", kube_context]
    kubectl_context_args = ["--context", kube_context]

    print(
        f"📡 Installing east-west gateway on {kube_context} (cluster_id={cluster_id})"
    )

    # Install the east-west gateway using istio/gateway Helm chart
    # We use a separate release name to avoid conflict with the ingress gateway
    # The networkGateway setting adds the
    # topology.istio.io/network label for multi-cluster
    _exec(
        "helm",
        "upgrade",
        "--install",
        "istio-eastwestgateway",
        "istio/gateway",
        *helm_context_args,
        "--namespace",
        "istio-system",
        "--set",
        "name=istio-eastwestgateway",
        # networkGateway adds topology.istio.io/network label
        # (required for multi-cluster)
        # See: https://istio.io/latest/docs/setup/install/multicluster/multi-primary_multi-network/
        "--set",
        f"networkGateway={network_name}",
        # Use NodePort for k3d compatibility
        "--set",
        "service.type=NodePort",
        # Expose port 15443 for mTLS passthrough
        "--set",
        "service.ports[0].name=tls",
        "--set",
        "service.ports[0].port=15443",
        "--set",
        "service.ports[0].targetPort=15443",
        "--set",
        "service.ports[0].nodePort=31443",
        "--wait",
    )

    # Label the gateway Service with discovery labels
    # These labels allow EndpointRegistry to discover this gateway
    _exec(
        "kubectl",
        *kubectl_context_args,
        "label",
        "service",
        "istio-eastwestgateway",
        "-n",
        "istio-system",
        "michelangelo.ai/east-west-gateway=true",
        f"michelangelo.ai/cluster-id={cluster_id}",
        "--overwrite",
    )

    # todo: ghosharitra: make this into a yaml
    # Create a Gateway CR with AUTO_PASSTHROUGH mode
    # This allows the gateway to route based on SNI without explicit VirtualServices
    gateway_manifest = {
        "apiVersion": "networking.istio.io/v1",
        "kind": "Gateway",
        "metadata": {
            "name": "cross-cluster-gateway",
            "namespace": "istio-system",
        },
        "spec": {
            "selector": {"istio": "eastwestgateway"},
            "servers": [
                {
                    "port": {"number": 15443, "name": "tls", "protocol": "TLS"},
                    "tls": {"mode": "AUTO_PASSTHROUGH"},
                    "hosts": ["*.svc.cluster.local"],
                }
            ],
        },
    }

    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as f:
        yaml.dump(gateway_manifest, f)
        f.flush()
        _exec(
            "kubectl",
            *kubectl_context_args,
            "apply",
            "-f",
            f.name,
        )

    print("✅ East-west gateway installed with labels:")
    print("   michelangelo.ai/east-west-gateway=true")
    print(f"   michelangelo.ai/cluster-id={cluster_id}")


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
