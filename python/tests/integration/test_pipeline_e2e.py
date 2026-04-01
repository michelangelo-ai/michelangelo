"""Level 3 end-to-end integration tests for Michelangelo pipeline execution.

Each test:
  1. Packages an existing example workflow into a uniflow tarball via build().
  2. Uploads the tarball to the sandbox MinIO bucket.
  3. Creates a Pipeline CRD via the gRPC API.
  4. Creates a PipelineRun CRD via the gRPC API.
  5. Polls get_pipeline_run() until a terminal state is reached.
  6. Asserts the run SUCCEEDED.

No new workflows are written – the tests exercise the existing examples:
  - examples/bert_cola/bert_cola.py   (bert_cola_workflow)
  - examples/boston_housing_xgb/boston_housing_xgb.py (train_workflow)

Run only these tests:
  pytest -m integration -v tests/integration/test_pipeline_e2e.py

Prerequisites:
  - The sandbox must be running (or MA_IT_CREATE_SANDBOX=true).
  - MA_IT_IMAGE must point to a Docker image already imported into k3d that
    has all example dependencies installed (built from examples/Dockerfile).
"""

import time
import uuid

import pytest

from michelangelo.gen.api.v2.pipeline_pb2 import Pipeline
from michelangelo.gen.api.v2.pipeline_run_pb2 import PipelineRun, PipelineRunState
from michelangelo.uniflow.core.build import build

from tests.integration.conftest import MINIO_BUCKET, TEST_IMAGE, TEST_NAMESPACE

# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

POLL_INTERVAL_SECONDS = 15
RUN_TIMEOUT_SECONDS = 1800  # 30 minutes – models can be slow to download

TERMINAL_STATES = frozenset(
    {
        PipelineRunState.PIPELINE_RUN_STATE_SUCCEEDED,
        PipelineRunState.PIPELINE_RUN_STATE_FAILED,
        PipelineRunState.PIPELINE_RUN_STATE_KILLED,
    }
)

# ---------------------------------------------------------------------------
# Helper
# ---------------------------------------------------------------------------


def _run_pipeline(
    api_client,
    s3_client,
    workflow_fn,
    pipeline_name: str,
    workflow_kwargs: dict,
) -> PipelineRunState:
    """Package, upload, submit, and poll a pipeline run. Return terminal state."""
    run_id = uuid.uuid4().hex[:8]
    full_pipeline_name = f"{pipeline_name}-{run_id}"
    run_name = f"run-{run_id}"
    tar_key = f"integration-test/{pipeline_name}/{run_id}.tar"

    # 1. Package the workflow into a gzipped tarball.
    package = build(workflow_fn)
    tar_bytes = package.to_tarball_bytes()

    # 2. Upload tarball to MinIO.
    s3_client.put_object(Bucket=MINIO_BUCKET, Key=tar_key, Body=tar_bytes)
    tar_url = f"s3://{MINIO_BUCKET}/{tar_key}"

    # 3. Create Pipeline CRD via the API.
    pipeline = Pipeline()
    pipeline.metadata.name = full_pipeline_name
    pipeline.metadata.namespace = TEST_NAMESPACE
    # The annotation tells the controllermgr which image to use for task pods.
    pipeline.metadata.annotations["michelangelo/uniflow-image"] = TEST_IMAGE
    pipeline.spec.type = Pipeline.PIPELINE_TYPE_TRAIN
    pipeline.spec.manifest.type = Pipeline.PipelineManifest.PIPELINE_MANIFEST_TYPE_UNIFLOW
    pipeline.spec.manifest.uniflow_tar = tar_url
    pipeline.spec.manifest.uniflow_function = workflow_fn.__name__
    pipeline.spec.owner.name = "integration-test"
    api_client.PipelineService.create_pipeline(pipeline)

    # 4. Create PipelineRun CRD.
    run = PipelineRun()
    run.metadata.name = run_name
    run.metadata.namespace = TEST_NAMESPACE
    run.spec.pipeline.name = full_pipeline_name
    run.spec.pipeline.namespace = TEST_NAMESPACE
    run.spec.actor.name = "integration-test"
    if workflow_kwargs:
        from google.protobuf.struct_pb2 import Struct

        s = Struct()
        s.update(workflow_kwargs)
        run.spec.input.CopyFrom(s)
    api_client.PipelineRunService.create_pipeline_run(run)

    # 5. Poll until terminal state or timeout.
    deadline = time.time() + RUN_TIMEOUT_SECONDS
    while time.time() < deadline:
        pr = api_client.PipelineRunService.get_pipeline_run(
            namespace=TEST_NAMESPACE, name=run_name
        )
        state = pr.status.state
        if state in TERMINAL_STATES:
            return state
        time.sleep(POLL_INTERVAL_SECONDS)

    raise TimeoutError(
        f"Pipeline run '{run_name}' did not reach a terminal state "
        f"within {RUN_TIMEOUT_SECONDS}s. Last state: {state}"
    )


# ---------------------------------------------------------------------------
# Tests
# ---------------------------------------------------------------------------


@pytest.mark.integration
@pytest.mark.slow
def test_bert_cola_pipeline_succeeds(api_client, s3_client):
    """bert_cola train_workflow runs end-to-end and completes successfully."""
    from examples.bert_cola.bert_cola import train_workflow

    state = _run_pipeline(
        api_client=api_client,
        s3_client=s3_client,
        workflow_fn=train_workflow,
        pipeline_name="it-bert-cola",
        # Small dataset slice to keep CI fast.
        workflow_kwargs={
            "path": "nyu-mll/glue",
            "name": "cola",
            "tokenizer_max_length": 64,
        },
    )
    assert state == PipelineRunState.PIPELINE_RUN_STATE_SUCCEEDED, (
        f"Expected SUCCEEDED but got {PipelineRunState.Name(state)}"
    )


@pytest.mark.integration
@pytest.mark.slow
def test_boston_housing_pipeline_succeeds(api_client, s3_client):
    """boston_housing_xgb train_workflow runs end-to-end and completes successfully."""
    from examples.boston_housing_xgb.boston_housing_xgb import train_workflow

    state = _run_pipeline(
        api_client=api_client,
        s3_client=s3_client,
        workflow_fn=train_workflow,
        pipeline_name="it-boston-housing",
        workflow_kwargs={
            "dataset_cols": (
                "CRIM,ZN,INDUS,CHAS,NOX,RM,AGE,DIS,RAD,TAX,PTRATIO,B,LSTAT,target"
            )
        },
    )
    assert state == PipelineRunState.PIPELINE_RUN_STATE_SUCCEEDED, (
        f"Expected SUCCEEDED but got {PipelineRunState.Name(state)}"
    )
