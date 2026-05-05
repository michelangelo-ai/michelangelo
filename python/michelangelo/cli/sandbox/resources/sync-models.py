#!/usr/bin/env python3
"""Sync Triton models from S3 and reconcile loaded set with the desired ConfigMap.

Talks directly to each Triton pod backing the inference Service rather than the
Service VIP, so model load/unload state matches across all replicas instead of
landing on whichever pod the Service round-robins to.
"""

import json
import os
import re
import ssl
import subprocess
import sys
import time
import urllib.error
import urllib.request

MODEL_BASE_DIR = "/mnt/models"
INFERENCE_SERVERS_FILE = "/config/inference-servers/servers.txt"
DEFAULT_BUCKET = "s3://deploy-models"
SYNC_INTERVAL_SECONDS = 60
HTTP_TIMEOUT_SECONDS = 10
VERSION_DIR_REGEX = re.compile(r"^[0-9]+$")

# Triton container port. The Service maps :80 -> :8000, but pod-direct calls
# bypass the Service so we hit the container port directly.
TRITON_HTTP_PORT = 8000

# In-cluster Kubernetes API endpoint and ServiceAccount token paths.
KUBE_API = "https://kubernetes.default.svc"
SA_TOKEN_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/token"
SA_CA_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
POD_NAMESPACE = "default"


def run(cmd: list[str], check: bool = True) -> subprocess.CompletedProcess:
    """Run a subprocess and return CompletedProcess; capture stdout/stderr as text."""
    return subprocess.run(cmd, check=check, capture_output=True, text=True)


def http_post_json(url: str, payload: dict) -> tuple[int, str]:
    """POST a JSON body and return (status, response body)."""
    req = urllib.request.Request(
        url,
        data=json.dumps(payload).encode("utf-8"),
        headers={"Content-Type": "application/json"},
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=HTTP_TIMEOUT_SECONDS) as resp:
            return resp.status, resp.read().decode("utf-8")
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode("utf-8", errors="replace")


def list_pod_addresses(service: str) -> list[str]:
    """Return pod IPs backing the given Service via its Endpoints object."""
    with open(SA_TOKEN_PATH) as f:
        token = f.read().strip()
    ctx = ssl.create_default_context(cafile=SA_CA_PATH)
    req = urllib.request.Request(
        f"{KUBE_API}/api/v1/namespaces/{POD_NAMESPACE}/endpoints/{service}",
        headers={"Authorization": f"Bearer {token}"},
    )
    try:
        with urllib.request.urlopen(req, context=ctx,
                                    timeout=HTTP_TIMEOUT_SECONDS) as resp:
            data = json.loads(resp.read())
    except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as e:
        print(f"  failed to list endpoints for {service}: {e}")
        return []
    return [
        addr["ip"]
        for subset in data.get("subsets") or []
        for addr in subset.get("addresses") or []
    ]


def triton_ready(host: str) -> bool:
    """Return True if Triton's /v2/health/ready endpoint responds 200."""
    try:
        with urllib.request.urlopen(
            f"http://{host}:{TRITON_HTTP_PORT}/v2/health/ready",
            timeout=HTTP_TIMEOUT_SECONDS,
        ) as resp:
            return resp.status == 200
    except (urllib.error.URLError, TimeoutError):
        return False


def loaded_models(host: str) -> set[str]:
    """Return the set of model names currently loaded and ready on Triton."""
    status, body = http_post_json(
        f"http://{host}:{TRITON_HTTP_PORT}/v2/repository/index",
        {"ready": True},
    )
    if status != 200:
        return set()
    try:
        return {entry["name"] for entry in json.loads(body) if entry.get("name")}
    except (json.JSONDecodeError, TypeError):
        return set()


def load_model(host: str, name: str) -> None:
    """Ask Triton to load a model from its model repository."""
    status, body = http_post_json(
        f"http://{host}:{TRITON_HTTP_PORT}/v2/repository/models/{name}/load", {}
    )
    if status == 200:
        print(f"    {host}: loaded {name}")
    else:
        print(f"    {host}: failed to load {name} (HTTP {status}): {body}")


def unload_model(host: str, name: str) -> None:
    """Ask Triton to unload a model from memory."""
    status, body = http_post_json(
        f"http://{host}:{TRITON_HTTP_PORT}/v2/repository/models/{name}/unload", {}
    )
    if status == 200:
        print(f"    {host}: unloaded {name}")
    else:
        print(f"    {host}: failed to unload {name} (HTTP {status}): {body}")


