"""Sandbox CLI for Michelangelo."""

import argparse
import base64
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

_cadence_domain = "default"
_default_compute_kube_cluster_name = "michelangelo-compute-0"

# Path to the Michelangelo Helm chart (relative to this file)
_chart_dir = Path(__file__).parent.parent.parent.parent.parent / "helm" / "michelangelo"

# Path to values-k3d.yaml — used to read Helm-managed NodePorts dynamically
_values_k3d_path = _chart_dir / "values-k3d.yaml"

# Hardcoded infra ports — services NOT installed by the michelangelo Helm chart.
# These are raw YAML resources deployed by _deploy_services() directly.
_infra_ports = [
    "3306:30001",  # MySQL
    "9091:30007",  # MinIO
    "9090:30008",  # MinIO Console
    "3000:30012",  # Grafana
    "9092:30015",  # Prometheus
    "5001:30013",  # MLflow Tracking Server
]

# Infra ports owned by optionally-excluded services. When the user passes
# --exclude {svc}, the corresponding host port is dropped from k3d's port
# forwards so it doesn't conflict with other processes on the host.
_infra_port_owner = {
    "3000:30012": "grafana",
    "9092:30015": "prometheus",
}

# Ray framework ports (not in Helm chart)
_ray_ports = [
    "10001:10001",  # Ray client port
    "8265:8265",  # Ray dashboard
]

# Maps host-side port → dotted path in values-k3d.yaml where NodePort is defined.
# Read at cluster-create time so a chart change propagates without editing this file.
_helm_nodeport_map = [
    ("15566", ("apiserver", "service", "nodePort")),  # Michelangelo API Server
    ("8081", ("envoy", "service", "nodePort")),  # Envoy gRPC-Web proxy
    ("8090", ("ui", "service", "nodePort")),  # Michelangelo UI
    ("8088", ("cadence", "web", "service", "nodePort")),  # Cadence Web
    ("8080", ("temporal", "web", "service", "nodePort")),  # Temporal Web
]


def _helm_chart_ports(workflow: str) -> list[str]:
    """Read control plane NodePorts from values-k3d.yaml.

    Returns host:nodeport strings for k3d's -p flag. NodePorts come from
    values-k3d.yaml (single source of truth). Host ports are sandbox
    conventions for localhost access.

    Cadence Web is included only when workflow=cadence; Temporal Web only
    when workflow=temporal.
    """
    with open(_values_k3d_path) as f:
        values = yaml.safe_load(f) or {}

    ports: list[str] = []
    for host_port, path in _helm_nodeport_map:
        # Skip engine-specific Web UIs based on active workflow
        if path[0] == "cadence" and workflow != "cadence":
            continue
        if path[0] == "temporal" and workflow != "temporal":
            continue
        node = values
        for key in path:
            node = (node or {}).get(key)
        if node is None:
            raise ValueError(
                f"values-k3d.yaml is missing NodePort at "
                f"{'.'.join(str(k) for k in path)} "
                f"(needed for host port {host_port})"
            )
        ports.append(f"{host_port}:{node}")
    return ports


