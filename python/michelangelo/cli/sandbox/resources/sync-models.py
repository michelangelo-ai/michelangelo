#!/usr/bin/env python3
"""Sync Triton models from S3 and reconcile loaded set with the desired ConfigMap.

Talks directly to each Triton pod backing the inference Service rather than the
Service VIP, so model load/unload state matches across all replicas instead of
landing on whichever pod the Service round-robins to.

Runs as a DaemonSet. Each instance owns the model repository on its own node and
reconciles only the Triton pods scheduled there, so every replica is served by the
daemon sharing its hostPath.

Each model-list entry carries a rollout phase written by the deployment controller:

  canary   load on the single replica named by ``canary_pod`` only
  staged   load on every replica; readiness does not depend on it yet
  serving  load on every replica; the readiness probe requires it (default)

The daemon only decides *where* a model is loaded. Whether the load succeeded is read
back per replica by the controller through Triton's repository index, so a failed
load is reported rather than retried in a tight loop here.
"""

import json
import os
import re
import ssl
import subprocess
import sys
import tarfile
import tempfile
import time
import urllib.error
import urllib.request

MODEL_BASE_DIR = "/mnt/models"
INFERENCE_SERVERS_FILE = "/config/inference-servers/servers.txt"
DEFAULT_BUCKET = "s3://deploy-models"
SYNC_INTERVAL_SECONDS = 60
HTTP_TIMEOUT_SECONDS = 10
VERSION_DIR_REGEX = re.compile(r"^[0-9]+$")

# A model that dropped out of the desired set keeps serving for this long before it is
# unloaded. Traffic is moved off a model before the controller removes its entry, so the
# grace only has to cover a transient bad read of the ConfigMap, not a route switch.
UNLOAD_GRACE_SECONDS = 150
# A load Triton rejected is retried no more often than this, so the controller sees a
# stable FAILED state instead of a model flapping between LOADING and UNAVAILABLE.
LOAD_RETRY_SECONDS = 300

PHASE_CANARY = "canary"
PHASE_STAGED = "staged"
PHASE_SERVING = "serving"

# Triton container port. The Service maps :80 -> :8000, but pod-direct calls
# bypass the Service so we hit the container port directly.
TRITON_HTTP_PORT = 8000

# In-cluster Kubernetes API endpoint and ServiceAccount token paths.
KUBE_API = "https://kubernetes.default.svc"
SA_TOKEN_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/token"
SA_CA_PATH = "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"
# The inference Service lives in the same namespace as the Triton pods this daemon
# reconciles, which is not always "default" (e.g. per-pipeline namespaces like
# "bert-cola"). Set via the Downward API in model-sync.yaml.tmpl.
POD_NAMESPACE = os.environ.get("POD_NAMESPACE", "default")


class PodStates:
    """Per-(pod, model) timers that survive across reconcile passes.

    ``undesired_since`` records when a loaded model first went missing from the desired
    set; ``failed_load_at`` records the last load attempt Triton rejected.
    """

    def __init__(self) -> None:
        self.undesired_since: dict[tuple[str, str], float] = {}
        self.failed_load_at: dict[tuple[str, str], float] = {}

    def forget_pods(self, live_pods: set[str]) -> None:
        """Drop timers for pods that no longer back the Service."""
        for timers in (self.undesired_since, self.failed_load_at):
            for key in [k for k in timers if k[0] not in live_pods]:
                del timers[key]


# One state holder per inference server, keyed by server name.
_POD_STATES: dict[str, PodStates] = {}


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


