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

short_description = "Manage the sandbox cluster."

description = """
Michelangelo Sandbox is a lightweight version of the Michelangelo platform, tailored for local development and testing.
This tool helps you create and manage a sandbox cluster directly on your machine.
"""

_dir = Path(__file__).parent

_michelangelo_sandbox_kube_cluster_name = "michelangelo-sandbox"
_kube_ports = [
    "3306:30001",  # MySQL
    "9091:30007",  # MinIO
    "9090:30008",  # MinIO
    "14566:30009",  # Michelangelo API Server
    "8081:30010",  # Envoy gRPC --> gRPC-web proxy
    "8090:30011",  # Michelangelo UI
    "3000:30012",  # Grafana
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
_default_job_hosting_kube_cluster_name = "michelangelo-jobs-0"


def init_arguments(p: argparse.ArgumentParser):
    sp = p.add_subparsers(dest="action", required=True)

    create_p = sp.add_parser("create", help="Create and start the cluster.")
    create_p.add_argument(
        "--exclude",
        help="Excludes specified services. Available options: apiserver, controllermgr, ui, worker",
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
        "--create-jobs-cluster",
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
        "--jobs-cluster-name",
        default=_default_job_hosting_kube_cluster_name,
        help="Name of the jobs cluster to create when --create-jobs-cluster is used (default: %s)."
        % _default_job_hosting_kube_cluster_name,
    )

    _ = sp.add_parser(
        "demo", help="Create demo project and pipelines in the sandbox cluster."
    )
    delete_p = sp.add_parser("delete", help="Delete the cluster.")
    delete_p.add_argument(
        "--jobs-cluster-name",
        default=_default_job_hosting_kube_cluster_name,
        help="Name of the jobs cluster to delete when --create-jobs-cluster is used (default: %s)."
        % _default_job_hosting_kube_cluster_name,
    )
    _ = sp.add_parser("start", help="Start the cluster.")
    _ = sp.add_parser("stop", help="Stop the cluster.")


def main(args=None):
    p = argparse.ArgumentParser(description=description)
    init_arguments(p)
    ns = p.parse_args(args=args)
    return run(ns)


def run(ns: argparse.Namespace):
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
CR_PAT environment variable is not set. To pull Michelangelo's containers from the GitHub Container Registry, please create a GitHub personal access token (classic) with the "read:packages" scope. Then, save this token to the CR_PAT environment variable, e.g.: `export CR_PAT=ghp_...`.

For a detailed guide, check https://docs.github.com/en/packages/working-with-a-github-packages-registry/working-with-the-container-registry#authenticating-with-a-personal-access-token-classic.

Be aware that CR_PAT environment variable is required while Michelangelo is NOT publicly accessible. Once we become public, the token will no longer be necessary, and this assertion will be removed.
"""
        )

    # Create a temporary registry file with the GitHub Container Registry authentication.
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

    # BLOCK END ----------------------------------------------------------------------

    _exec(*args)

    resources = [
        "boot.yaml",
        "mysql.yaml",
        "michelangelo-config.yaml",
        "aws-credentials.yaml",
        "yscope-log-viewer-deployment.yaml",
        "logs-bucket-creation.yaml",
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

    # Grafana

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

    for r in resources:
        _kube_create(_dir / "resources" / r)

    _assert_command(
        "helm", "Helm not found, please install it: https://helm.sh/docs/intro/install/"
    )

    # Handle the case when helm repo list returns non-zero exit status (no repositories)
    try:
        helm_existing_repos = subprocess.check_output(["helm", "repo", "list"]).decode()
    except subprocess.CalledProcessError:
        # helm repo list returns non-zero exit status when no repositories are configured
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

    # Create the jobs cluster if requested
    if ns.create_jobs_cluster:
        _create_jobs_cluster(ns.jobs_cluster_name)
        _create_cluster_crd(ns.jobs_cluster_name)
        _create_cluster_secrets(ns.jobs_cluster_name)

    _kube_wait()

    print(
        "\n🚀 Sandbox created successfully. To access the services, please use the following links:\n"
    )
    for title, url, comment in links:
        print(f"  - {title}: {url} {comment}")

    print()


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
    )


def _create_kuberay_operator(helm_existing_repos):
    """
    Create the KubeRay operator using Helm.
    Reference: https://docs.ray.io/en/releases-2.49.1/cluster/kubernetes/getting-started/kuberay-operator-installation.html#method-1-helm-recommended
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


def _create_demo_crs(_: argparse.Namespace):
    """Create demo Custom Resources (CRs) for the sandbox environment."""
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
            f"Cluster {_michelangelo_sandbox_kube_cluster_name} not found. Please run 'ma sandbox create' first."
        )

    # Check if cluster is running
    try:
        _exec("kubectl", "cluster-info", raise_error=True)
    except subprocess.CalledProcessError:
        _err_exit(
            f"Cluster {_michelangelo_sandbox_kube_cluster_name} is not running. Please run 'ma sandbox start' first."
        )

    demo_dir = _dir / "demo"
    project_yaml_path = demo_dir / "project.yaml"

    # Extract namespace from project.yaml
    with open(project_yaml_path) as f:
        project_yaml = yaml.safe_load(f)
    namespace = project_yaml.get("metadata", {}).get("namespace", "default")

    _exec("kubectl", "create", "namespace", namespace)

    # Create project first. Project CRD is essentially the "parent" of other CRDs. Under
    # normal circumstances, users must create a project before creating other CRDs.
    _kube_create(project_yaml_path)

    # Create all other YAML files in the demo directory
    for yaml_file in demo_dir.glob("*.yaml"):
        if yaml_file.name != "project.yaml":
            _kube_create(yaml_file)

    print(f"\nDemo CRs created in namespace {namespace}.")