def init_arguments(p: argparse.ArgumentParser):
    """Initialize command-line arguments for the sandbox CLI."""
    sp = p.add_subparsers(dest="action", required=True)

    create_p = sp.add_parser("create", help="Create and start the cluster.")
    create_p.add_argument(
        "--exclude",
        help=(
            "Excludes specified services. "
            "Control plane (Helm): apiserver, controllermgr, ui, worker. "
            "Infrastructure: prometheus, grafana, ray, spark."
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
        "--wait-timeout",
        type=int,
        default=600,
        help="Seconds to wait for pods to be ready (default: 600).",
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

    sync_p = sp.add_parser(
        "sync",
        help=(
            "Redeploy services into an existing cluster, skipping cluster creation "
            "and image import. Falls back to a full create if the cluster does not "
            "exist."
        ),
    )
    sync_p.add_argument(
        "--exclude",
        help=(
            "Excludes specified services. "
            "Control plane (Helm): apiserver, controllermgr, ui, worker. "
            "Infrastructure: prometheus, grafana, ray, spark."
        ),
        nargs="+",
        default=[],
    )
    sync_p.add_argument(
        "--workflow",
        choices=["cadence", "temporal"],
        default="cadence",
        help="Choose workflow engine: cadence or temporal (default: cadence).",
    )
    sync_p.add_argument(
        "--wait-timeout",
        type=int,
        default=600,
        help="Seconds to wait for pods to be ready (default: 600).",
    )
    sync_p.add_argument(
        "--include-experimental",
        help="Include experimental services.",
        nargs="+",
        default=[],
    )
    sync_p.add_argument(
        "--set",
        dest="helm_set",
        metavar="KEY=VALUE",
        action="append",
        default=[],
        help="Pass arbitrary --set KEY=VALUE flags through to helm upgrade/install.",
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
    if ns.action == "sync":
        return _sync(ns)
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
    infra_ports = [
        p for p in _infra_ports if _infra_port_owner.get(p) not in ns.exclude
    ]
    ports = infra_ports + _helm_chart_ports(ns.workflow)
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

    _exec(*args)

    _deploy_services(ns)


def _sync(ns: argparse.Namespace):
    """Restart only Michelangelo app services in an existing cluster.

    Infrastructure (MySQL, Cadence, MinIO, Grafana, Prometheus, kuberay,
    spark-operator) is left running as-is.  Only the Michelangelo application
    pods (apiserver, envoy, ui) are restarted so that a new image/config is
    picked up quickly without touching the long-initializing infra.

    If the cluster does not exist, falls back to a full ``create``.  When the
    cluster already exists the k3d cluster creation and ``k3d image import``
    steps are skipped — the examples image is already present in the k3s
    containerd content store from the previous run.  All Kubernetes resources
    are deleted and re-created so each CI run starts with a clean application
    state.
    """
    assert ns

    cluster_exists = (
        subprocess.run(
            ["k3d", "cluster", "get", _michelangelo_sandbox_kube_cluster_name],
            capture_output=True,
        ).returncode
        == 0
    )

    if not cluster_exists:
        print("No existing cluster found — performing a full create.")
        return _create(ns)

    print(
        "Existing cluster found — restarting app services "
        "(leaving infrastructure running)."
    )

    # Start the cluster in case it was stopped at the end of a previous run.
    _exec("k3d", "cluster", "start", _michelangelo_sandbox_kube_cluster_name)

    # Wait for the API server to become reachable after start.
    _exec(
        "kubectl",
        "wait",
        "--for=condition=ready",
        "node",
        "--all",
        "--timeout=120s",
    )

    # Upgrade or install the control plane via Helm.
    # Infrastructure (mysql, cadence, minio, grafana, prometheus) is left running.

    _refresh_mysql_schema()

    _ensure_credentials_secret()
    _helm_ensure_repos()
    helm_args = _build_helm_set_args(ns)

    # Check if there is a healthy deployed release we can upgrade.
    status_result = subprocess.run(
        ["helm", "status", "michelangelo", "-o", "json"],
        capture_output=True,
        text=True,
    )
    release_healthy = (
        status_result.returncode == 0 and '"status":"deployed"' in status_result.stdout
    )

    if release_healthy:
        # Healthy release: upgrade in-place, keeping infra running.
        _exec(
            "helm",
            "upgrade",
            "michelangelo",
            str(_chart_dir),
            "-f",
            str(_chart_dir / "values-k3d.yaml"),
            "--dependency-update",
            "--reuse-values",
            *helm_args,
        )
        # Force-restart app deployments so they always pick up the latest
        # configmap values (helm upgrade only restarts pods when the pod
        # template spec changes, but values-only changes may not alter it).
        for deploy in (
            "michelangelo-apiserver",
            "michelangelo-controllermgr",
            "michelangelo-worker",
        ):
            subprocess.run(
                [
                    "kubectl",
                    "rollout",
                    "restart",
                    f"deployment/{deploy}",
                    "-n",
                    "default",
                ],
                capture_output=True,
            )
        # Wait for the restarted rollouts to complete before proceeding.
        for deploy in (
            "michelangelo-apiserver",
            "michelangelo-controllermgr",
            "michelangelo-worker",
        ):
            subprocess.run(
                [
                    "kubectl",
                    "rollout",
                    "status",
                    f"deployment/{deploy}",
                    "-n",
                    "default",
                    "--timeout=300s",
                ],
                capture_output=False,
            )
    else:
        # Missing or broken release: uninstall cleanly, then reinstall from scratch.
        subprocess.run(
            ["helm", "uninstall", "michelangelo", "--ignore-not-found", "--wait"],
            capture_output=False,
        )
        # After uninstall, force-delete any remaining Services from the chart
        # to free their NodePorts before reinstalling.
        _helm_delete_services(helm_args)
        _helm_adopt_orphaned_resources(helm_args)
        _exec(
            "helm",
            "install",
            "michelangelo",
            str(_chart_dir),
            "-f",
            str(_chart_dir / "values-k3d.yaml"),
            "--dependency-update",
            *helm_args,
        )

    _helm_wait(ns)


def _refresh_mysql_schema():
    """Drop and recreate the michelangelo database from the current schema.

    The schema lives in mysql-ingester.yaml as a ConfigMap that an init Job
    applies via `mysql < init-schema.sql`. The schema uses CREATE TABLE IF
    NOT EXISTS, so re-running the Job against an existing database is a
    no-op and won't pick up renames or column changes. To get a clean
    application of the current schema we drop the database first, then
    re-apply the init Job.
    """
    print("Refreshing MySQL schema (drop + recreate michelangelo database)...")
    subprocess.run(
        [
            "kubectl",
            "exec",
            "mysql",
            "--",
            "mysql",
            "-uroot",
            "-proot",
            "-e",
            "DROP DATABASE IF EXISTS michelangelo;",
        ],
        check=True,
    )
    # The init Job from the previous sync is already in Completed state;
    # kubectl apply on a Completed Job is a no-op (Job spec is immutable),
    # so we have to delete it before re-apply.
    subprocess.run(
        ["kubectl", "delete", "job", "ingester-schema-init", "--ignore-not-found=true"],
        check=False,
    )
    _kube_apply(_dir / "resources" / "mysql-ingester.yaml")
    print("Waiting for ingester-schema-init Job to complete...")
    subprocess.run(
        [
            "kubectl",
            "wait",
            "--for=condition=complete",
            "job/ingester-schema-init",
            "--timeout=120s",
        ],
        check=True,
    )


def _helm_ensure_repos():
    """Add cadence and temporal helm repos if not already present."""
    try:
        helm_existing_repos = subprocess.check_output(["helm", "repo", "list"]).decode()
    except subprocess.CalledProcessError:
        helm_existing_repos = ""
    if "cadence-workflow" not in helm_existing_repos:
        _exec(
            "helm",
            "repo",
            "add",
            "cadence-workflow",
            "https://cadence-workflow.github.io/cadence-charts",
        )
    if "temporal" not in helm_existing_repos:
        _exec("helm", "repo", "add", "temporal", "https://go.temporal.io/helm-charts")


def _helm_delete_services(helm_args: list[str]):
    """Delete Services that would conflict with the chart's NodePorts.

    After helm uninstall, old Services (possibly with different names from
    a previous install structure) can still hold NodePorts. We scan all
    Services in the cluster for conflicting NodePorts and delete them.
    """
    # Collect the NodePorts the chart wants to allocate.
    result = subprocess.run(
        [
            "helm",
            "template",
            "michelangelo",
            str(_chart_dir),
            "-f",
            str(_chart_dir / "values-k3d.yaml"),
            *helm_args,
        ],
        capture_output=True,
        text=True,
    )
    wanted_ports: set[int] = set()
    if result.returncode == 0:
        for doc in yaml.safe_load_all(result.stdout):
            if not doc or doc.get("kind") != "Service":
                continue
            for port in (doc.get("spec") or {}).get("ports") or []:
                if np := port.get("nodePort"):
                    wanted_ports.add(int(np))

    if not wanted_ports:
        return

    # Find any Services in the cluster using those NodePorts and delete them.
    _jsonpath = (
        "{range .items[*]}"
        "{.metadata.namespace}/{.metadata.name}"
        ":{.spec.ports[*].nodePort} {end}"
    )
    all_svcs = subprocess.run(
        [
            "kubectl",
            "get",
            "service",
            "--all-namespaces",
            "-o",
            f"jsonpath={_jsonpath}",
        ],
        capture_output=True,
        text=True,
    )
    for entry in all_svcs.stdout.split():
        if ":" not in entry:
            continue
        ns_name, ports_str = entry.split(":", 1)
        namespace, name = ns_name.split("/", 1)
        for p in ports_str.split():
            try:
                if int(p) in wanted_ports:
                    print(
                        f"[sandbox] deleting conflicting service"
                        f" {namespace}/{name} (NodePort {p})"
                    )
                    subprocess.run(
                        [
                            "kubectl",
                            "delete",
                            "service",
                            name,
                            "-n",
                            namespace,
                            "--ignore-not-found=true",
                        ],
                        capture_output=True,
                    )
                    break
            except ValueError:
                pass


def _helm_adopt_orphaned_resources(helm_args: list[str]):
    """Clean up resources that would block helm upgrade --install.

    Helm 3 refuses to manage resources missing its ownership annotations.
    We render the chart manifests and for each resource that exists in the
    cluster WITHOUT Helm ownership labels, we delete it so the install can
    recreate it cleanly. Resources already managed by Helm (correct labels)
    are left untouched.
    """
    result = subprocess.run(
        [
            "helm",
            "template",
            "michelangelo",
            str(_chart_dir),
            "-f",
            str(_chart_dir / "values-k3d.yaml"),
            *helm_args,
        ],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        return
    for doc in yaml.safe_load_all(result.stdout):
        if not doc:
            continue
        kind = doc.get("kind", "")
        name = (doc.get("metadata") or {}).get("name", "")
        namespace = (doc.get("metadata") or {}).get("namespace", "default")
        if not kind or not name:
            continue
        # Check if this resource exists and lacks Helm ownership annotations.
        get_result = subprocess.run(
            [
                "kubectl",
                "get",
                f"{kind.lower()}/{name}",
                "-n",
                namespace,
                "-o",
                "jsonpath={.metadata.annotations.meta\\.helm\\.sh/release-name}",
            ],
            capture_output=True,
            text=True,
        )
        if get_result.returncode != 0:
            continue  # resource doesn't exist — no action needed
        if get_result.stdout.strip() == "michelangelo":
            continue  # already owned by this release — leave it
        # Resource exists but is not owned by Helm — delete it so Helm can recreate.
        subprocess.run(
            [
                "kubectl",
                "delete",
                f"{kind.lower()}/{name}",
                "-n",
                namespace,
                "--ignore-not-found=true",
            ],
            capture_output=True,
        )


def _deploy_app_services(ns: argparse.Namespace):
    """Install the Michelangelo control plane via Helm."""
    _ensure_credentials_secret()
    _helm_ensure_repos()
    helm_args = _build_helm_set_args(ns)
    _helm_adopt_orphaned_resources(helm_args)
    _exec(
        "helm",
        "upgrade",
        "--install",
        "michelangelo",
        str(_chart_dir),
        "-f",
        str(_chart_dir / "values-k3d.yaml"),
        "--dependency-update",
        *helm_args,
    )
    _helm_wait(ns)


def _helm_wait(ns: argparse.Namespace):
    """Wait for the Michelangelo Helm release pods to become ready.

    Uses a two-stage wait:
    1. Wait for the apiserver Deployment to become Available — waits on the
       Deployment object (created immediately by Helm) so there is no
       'no matching resources found' race. The apiserver runs a schema-init
       container so it takes 30-60s longer than the other services.
    2. Wait for all remaining Helm-managed Deployments to become Available.
    """
    timeout = getattr(ns, "wait_timeout", 600)
    instance_selector = "app.kubernetes.io/instance=michelangelo"

    # Stage 1: apiserver Deployment (schema-init can take 30-60s)
    print("Waiting for apiserver to become available (schema-init runs first)...")
    _exec(
        "kubectl",
        "wait",
        "deployment",
        "-l",
        f"{instance_selector},app.kubernetes.io/component=apiserver",
        "--for=condition=available",
        "--timeout=180s",
    )

    # Stage 2: remaining Helm-managed Deployments
    print("Waiting for remaining control plane services...")
    _exec(
        "kubectl",
        "wait",
        "deployment",
        "-l",
        instance_selector,
        "--for=condition=available",
        f"--timeout={timeout}s",
    )


def _build_helm_set_args(ns: argparse.Namespace) -> list[str]:
    """Convert sandbox CLI flags to Helm --set arguments for the control plane."""
    args = []

    # Workflow engine — cadence is the default in values-k3d.yaml.
    # Always set the engine explicitly so that switching --workflow between
    # runs (e.g. cadence → temporal) overrides any --reuse-values residue.
    if ns.workflow == "temporal":
        args += [
            "--set",
            "workflow.engine=temporal",
            "--set",
            "workflow.endpoint=michelangelo-temporal-frontend:7233",
            "--set",
            "cadence.enabled=false",  # ensure cadence subchart is off
            "--set",
            "temporal.enabled=true",  # enable temporal subchart
        ]
    else:
        args += [
            "--set",
            "workflow.engine=cadence",
            "--set",
            "workflow.endpoint=michelangelo-cadence-frontend:7833",
            "--set",
            "temporal.enabled=false",  # ensure temporal subchart is off
            "--set",
            "cadence.enabled=true",
        ]

    # Service exclusions → enabled=false toggles
    exclude_map = {
        "apiserver": "apiserver.enabled=false",
        "ui": "ui.enabled=false",
        "worker": "worker.enabled=false",
        "controllermgr": "controllermgr.enabled=false",
    }
    for svc, helm_arg in exclude_map.items():
        if svc in getattr(ns, "exclude", []):
            args += ["--set", helm_arg]

    # envoy is paired with ui — disable both together
    if "ui" in getattr(ns, "exclude", []):
        args += ["--set", "envoy.enabled=false"]

    # Arbitrary --set passthrough from the caller (e.g. CI workflow)
    for kv in getattr(ns, "helm_set", []):
        args += ["--set", kv]

    return args


def _deploy_services(ns: argparse.Namespace):
    assert ns
    resources = [
        "boot.yaml",
        "mysql.yaml",  # MySQL database
        "mysql-ingester.yaml",  # Auto-generated ingester schema from protobuf
        "michelangelo-config.yaml",
    ]
    links = []

    # Both Cadence and Temporal are now Helm subcharts — engine switching
    # is handled by cadence.enabled/temporal.enabled --set flags in
    # _build_helm_set_args(). No separate helm uninstall needed.

    if ns.workflow == "cadence":
        # Cadence is now installed as a Helm subchart (cadence.enabled=true in
        # values-k3d.yaml) — no longer deployed as a bare Pod via cadence.yaml.
        # The Web UI link is printed in helm install NOTES.txt.
        links.append(
            (
                "Cadence Web UI",
                "http://localhost:8088",
                "",
            )
        )
    elif ns.workflow == "temporal":
        # If switching from a previous cadence install, remove cadence pods.
        subprocess.run(
            [
                "kubectl",
                "delete",
                "pod",
                "cadence",
                "cadence-web",
                "--ignore-not-found=true",
                "--grace-period=0",
            ],
            capture_output=True,
            check=False,
        )
        subprocess.run(
            ["kubectl", "delete", "svc", "cadence", "--ignore-not-found=true"],
            capture_output=True,
            check=False,
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

    # KubeRay History Server (core resource, deployed alongside MinIO)
    resources.append("history-server.yaml")
    links.append(
        (
            "Ray History Server",
            "http://localhost:3001",
            "",
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
        # Installed via Helm by _deploy_app_services() below.
        pass
    if "ui" not in ns.exclude:
        # Installed via Helm by _deploy_app_services() below.
        links.append(
            (
                "Michelangelo UI",
                "http://localhost:8090",
                "",
            )
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
    bucket_names = ["logs", "default", "deploy-models", "ray-history"]
    if "mlflow" in ns.include_experimental:
        bucket_names.append("mlflow")
        print("🪣 Adding MLflow bucket to S3 setup")

    # Create bucket setup with dynamic bucket list
    _create_bucket_setup(bucket_names)
    for r in resources:
        _kube_apply(_dir / "resources" / r)

    # Create credentials secrets only if they don't already exist.
    _ensure_credentials_secret()
    # Patch michelangelo-config to match the live secret values.
    _sync_config_from_secret()

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

    _kube_wait(timeout=getattr(ns, "wait_timeout", 600))

    # Install the Michelangelo control plane (apiserver, envoy, ui, worker,
    # controllermgr, and Cadence or Temporal subchart) via Helm.
    # Must happen BEFORE domain registration — Cadence frontend only exists
    # after helm install.
    _deploy_app_services(ns)

    if ns.workflow == "cadence":
        _create_cadence_domain(links)

    # Create separate compute cluster if requested
    create_compute_cluster = getattr(ns, "create_compute_cluster", False)
    compute_cluster_name = getattr(
        ns, "compute_cluster_name", _default_compute_kube_cluster_name
    )
    if create_compute_cluster:
        _create_compute_cluster(compute_cluster_name)
        _create_compute_cluster_crd(compute_cluster_name)
        _apply_compute_cluster_rbac(compute_cluster_name)
        _create_compute_cluster_secrets(compute_cluster_name)
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
        'value: "logs,default,deploy-models"', f'value: "{bucket_names_str}"'
    )

    # Create temporary file with modified content
    with tempfile.NamedTemporaryFile(
        mode="w", suffix=".yaml", delete=False
    ) as temp_file:
        temp_file.write(modified_content)
        temp_file.flush()

        # Apply the modified bucket setup
        _exec("kubectl", "apply", "-f", temp_file.name)

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
        "upgrade",
        "--install",
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
        "upgrade",
        "--install",
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

    _import_kuberay_images()


_KUBERAY_IMAGES = [
    "ghcr.io/michelangelo-ai/kuberay-collector:main",
    "ghcr.io/michelangelo-ai/kuberay-historyserver:main",
]


def _import_kuberay_images():
    """Pull kuberay images from GHCR and import them into k3d.

    Non-fatal: prints a warning on failure since the collector sidecar and
    history server are optional for basic sandbox usage.
    """
    for image in _KUBERAY_IMAGES:
        print(f"Importing {image} into k3d...")
        pull = subprocess.run(
            ["docker", "pull", image],
            capture_output=True,
        )
        if pull.returncode != 0:
            print(f"Warning: could not pull {image}. Skipping.")
            continue
        result = subprocess.run(
            [
                "k3d",
                "image",
                "import",
                image,
                "-c",
                _michelangelo_sandbox_kube_cluster_name,
            ],
            capture_output=True,
        )
        if result.returncode != 0:
            print(f"Warning: could not import {image} into k3d.")
        else:
            print(f"Successfully imported {image} into k3d.")


def _create_cadence_domain(links):
    """Register the Cadence domain, treating 'already exists' as success.

    On a fresh cluster the Cadence frontend takes 60-90 s to start, so we
    retry up to 20 times.  When infrastructure is kept running between CI
    runs the domain will already be registered; that is not an error.
    """
    # Wait for Cadence frontend to be ready before registering domain.
    print("Waiting for Cadence frontend to be ready...")
    _exec(
        "kubectl",
        "wait",
        "--for=condition=available",
        "deployment",
        "-l",
        "app.kubernetes.io/name=cadence,app.kubernetes.io/component=frontend",
        "--timeout=300s",
    )
    pod_name = uuid.uuid4().hex
    args = [
        "kubectl",
        "run",
        pod_name,
        "--restart=Never",
        "--rm",
        "--stdin",
        "--image",
        "ubercadence/cli:v1.2.6",
        "--env=CADENCE_CLI_ADDRESS=michelangelo-cadence-frontend:7933",
        "--command",
        "--",
        "cadence",
        "--domain",
        _cadence_domain,
        "domain",
        "register",
        "--rd",
        "1",
    ]
    for attempt in range(21):  # 0..20 inclusive = 21 tries
        print("[+]", " ".join(args))
        result = subprocess.run(args, capture_output=True, text=True)
        combined = result.stdout + result.stderr
        if result.returncode == 0:
            return
        if "Domain already exists" in combined or "already registered" in combined:
            print(f"Cadence domain '{_cadence_domain}' already registered — skipping.")
            return
        if attempt < 20:
            print(f"retrying after 5 seconds... (attempt {attempt + 1}/20)")
            # Print captured output so the log is visible
            if combined.strip():
                print(combined.strip())
            time.sleep(5)
    # Last attempt failed — surface the error and exit
    print(combined.strip())
    sys.exit(result.returncode)


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
    # Uninstall the michelangelo Helm release if present.
    # Credential Secrets have resource-policy: keep so they survive uninstall.
    subprocess.run(
        ["helm", "uninstall", "michelangelo"],
        capture_output=True,
        check=False,
    )

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


def _ensure_credentials_secret():
    """Create object-storage-credentials and aws-credentials Secrets only if absent.

    This is deliberately create-only: a sandbox VM that was pre-configured
    with non-default credentials (e.g. the GCP CI runner) keeps its own
    values across every ``ma sandbox sync`` run.  Local dev gets the
    default minioadmin credentials from the YAML files on first create.
    """
    for secret_name, yaml_file in [
        ("object-storage-credentials", "object-storage-credentials.yaml"),
        ("aws-credentials", "aws-credentials.yaml"),
        ("minio-credentials", "minio-credentials.yaml"),
    ]:
        exists = (
            subprocess.run(
                ["kubectl", "get", "secret", secret_name],
                capture_output=True,
            ).returncode
            == 0
        )
        if not exists:
            print(f"Creating {secret_name} Secret from defaults...")
            _kube_apply(_dir / "resources" / yaml_file)
        else:
            print(
                f"Secret '{secret_name}' already exists — "
                f"skipping (preserving VM credentials)."
            )


def _sync_config_from_secret():
    """Patch michelangelo-config ConfigMap credentials from object-storage-credentials.

    Ray pods consume the michelangelo-config ConfigMap via envFrom. After the
    ConfigMap is (re)applied from the YAML file (which contains minioadmin
    defaults), this function overwrites the credential fields with whatever
    is actually in the object-storage-credentials Secret, so all consumers see
    the same credentials.
    """
    result = subprocess.run(
        [
            "kubectl",
            "get",
            "secret",
            "object-storage-credentials",
            "-o",
            "jsonpath={.data.AWS_ACCESS_KEY_ID} {.data.AWS_SECRET_ACCESS_KEY}",
        ],
        capture_output=True,
        text=True,
    )
    if result.returncode != 0:
        # Secret will be created by _ensure_credentials_secret
        return

    import base64

    parts = result.stdout.strip().split()
    if len(parts) != 2:
        return
    access_key = base64.b64decode(parts[0]).decode()
    secret_key = base64.b64decode(parts[1]).decode()

    subprocess.run(
        [
            "kubectl",
            "patch",
            "configmap",
            "michelangelo-config",
            "--patch",
            f'{{"data":{{"AWS_ACCESS_KEY_ID":"{access_key}","AWS_SECRET_ACCESS_KEY":"{secret_key}"}}}}',
        ],
        check=False,
        capture_output=True,
    )


def _kube_apply(path: Path):
    _exec("kubectl", "apply", "-f", str(path))


def _kube_wait(pods: bool = True, jobs: bool = True, timeout: int = 600):
    if pods:
        _exec(
            "kubectl",
            "wait",
            "--for=condition=ready",
            "pod",
            "-l",
            "app",
            f"--timeout={timeout}s",
        )
    if jobs:
        _exec(
            "kubectl",
            "wait",
            "--all",
            "jobs",
            "--for=condition=complete",
            f"--timeout={timeout}s",
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


def _setup_inference_server_secrets():
    """Create RBAC and credentials for inference server cluster access.

    Applies an inference-server-manager ServiceAccount with permissions to
    manage Deployments, Services, and ConfigMaps (required for Triton provisioning).
    Stores a long-lived bearer token as a Secret so the clientfactory can build
    a remote kube client for the sandbox cluster using kubernetes.default.svc:443.

    The CA secret (cluster-michelangelo-sandbox-ca-data) is already created by
    the sandbox create flow; we only need to provision the token here.
    """
    cluster_name = _michelangelo_sandbox_kube_cluster_name
    token_secret_name = f"cluster-{cluster_name}-is-token"

    # Check if the token secret already exists to make this idempotent.
    exists = (
        subprocess.run(
            ["kubectl", "get", "secret", token_secret_name],
            capture_output=True,
        ).returncode
        == 0
    )
    if exists:
        print(
            f"Secret '{token_secret_name}' already exists — "
            "skipping inference server credential setup."
        )
        return

    # Apply ServiceAccount + ClusterRole + ClusterRoleBinding.
    _kube_apply(_dir / "resources" / "rbac-inferenceserver.yaml")

    # Mint a long-lived token (same duration as ray-manager) so the sandbox
    # does not require frequent re-creation.
    token_decoded = (
        subprocess.check_output(
            [
                "kubectl",
                "create",
                "token",
                "inference-server-manager",
                "-n",
                "default",
                "--duration=87600h",
            ]
        )
        .decode()
        .strip()
    )

    token_secret = {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {"name": token_secret_name, "namespace": "default"},
        "stringData": {"token": token_decoded},
    }

    with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
        yaml.dump(token_secret, f)
        f.flush()
        _exec("kubectl", "apply", "-f", f.name)

    print(f"Created inference server credentials for cluster '{cluster_name}'")


def _create_inference_demo_crs():
    """Create an inference server for the sandbox cluster for demo purposes."""
    print("🚀 Setting up Michelangelo AI Inference Demo...")

    # Setup istio with Gateway API
    # This allows usage of HTTPRoutes to route traffic to the inference server.
    _setup_istio_with_gateway_api()

    # Create the SA, RBAC, and token secret that the clientfactory uses to
    # connect to the sandbox cluster as a ClusterTarget.
    _setup_inference_server_secrets()

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
    print("Installing Gateway API CRDs...")
    _exec(
        "kubectl",
        "apply",
        "-f",
        "https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.1.0/standard-install.yaml",
    )
    _exec(
        "kubectl",
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
        [
            "kubectl",
            "-n",
            "default",
            "port-forward",
            "svc/ma-gateway-istio",
            "8080:80",
        ],
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