def list_pods(service: str, node_name: str | None = None) -> list[tuple[str, str]]:
    """Return (pod IP, pod name) pairs backing the given Service via its Endpoints object.

    Not-ready addresses are included on purpose: with the model-aware readiness probe a
    fresh replica stays not-ready until this daemon has loaded its serving models, so
    reading only the ready addresses would never let a new pod become ready.

    When node_name is given, only pods scheduled on that node are returned.
    """
    with open(SA_TOKEN_PATH) as f:
        token = f.read().strip()
    ctx = ssl.create_default_context(cafile=SA_CA_PATH)
    req = urllib.request.Request(
        f"{KUBE_API}/api/v1/namespaces/{POD_NAMESPACE}/endpoints/{service}",
        headers={"Authorization": f"Bearer {token}"},
    )
    try:
        with urllib.request.urlopen(
            req, context=ctx, timeout=HTTP_TIMEOUT_SECONDS
        ) as resp:
            data = json.loads(resp.read())
    except (urllib.error.URLError, TimeoutError, json.JSONDecodeError) as e:
        print(f"  failed to list endpoints for {service}: {e}")
        return []
    pods: list[tuple[str, str]] = []
    for subset in data.get("subsets") or []:
        for addr in (subset.get("addresses") or []) + (
            subset.get("notReadyAddresses") or []
        ):
            if node_name is not None and addr.get("nodeName") != node_name:
                continue
            ip = addr.get("ip")
            if not ip:
                continue
            name = (addr.get("targetRef") or {}).get("name") or ip
            pods.append((ip, name))
    return pods


def triton_live(host: str) -> bool:
    """Return True if Triton's HTTP server answers /v2/health/live.

    Liveness rather than readiness: readiness is now tied to the serving model set, so a
    replica that is waiting for this daemon to load its models is unready but must still
    be talked to.
    """
    try:
        with urllib.request.urlopen(
            f"http://{host}:{TRITON_HTTP_PORT}/v2/health/live",
            timeout=HTTP_TIMEOUT_SECONDS,
        ) as resp:
            return resp.status == 200
    except (urllib.error.URLError, TimeoutError):
        return False


def repository_index(host: str) -> dict[str, dict]:
    """Return Triton's repository index keyed by model name.

    Every model in the repository is listed. Loaded models carry ``state`` READY,
    LOADING or UNLOADING; a model whose last load failed is UNAVAILABLE with the load
    error in ``reason``; a model that was never loaded has no state at all.
    """
    status, body = http_post_json(
        f"http://{host}:{TRITON_HTTP_PORT}/v2/repository/index", {}
    )
    if status != 200:
        return {}
    try:
        return {
            entry["name"]: entry
            for entry in json.loads(body)
            if isinstance(entry, dict) and entry.get("name")
        }
    except (json.JSONDecodeError, TypeError):
        return {}


def load_model(host: str, name: str) -> bool:
    """Ask Triton to load a model from its model repository; True on success."""
    status, body = http_post_json(
        f"http://{host}:{TRITON_HTTP_PORT}/v2/repository/models/{name}/load", {}
    )
    if status == 200:
        print(f"    {host}: loaded {name}")
        return True
    print(f"    {host}: failed to load {name} (HTTP {status}): {body}")
    return False


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


def safe_extractall(tar: tarfile.TarFile, dest: str) -> None:
    """Extract a tar archive into dest without path-traversal risk.

    Uses filter="data" on Python 3.12+ (PEP 706), which strips absolute paths, ".."
    traversal components, and unsafe symlinks. Older runtimes get an equivalent
    manual member check.
    """
    if sys.version_info >= (3, 12):
        tar.extractall(dest, filter="data")
        return
    real_dest = os.path.realpath(dest)
    safe_members = []
    for member in tar.getmembers():
        if not member.name:
            continue
        member_path = os.path.realpath(os.path.join(dest, member.name))
        if member_path != real_dest and not member_path.startswith(real_dest + os.sep):
            raise ValueError(
                f"refusing to extract '{member.name}': path escapes '{dest}'"
            )
        safe_members.append(member)
    tar.extractall(dest, members=safe_members)


def extract_tar_model(storage_path: str, model_dir: str, endpoint_url: str) -> None:
    """Download a tar-archived model artifact and unpack it into model_dir.

    The packaging step archives directory artifacts before upload, so a pipeline-pushed
    Triton model arrives as one object instead of a prefix. Its members are stored
    relative to the model root, so extracting here yields the layout Triton expects.
    """
    with tempfile.TemporaryDirectory() as tmp_dir:
        archive = os.path.join(tmp_dir, "model.tar")
        result = run(
            ["aws", "s3", "cp", storage_path, archive, "--endpoint-url", endpoint_url],
            check=False,
        )
        if result.returncode != 0 or not os.path.isfile(archive):
            print(f"  failed to download {storage_path}: {result.stderr.strip()}")
            return
        try:
            with tarfile.open(archive) as tar:
                safe_extractall(tar, model_dir)
        except (tarfile.TarError, ValueError) as e:
            print(f"  failed to extract {storage_path}: {e}")


