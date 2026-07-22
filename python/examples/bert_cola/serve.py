"""Post-training serving pipeline for BERT CoLA.

All post-training steps (package → upload → register → deploy) run inside a
single RayTask so they execute on the compute cluster where the checkpoint
lives, and can reach the sandbox apiserver via MA_API_SERVER in the ConfigMap.
"""

from __future__ import annotations

import logging
import os
import shutil
import tempfile

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask

log = logging.getLogger(__name__)

_MODEL_CLASS = "examples.bert_cola.model.BertColaModel"
_MODEL_NAME = "bert-cola"


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="2Gi",
        worker_cpu=1,
        worker_memory="2Gi",
        worker_instances=0,
    ),
)
def upload_and_deploy(checkpoint_uri: str) -> str:
    """Package, upload, register and deploy the trained BERT model.

    Runs as a RayTask on its own ephemeral cluster. Downloads the checkpoint
    from shared storage (uploaded by the train step, which runs on a
    different cluster) rather than assuming a local path.
    Requires MA_API_SERVER in the pod's ConfigMap pointing to the apiserver.

    Steps:
        1. Download checkpoint from shared storage and load via BertColaModel.
        2. Package as Triton Python backend via CustomTritonPackager.
        3. Upload raw + deployable to MinIO via MinioStorageBackend.
        4. Register with model registry via APIRegistryClient → Model + Revision.
        5. Flat-upload the deployable dir to the revision-keyed prefix the
           deployment controller reads from (required for KServe backends).
        6. Create Deployment CRD → controller runs zonal rollout.

    Returns:
        deployment_name — name of the created Deployment CRD.
    """
    import os
    import shutil
    import tempfile

    from michelangelo.api.v2 import APIClient
    from michelangelo.gen.api import options_pb2
    from michelangelo.gen.api.v2 import deployment_pb2, deployment_svc_pb2
    from michelangelo.gen.api.v2.deployment_svc_pb2_grpc import DeploymentServiceStub
    from michelangelo.gen.k8s.io.apimachinery.pkg.apis.meta.v1 import generated_pb2 as meta_pb2
    from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend
    from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
    from michelangelo.lib.model_manager.registry.api_client import APIRegistryClient
    from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
    from examples.bert_cola.model import BertColaModel

    ma_api_server = os.environ.get("MA_API_SERVER", "localhost:15566")
    ma_namespace = os.environ.get("MA_NAMESPACE", "default")
    inference_server = os.environ.get("BERT_COLA_INFERENCE_SERVER", "inference-server-multi")

    endpoint = os.environ.get("AWS_ENDPOINT_URL", "http://minio:9091")
    endpoint_host = endpoint.replace("http://", "").replace("https://", "")
    secure = os.environ.get("MA_FILE_SYSTEM_S3_SCHEME", "http") == "https"

    storage = MinioStorageBackend(
        endpoint=endpoint_host,
        bucket="deploy-models",
        access_key=os.environ.get("AWS_ACCESS_KEY_ID", "minioadmin"),
        secret_key=os.environ.get("AWS_SECRET_ACCESS_KEY", "minioadmin"),
        secure=secure,
        create_bucket_if_missing=True,
    )

    # Step 1: Download checkpoint from shared storage and load.
    checkpoint_dir = tempfile.mkdtemp(prefix="bert_cola_checkpoint_")
    log.info("Downloading checkpoint from %s to %s", checkpoint_uri, checkpoint_dir)
    storage.download(checkpoint_uri, checkpoint_dir)
    model = BertColaModel.load(checkpoint_dir)

    raw_dir = tempfile.mkdtemp(prefix="bert_cola_raw_")
    deployable_dir = None
    try:
        model.save(raw_dir)

        # Step 2: Package as Triton Python backend.
        schema = ModelSchema(
            input_schema=[
                ModelSchemaItem(name="input_ids",      data_type=DataType.LONG,  shape=[128]),
                ModelSchemaItem(name="attention_mask",  data_type=DataType.LONG,  shape=[128]),
                ModelSchemaItem(name="token_type_ids",  data_type=DataType.LONG,  shape=[128]),
            ],
            output_schema=[
                ModelSchemaItem(name="logits", data_type=DataType.FLOAT, shape=[2]),
            ],
        )

        packager = CustomTritonPackager()
        deployable_dir = packager.create_model_package(
            model_path=raw_dir,
            model_class=_MODEL_CLASS,
            model_schema=schema,
            model_name=_MODEL_NAME,
            include_import_prefixes=["examples", "michelangelo"],
        )
        log.info("Triton package built at %s", deployable_dir)

        # Step 3: Upload to MinIO.
        raw_uri = storage.upload(raw_dir, f"{_MODEL_NAME}/raw")
        deployable_uri = storage.upload(deployable_dir, f"{_MODEL_NAME}/deployable")
        log.info("Uploaded raw=%s deployable=%s", raw_uri, deployable_uri)

        # Step 4: Register model → creates Model + Revision CRDs.
        client = APIClient(endpoint=ma_api_server, caller="bert-cola-pipeline")
        registry = APIRegistryClient(svc=client.ModelService, namespace=ma_namespace)

        registered = registry.register_model(
            name=_MODEL_NAME,
            artifact_uri=raw_uri,
            deployable_artifact_uri=deployable_uri,
            labels={
                "framework": "pytorch",
                "task": "text-classification",
                "dataset": "cola",
                "package_type": "TRITON_PYTHON",
            },
            metadata={"model_class": "bert-base-cased", "num_labels": "2"},
        )
        revision_name = registered.version
        log.info("Model registered: version=%s", revision_name)

        # Step 5: Flat-upload the deployable dir to the revision-keyed prefix
        # the deployment controller reads from. BatchRolloutActor derives
        # storageURI as s3://deploy-models/<revision_name>/ regardless of
        # backend (see batch_rollout_actor.go); KServe's storage-initializer
        # fetches storageUri as a plain directory with no untar step, so this
        # must be a flat layout, not the tarred deployable_uri above.
        kserve_uri = storage.upload_flat(deployable_dir, revision_name)
        log.info("Flat-uploaded deployable to %s for KServe/backend loading", kserve_uri)

        # Step 6: Create Deployment CRD → controller runs zonal rollout.
        deployment_name = f"{_MODEL_NAME}-deployment"
        deployment = deployment_pb2.Deployment(
            metadata=meta_pb2.ObjectMeta(name=deployment_name, namespace=ma_namespace),
            spec=deployment_pb2.DeploymentSpec(
                desired_revision=options_pb2.ResourceIdentifier(
                    name=revision_name, namespace=ma_namespace,
                ),
                inference_server=options_pb2.ResourceIdentifier(
                    name=inference_server, namespace=ma_namespace,
                ),
                definition=deployment_pb2.TargetDefinition(
                    type=deployment_pb2.TARGET_TYPE_INFERENCE_SERVER,
                ),
                strategy=deployment_pb2.DeploymentStrategy(
                    zonal=deployment_pb2.ZonalUpdate(rollout_period_in_seconds=300),
                ),
            ),
        )

        stub = DeploymentServiceStub(client._context.channel)
        req = deployment_svc_pb2.CreateDeploymentRequest(deployment=deployment)
        headers = client._context.header_provider.get_headers({})
        metadata = (*sorted(headers.items()), ("ttl", "600"))
        resp = stub.CreateDeployment(req, metadata=metadata, timeout=60)
        log.info("Deployment created: %s", resp.deployment.metadata.name)
        return resp.deployment.metadata.name

    finally:
        shutil.rmtree(checkpoint_dir, ignore_errors=True)
        shutil.rmtree(raw_dir, ignore_errors=True)
        if deployable_dir:
            shutil.rmtree(deployable_dir, ignore_errors=True)