def has_valid_model_structure(model_dir: str) -> bool:
    """True if model_dir holds a numeric version subdir containing model.pt."""
    if not os.path.isdir(model_dir):
        return False
    for entry in os.listdir(model_dir):
        version_dir = os.path.join(model_dir, entry)
        if (
            os.path.isdir(version_dir)
            and VERSION_DIR_REGEX.match(entry)
            and os.path.isfile(os.path.join(version_dir, "model.pt"))
        ):
            return True
    return False


def sync_model(storage_path: str, model_dir: str, endpoint_url: str) -> None:
    """Re-download a model from S3 into model_dir, replacing any prior contents."""
    print(f"  syncing {storage_path} -> {model_dir}")
    if os.path.isdir(model_dir):
        # Stale or partial download; remove and re-fetch to avoid mixed-version state.
        run(["rm", "-rf", model_dir])
    os.makedirs(model_dir, exist_ok=True)
    run(
        [
            "aws",
            "s3",
            "sync",
            storage_path,
            f"{model_dir}/",
            "--exact-timestamps",
            "--endpoint-url",
            endpoint_url,
        ],
        check=False,
    )


def read_servers(path: str) -> list[str]:
    """Read the inference-servers list (one name per line, # for comments)."""
    if not os.path.isfile(path):
        return []
    out = []
    with open(path) as f:
        for raw in f:
            line = raw.strip()
            if line and not line.startswith("#"):
                out.append(line)
    return out


def read_model_list(server: str) -> list[dict]:
    """Read the per-server model list ConfigMap; return [] if missing or malformed."""
    path = f"/config/{server}/model-list.json"
    if not os.path.isfile(path):
        return []
    try:
        with open(path) as f:
            return json.load(f)
    except (json.JSONDecodeError, OSError) as e:
        print(f"  failed to read {path}: {e}")
        return []


def reconcile_pod(pod_ip: str, desired: dict) -> None:
    """Reconcile load/unload state on a single Triton pod."""
    if not triton_ready(pod_ip):
        print(f"  {pod_ip}: not ready, skipping")
        return
    currently_loaded = loaded_models(pod_ip)
    print(f"  {pod_ip}: loaded {sorted(currently_loaded)}")
    for name in currently_loaded - desired.keys():
        unload_model(pod_ip, name)
    for name in desired.keys() - loaded_models(pod_ip):
        load_model(pod_ip, name)


def reconcile_server(server: str, endpoint_url: str) -> None:
    """One reconcile pass for an inference server: sync S3, then load/unload per pod."""
    print(f"--- {server} ---")
    config = read_model_list(server)
    desired = {entry["name"]: entry for entry in config if entry.get("name")}
    print(f"  desired: {sorted(desired)}")

    server_dir = os.path.join(MODEL_BASE_DIR, server)
    os.makedirs(server_dir, exist_ok=True)
    for name, entry in desired.items():
        storage_path = entry.get("storage_path") or f"{DEFAULT_BUCKET}/{name}/"
        model_dir = os.path.join(server_dir, name)
        if not has_valid_model_structure(model_dir):
            sync_model(storage_path, model_dir, endpoint_url)

    pod_ips = list_pod_addresses(f"{server}-inference-service")
    if not pod_ips:
        print("  no Triton pods found, skipping load/unload reconcile")
        return
    print(f"  pods: {pod_ips}")
    for pod_ip in pod_ips:
        reconcile_pod(pod_ip, desired)


def configure_aws() -> None:
    """Write AWS credentials and S3 endpoint into the awscli config."""
    endpoint = os.environ["AWS_ENDPOINT_URL"]
    print(f"configuring aws cli for endpoint {endpoint}")
    run(["aws", "configure", "set", "aws_access_key_id",
         os.environ["AWS_ACCESS_KEY_ID"]])
    run(
        [
            "aws",
            "configure",
            "set",
            "aws_secret_access_key",
            os.environ["AWS_SECRET_ACCESS_KEY"],
        ]
    )
    run(["aws", "configure", "set", "default.s3.endpoint_url", endpoint])


def main() -> int:
    """Entrypoint: sync loop, one pass per SYNC_INTERVAL_SECONDS over all servers."""
    configure_aws()
    os.makedirs(MODEL_BASE_DIR, exist_ok=True)

    servers = read_servers(INFERENCE_SERVERS_FILE)
    print(f"sync daemon on node {os.environ.get('NODE_NAME', '?')}, servers: {servers}")

    endpoint_url = os.environ["AWS_ENDPOINT_URL"]
    while True:
        print("=" * 40)
        for server in servers:
            try:
                reconcile_server(server, endpoint_url)
            except Exception as e:
                print(f"  {server}: reconcile failed: {e}")
        print(f"sleeping {SYNC_INTERVAL_SECONDS}s")
        time.sleep(SYNC_INTERVAL_SECONDS)


if __name__ == "__main__":
    sys.exit(main())