def sync_model(storage_path: str, model_dir: str, endpoint_url: str) -> None:
    """Re-download a model from S3 into model_dir, replacing any prior contents."""
    print(f"  syncing {storage_path} -> {model_dir}")
    if os.path.isdir(model_dir):
        # Stale or partial download; remove and re-fetch to avoid mixed-version state.
        run(["rm", "-rf", model_dir])
    os.makedirs(model_dir, exist_ok=True)
    if storage_path.endswith(".tar"):
        extract_tar_model(storage_path, model_dir, endpoint_url)
        return
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


def read_model_list(server: str) -> list[dict] | None:
    """Read the per-server model list ConfigMap.

    Returns [] when the ConfigMap has no list yet and None when it exists but cannot be
    read, so a transient bad read is not mistaken for "unload everything".
    """
    path = f"/config/{server}/model-list.json"
    if not os.path.isfile(path):
        return []
    try:
        with open(path) as f:
            entries = json.load(f)
    except (json.JSONDecodeError, OSError) as e:
        print(f"  failed to read {path}: {e}")
        return None
    return entries if isinstance(entries, list) else None


def entry_phase(entry: dict) -> str:
    """Return the entry's rollout phase; entries written before phases exist are serving."""
    return entry.get("phase") or PHASE_SERVING


def build_desired(config: list[dict]) -> dict[str, dict]:
    """Collapse the model list into one entry per model name.

    The list holds one entry per (deployment, model). Triton keys its repository by
    model name, so entries sharing a name describe a single model that stays loaded
    until the last deployment referencing it goes away. Every phase counts: a canary
    model has to be on disk before its one replica can load it.
    """
    desired: dict[str, dict] = {}
    for entry in config:
        name = entry.get("name")
        if name:
            desired.setdefault(name, entry)
    return desired


def desired_for_pod(config: list[dict], pod_name: str) -> set[str]:
    """Return the model names this replica should have loaded.

    Staged and serving entries apply to every replica. A canary entry applies only to
    the replica it names, which is how the controller validates a new model on one pod
    before the rest of the cluster loads it.
    """
    wanted: set[str] = set()
    for entry in config:
        name = entry.get("name")
        if not name:
            continue
        if entry_phase(entry) == PHASE_CANARY:
            if entry.get("canary_pod") == pod_name:
                wanted.add(name)
        else:
            wanted.add(name)
    return wanted


def format_claims(config: list[dict]) -> str:
    """Render each desired model with the deployments claiming it, for the sync log."""
    claims: dict[str, set[str]] = {}
    for entry in config:
        name = entry.get("name")
        if not name:
            continue
        owner = entry.get("deployment_name") or "?"
        phase = entry_phase(entry)
        if phase == PHASE_CANARY:
            owner = f"{owner}:canary@{entry.get('canary_pod') or '?'}"
        elif phase != PHASE_SERVING:
            owner = f"{owner}:{phase}"
        claims.setdefault(name, set()).add(owner)
    return (
        ", ".join(
            f"{name}[{','.join(sorted(owners))}]"
            for name, owners in sorted(claims.items())
        )
        or "(none)"
    )


