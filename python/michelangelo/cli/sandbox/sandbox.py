import sys
import argparse
import subprocess
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
]
_cadence_domain = "default"


def init_arguments(p: argparse.ArgumentParser):
    sp = p.add_subparsers(dest="action", required=True)

    _ = sp.add_parser("create", help="Create and start the cluster.")
    _ = sp.add_parser("delete", help="Delete the cluster.")
    _ = sp.add_parser("start", help="Start the cluster.")
    _ = sp.add_parser("stop", help="Stop the cluster.")


def main(args=None):
    p = argparse.ArgumentParser(description=description)
    init_arguments(p)
    ns = p.parse_args(args=args)
    return run(ns)


def run(ns: argparse.Namespace):
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

    _exec(*args)

    for f in ["boot.yaml", "mysql.yaml", "cadence.yaml"]:
        _kube_create(_dir / "resources" / f)

    _exec("kubectl", "create", "-k", "github.com/ray-project/kuberay/ray-operator/config/default?ref=v1.2.2")

    _exec("kubectl", "wait", "--all", "pods", "--for=condition=ready")
    _exec("kubectl", "-n", "ray-system", "wait", "--all", "deployments", "--for=condition=available")

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
) -> int:
    for i in range(retry_attempts + 1):
        try:
            print("[+]", " ".join(args))
            return subprocess.check_call(args)
        except Exception as e:
            if i == retry_attempts:
                raise e
            print("retrying after", retry_delay_seconds, "seconds...")
            time.sleep(retry_delay_seconds)


if __name__ == "__main__":
    sys.exit(main())