def _delete(ns: argparse.Namespace):
    assert ns
    if ns.jobs_cluster_name:
        _exec("k3d", "cluster", "delete", ns.jobs_cluster_name)
    else:
        _exec("k3d", "cluster", "delete", _default_job_hosting_kube_cluster_name)
    _exec("k3d", "cluster", "delete", _michelangelo_sandbox_kube_cluster_name)


def _start(ns: argparse.Namespace):
    assert ns
    _exec("k3d", "cluster", "start", _michelangelo_sandbox_kube_cluster_name)


def _stop(ns: argparse.Namespace):
    assert ns
    _exec("k3d", "cluster", "stop", _michelangelo_sandbox_kube_cluster_name)


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


def _apply_jobs_rbac(cluster_name: str):
    """Apply RBAC for Ray management in the jobs cluster.

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
    env: dict[str, str] = None,
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
        "--stdin",  # Keep stdin open on the container in the pod, allowing the command to block until completion.
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
    """
    Execute a shell command with optional retries. If the command exits with a non-zero code, it will be retried up to
    retry_attempts times, waiting retry_delay_seconds between attempts.

    Parameters:
        *args: Variable-length argument list representing the command to run and its arguments.
        retry_attempts: Number of times to retry the command on failure. Defaults to 0 (no retry).
        retry_delay_seconds: Number of seconds to wait between retries. Defaults to 5.
        raise_error: Determines how to handle errors after the final retry. If True, the function will raise a
            subprocess.CalledProcessError. If False, the function will terminate the program with the exit code of the
            failed command. Defaults to False.

    Returns:
        None.

    Raises:
        subprocess.CalledProcessError: If the command fails after all retries and raise_error is True.

    Examples:
        - Basic usage with a single command: _exec("ls", "-l", "~/bin")
        - Run a script with retries: _exec("bash", "my_script.sh", retry_attempts=3, retry_delay_seconds=2)

    Side Effects:
        - Prints the command being executed and retry messages if any.
        - Terminates the program if raise_error is False and retries are exhausted.
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


def _create_jobs_cluster(cluster_name: str):
    """Create a dedicated cluster for jobs."""

    args = [
        "k3d",
        "cluster",
        "create",
        cluster_name,
        "--servers",
        "1",
        "--agents",
        "2",  # More worker nodes for Ray
        "--kubeconfig-switch-context=false",  # Don't switch kubectl context to this cluster
    ]

    # Add port mappings for Ray
    for p in _ray_ports:
        args += ["-p", f"{p}@agent:0"]

    _exec(*args)

    # Add kuberay operator to the jobs cluster
    _exec(
        "kubectl",
        "--context",
        f"k3d-{cluster_name}",
        "create",
        "-k",
        "github.com/ray-project/kuberay/ray-operator/config/default?ref=v1.0.0",
    )

    # Apply RBAC for ray-manager in the jobs cluster
    _apply_jobs_rbac(cluster_name)

    print(f"\nJobs cluster '{cluster_name}' created successfully.")


# Given a cluster name, create a Cluster CRD in the sandbox cluster
def _create_cluster_crd(cluster_name: str):
    """Create a Cluster CRD for the Ray jobs cluster in the sandbox cluster."""
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


def _create_cluster_secrets(cluster_name: str):
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


if __name__ == "__main__":
    sys.exit(main())
