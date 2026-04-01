"""Pytest fixtures for Level 3 (full-sandbox) integration tests.

All configuration is driven by environment variables so the same tests run
locally (against an already-running sandbox) and in CI (where the sandbox is
created fresh by the workflow).

Environment variables (defaults in parentheses):
  MA_API_SERVER        – gRPC address of the sandbox API server (localhost:15566)
  MINIO_ENDPOINT       – MinIO HTTP endpoint (http://localhost:9091)
  MINIO_BUCKET         – S3 bucket for workflow tarballs (default)
  MINIO_ACCESS_KEY     – MinIO access key (minioadmin)
  MINIO_SECRET_KEY     – MinIO secret key (minioadmin)
  MA_IT_NAMESPACE      – k8s namespace used for test resources (ma-integration-test)
  MA_IT_IMAGE          – Docker image used for pipeline tasks (ma-it:latest)
  MA_IT_DOCKER_FILE    – Dockerfile for the task image (examples/Dockerfile)
  MA_IT_CREATE_SANDBOX – Set "true" to create/delete the k3d sandbox automatically
"""

import os
import subprocess
import time

import grpc
import pytest

from michelangelo.api.v2 import APIClient

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------

SANDBOX_CLUSTER = "michelangelo-sandbox"
API_SERVER = os.getenv("MA_API_SERVER", "localhost:15566")
MINIO_ENDPOINT = os.getenv("MINIO_ENDPOINT", "http://localhost:9091")
MINIO_BUCKET = os.getenv("MINIO_BUCKET", "default")
MINIO_ACCESS_KEY = os.getenv("MINIO_ACCESS_KEY", "minioadmin")
MINIO_SECRET_KEY = os.getenv("MINIO_SECRET_KEY", "minioadmin")
TEST_NAMESPACE = os.getenv("MA_IT_NAMESPACE", "ma-integration-test")
TEST_IMAGE = os.getenv("MA_IT_IMAGE", "ma-it:latest")
DOCKER_FILE = os.getenv("MA_IT_DOCKER_FILE", "examples/Dockerfile")
CREATE_SANDBOX = os.getenv("MA_IT_CREATE_SANDBOX", "false").lower() == "true"

# ---------------------------------------------------------------------------
# Internal helpers
# ---------------------------------------------------------------------------


def _cluster_exists(name: str) -> bool:
    return subprocess.run(
        ["k3d", "cluster", "get", name], capture_output=True
    ).returncode == 0


def _wait_for_api_server(address: str, timeout: int = 180) -> None:
    """Block until the gRPC server is reachable."""
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            channel = grpc.insecure_channel(address)
            grpc.channel_ready_future(channel).result(timeout=5)
            return
        except grpc.FutureTimeoutError:
            time.sleep(5)
    raise TimeoutError(f"API server {address} not ready after {timeout}s")


def _ensure_project(client) -> None:
    """Create the test project namespace (idempotent)."""
    from michelangelo.gen.api.v2.project_pb2 import Project

    try:
        client.ProjectService.get_project(
            namespace=TEST_NAMESPACE, name=TEST_NAMESPACE
        )
        return
    except Exception:
        pass

    project = Project()
    project.metadata.name = TEST_NAMESPACE
    project.metadata.namespace = TEST_NAMESPACE
    project.spec.owner.name = "integration-test"
    project.spec.description = "Automated integration test namespace"
    client.ProjectService.create_project(project)


# ---------------------------------------------------------------------------
# Session-scoped fixtures
# ---------------------------------------------------------------------------


@pytest.fixture(scope="session")
def sandbox():
    """Ensure the sandbox k3d cluster is running and the task image is loaded.

    Set MA_IT_CREATE_SANDBOX=true in CI to have the fixture create and tear
    down the cluster automatically.  For local development, start the sandbox
    manually with `mactl sandbox create` and leave this variable unset.
    """
    created = False

    if CREATE_SANDBOX:
        if not _cluster_exists(SANDBOX_CLUSTER):
            subprocess.run(
                [
                    "poetry", "run", "mactl", "sandbox", "create",
                    "--workflow", "cadence",
                ],
                check=True,
            )
            created = True
    else:
        if not _cluster_exists(SANDBOX_CLUSTER):
            pytest.skip(
                f"Sandbox cluster '{SANDBOX_CLUSTER}' is not running. "
                "Run `mactl sandbox create` or set MA_IT_CREATE_SANDBOX=true."
            )

    # Build the task Docker image from the examples Dockerfile and import it
    # into k3d so that IMAGE_PULL_POLICY=Never works inside the cluster.
    subprocess.run(
        ["docker", "build", "-t", TEST_IMAGE, "-f", DOCKER_FILE, "."],
        check=True,
    )
    subprocess.run(
        ["k3d", "image", "import", TEST_IMAGE, "--cluster", SANDBOX_CLUSTER],
        check=True,
    )

    _wait_for_api_server(API_SERVER)

    yield

    if created:
        subprocess.run(
            ["poetry", "run", "mactl", "sandbox", "delete"],
            check=False,  # best-effort cleanup
        )


@pytest.fixture(scope="session")
def api_client(sandbox):
    """Return the APIClient configured to talk to the sandbox."""
    os.environ["MA_API_SERVER"] = API_SERVER
    APIClient.set_caller("integration-test")
    _ensure_project(APIClient)
    return APIClient


@pytest.fixture(scope="session")
def s3_client(sandbox):
    """Return a boto3 client pointed at the sandbox MinIO."""
    import boto3

    return boto3.client(
        "s3",
        endpoint_url=MINIO_ENDPOINT,
        aws_access_key_id=MINIO_ACCESS_KEY,
        aws_secret_access_key=MINIO_SECRET_KEY,
        region_name="us-east-1",
    )
