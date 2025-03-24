import json
import os
import sys
import argparse
import shutil
import subprocess
import tempfile
import time
import uuid
from pathlib import Path

short_description = "Manage the sandbox cluster."

description = """
Michelangelo Sandbox is a lightweight version of the Michelangelo platform, tailored for local development and testing.
This tool helps you create and manage a sandbox cluster directly on your machine.
"""

_dir = Path(__file__).parent

_kube_name = "michelangelo-sandbox"
_kube_ports = [
    "3306:30001",  # MySQL
    "7833:30002",  # Cadence gRPC
    "7933:30003",  # Cadence TChannel
    "8088:30004",  # Cadence Web
    "9091:30007",  # MinIO
    "9090:30008",  # MinIO
    "14566:30009",  # Michelangelo API Server
    "8081:30010", # Envoy gRPC --> gRPC-web proxy
    "3000:30000",   # Grafana (NodePort)
    "3100:31000",   # Loki (NodePort)
]
_cadence_domain = "default"


def init_arguments(p: argparse.ArgumentParser):
    sp = p.add_subparsers(dest="action", required=True)

    create_p = sp.add_parser("create", help="Create and start the cluster.")
    create_p.add_argument("--exclude", help="Excludes the specified services.", nargs="+", default=[])
    create_p.add_argument(
        "--monitor",
        action="store_true",
        help="Install monitoring stack only (Loki + Grafana)."
    )

    _ = sp.add_parser("delete", help="Delete the cluster.")
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
    _assert_command("kubectl", "kubectl not found, please install it: https://kubernetes.io/docs/tasks/tools/#kubectl")

    if ns.action == "create":
        return _create(ns)
    if ns.action == "delete":
        return _delete(ns)
    if ns.action == "start":
        return _start(ns)
    if ns.action == "stop":
        return _stop(ns)

    raise ValueError(f"Unsupported action: {ns.action}")


def _create(ns: argparse.Namespace):
    assert ns
    args = ["k3d", "cluster", "create", _kube_name, "--servers", "1", "--agents", "1"]
    for p in _kube_ports:
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

    resources = ["boot.yaml",
                 "mysql.yaml",
                 "cadence.yaml",
                 "minio.yaml",
                 "michelangelo-config.yaml",
                 "fluent-bit-aws-secret.yaml",
                 "fluent-bit-config.yaml"]
    if "worker" not in ns.exclude:
        resources.append("michelangelo-worker.yaml")
    if "apiserver" not in ns.exclude:
        resources.append("michelangelo-apiserver.yaml")
    if "controllermgr" not in ns.exclude:
        resources.append("michelangelo-controllermgr.yaml")
    if "ui" not in ns.exclude:
        resources.append("envoy.yaml")

    for r in resources:
        _kube_create(_dir / "resources" / r)

    _exec("kubectl", "create", "-k", "github.com/ray-project/kuberay/ray-operator/config/default?ref=v1.0.0")

    _exec("kubectl", "wait", "--all", "pods", "--for=condition=ready", "--timeout=600s")
    _exec("kubectl", "-n", "ray-system", "wait", "--all", "deployments", "--for=condition=available", "--timeout=600s")

    links = []

    _kube_run(
        image="ubercadence/cli:v1.2.6",
        command=["cadence", "--domain", _cadence_domain, "domain", "register", "--rd", "1"],
        env={
            "CADENCE_CLI_ADDRESS": "cadence:7933",
        },
        retry_attempts=3,
    )
    links.append(("Cadence Dashboard", f"http://localhost:8088/domains/{_cadence_domain}/workflows", ""))

    print()
    print("Sandbox created. To access the services, please use the following links:")
    for title, url, comment in links:
        print(f"  - {title}: {url} {comment}")

    if ns.monitor:
        # Install Fluent Bit with Loki backend
        _exec("helm", "repo", "add", "fluent", "https://fluent.github.io/helm-charts")
        _exec("helm", "repo", "update")

        _exec(
            "helm", "upgrade", "--install", "fluent-bit", "fluent/fluent-bit",
            "--namespace", "kube-system",
            "--set", "backend.type=loki",
            "--set", "backend.loki.host=loki.default.svc.cluster.local",
            "--set", "backend.loki.port=3100"
        )
        # Add Helm repo for Grafana
        _exec("helm", "repo", "add", "grafana", "https://grafana.github.io/helm-charts")
        _exec("helm", "repo", "update")

        # Install Loki (backend only)
        _exec(
            "helm", "upgrade", "--install", "loki", "grafana/loki-stack",
            "--namespace", "kube-system",
            "--set", "grafana.enabled=false",
            "--set", "promtail.enabled=false",
            "--set", "loki.enabled=true"
        )

        # Install Grafana with Loki datasource
        _exec(
            "helm", "upgrade", "--install", "grafana", "grafana/grafana",
            "--namespace", "kube-system",
            "--set", "adminPassword=admin",
            "--set", "service.type=NodePort",
            "--set", "service.nodePort=30000",
            "--set", "datasources.datasources\\.yaml.apiVersion=1",
            "--set", "datasources.datasources\\.yaml.datasources[0].name=Loki",
            "--set", "datasources.datasources\\.yaml.datasources[0].type=loki",
            "--set", "datasources.datasources\\.yaml.datasources[0].url=http://loki.kube-system.svc.cluster.local:3100"
        )

        print("\n✅ Monitoring stack installed.")
        print("Grafana: http://localhost:3000 (admin / admin)")
        print("Loki:    http://localhost:3100")

    print()
    print("ok.")


def _delete(ns: argparse.Namespace):
    assert ns
    _exec("k3d", "cluster", "delete", _kube_name)


def _start(ns: argparse.Namespace):
    assert ns
    _exec("k3d", "cluster", "start", _kube_name)


def _stop(ns: argparse.Namespace):
    assert ns
    _exec("k3d", "cluster", "stop", _kube_name)


def _kube_create(path: Path):
    _exec("kubectl", "create", "-f", str(path))


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


if __name__ == "__main__":
    sys.exit(main())