def reconcile_pod(
    pod_ip: str, pod_name: str, wanted: set[str], state: PodStates, now: float
) -> None:
    """Reconcile load/unload state on a single Triton pod."""
    if not triton_live(pod_ip):
        print(f"  {pod_name} ({pod_ip}): triton not up yet, skipping")
        return
    index = repository_index(pod_ip)
    loaded = {
        name
        for name, entry in index.items()
        if entry.get("state") in ("READY", "LOADING")
    }
    failed = {
        name: entry.get("reason")
        for name, entry in index.items()
        if entry.get("state") == "UNAVAILABLE"
        and entry.get("reason")
        and entry.get("reason") != "unloaded"
    }
    print(f"  {pod_name} ({pod_ip}): loaded {sorted(loaded)}, wanted {sorted(wanted)}")

    for name in wanted:
        state.undesired_since.pop((pod_name, name), None)
    for name in sorted(loaded - wanted):
        since = state.undesired_since.setdefault((pod_name, name), now)
        remaining = UNLOAD_GRACE_SECONDS - (now - since)
        if remaining > 0:
            print(f"    {pod_name}: {name} no longer desired, unloading in {int(remaining)}s")
            continue
        unload_model(pod_ip, name)
        state.undesired_since.pop((pod_name, name), None)

    for name in sorted(wanted - loaded):
        key = (pod_name, name)
        if name not in failed:
            # Never attempted, or unloaded: (re)issue the load and forget any old failure.
            state.failed_load_at.pop(key, None)
            if not load_model(pod_ip, name):
                state.failed_load_at[key] = now
            continue
        last_attempt = state.failed_load_at.get(key)
        if last_attempt is not None and now - last_attempt < LOAD_RETRY_SECONDS:
            wait = int(LOAD_RETRY_SECONDS - (now - last_attempt))
            print(f"    {pod_name}: {name} failed to load ({failed[name]}); retry in {wait}s")
            continue
        # Triton accepted the previous load request but the load itself failed. Record
        # the attempt regardless of the HTTP outcome so the model does not flap between
        # LOADING and UNAVAILABLE faster than the controller can observe the failure.
        state.failed_load_at[key] = now
        load_model(pod_ip, name)


def reconcile_server(server: str, endpoint_url: str, node_name: str) -> None:
    """One reconcile pass for an inference server: sync S3, then load/unload per pod.

    Scoped to the Triton pods on this node. The model repository is a hostPath, so a pod
    can only load what this node's daemon downloaded; peer daemons cover other nodes.
    Nodes running no Triton pod skip the download entirely rather than pulling artifacts
    nothing will read.
    """
    print(f"--- {server} ---")
    pods = list_pods(f"{server}-inference-service", node_name)
    state = _POD_STATES.setdefault(server, PodStates())
    state.forget_pods({name for _, name in pods})
    if not pods:
        print(f"  no Triton pods on node {node_name}, nothing to do")
        return
    print(f"  pods on this node: {[name for _, name in pods]}")

    config = read_model_list(server)
    if config is None:
        print("  model list unreadable, leaving loaded models untouched this pass")
        return
    desired = build_desired(config)
    print(f"  desired: {format_claims(config)}")

    server_dir = os.path.join(MODEL_BASE_DIR, server)
    os.makedirs(server_dir, exist_ok=True)
    for name, entry in desired.items():
        storage_path = entry.get("storage_path") or f"{DEFAULT_BUCKET}/{name}/"
        model_dir = os.path.join(server_dir, name)
        if not has_valid_model_structure(model_dir):
            sync_model(storage_path, model_dir, endpoint_url)

    now = time.monotonic()
    for pod_ip, pod_name in pods:
        reconcile_pod(pod_ip, pod_name, desired_for_pod(config, pod_name), state, now)


def configure_aws() -> None:
    """Write AWS credentials and S3 endpoint into the awscli config."""
    endpoint = os.environ["AWS_ENDPOINT_URL"]
    print(f"configuring aws cli for endpoint {endpoint}")
    run(
        [
            "aws",
            "configure",
            "set",
            "aws_access_key_id",
            os.environ["AWS_ACCESS_KEY_ID"],
        ]
    )
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
    node_name = os.environ.get("NODE_NAME")
    if not node_name:
        print(
            "NODE_NAME is not set, so the daemon cannot tell which Triton pods share "
            "this node's model repository. Set it from spec.nodeName."
        )
        return 1

    configure_aws()
    os.makedirs(MODEL_BASE_DIR, exist_ok=True)

    servers = read_servers(INFERENCE_SERVERS_FILE)
    print(f"sync daemon on node {node_name}, servers: {servers}")

    endpoint_url = os.environ["AWS_ENDPOINT_URL"]
    while True:
        print("=" * 40)
        for server in servers:
            try:
                reconcile_server(server, endpoint_url, node_name)
            except Exception as e:
                print(f"  {server}: reconcile failed: {e}")
        print(f"sleeping {SYNC_INTERVAL_SECONDS}s")
        time.sleep(SYNC_INTERVAL_SECONDS)


if __name__ == "__main__":
    sys.exit(main())
