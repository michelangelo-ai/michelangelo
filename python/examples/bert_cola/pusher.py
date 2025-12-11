"""Model pusher task for BERT CoLA model deployment."""

import logging
import os
import tempfile

import fsspec
import mlflow.pytorch
import numpy as np

import michelangelo.uniflow.core as uniflow
from michelangelo.api.v2.client import APIClient
from michelangelo.gen.api.v2.model_pb2 import Model, ModelKind
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)
from michelangelo.uniflow.plugins.ray import RayTask

log = logging.getLogger(__name__)


# Define the model schema for BERT CoLA
BERT_COLA_SCHEMA = ModelSchema(
    input_schema=[
        ModelSchemaItem(
            name="input_ids",
            data_type=DataType.LONG,  # 64-bit integer
            shape=[-1],  # Variable length sequence
        ),
        ModelSchemaItem(
            name="attention_mask",
            data_type=DataType.LONG,  # 64-bit integer
            shape=[-1],  # Variable length sequence
        ),
    ],
    output_schema=[
        ModelSchemaItem(
            name="logits",
            data_type=DataType.FLOAT,
            shape=[2],  # Binary classification logits
        ),
    ],
)


def upload_to_storage(local_dir: str, storage_path: str):
    """Upload a local directory to object storage.

    Args:
        local_dir: Local directory path to upload.
        storage_path: Destination storage path (s3:// or gs://).
    """
    for root, _, files in os.walk(local_dir):
        for file in files:
            local_file = os.path.join(root, file)
            relative_path = os.path.relpath(local_file, local_dir)
            remote_path = f"{storage_path}/{relative_path}"

            with (
                fsspec.open(remote_path, mode="wb") as remote_file,
                open(local_file, "rb") as local_f,
            ):
                remote_file.write(local_f.read())
            log.info(f"Uploaded {local_file} to {remote_path}")


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="4Gi",
        worker_instances=1,
        breakpoint=False,
    ),
    cache_enabled=False,
)
def pusher(model_uri: str, deployed_model_name: str):
    """Package and deploy a trained BERT model to Triton.

    Args:
        model_uri: URI of the trained model in MLflow (e.g., s3://mlflow/run_id/artifacts/model).
        deployed_model_name: Name for the deployed model in Triton.

    Returns:
        The deployed model name.
    """
    # Load the MLflow model and save to a local directory
    model = mlflow.pytorch.load_model(model_uri, map_location="cpu")
    model.eval()

    with tempfile.TemporaryDirectory() as temp_dir:
        # Save the raw model artifacts
        raw_model_path = os.path.join(temp_dir, "raw_model")
        os.makedirs(raw_model_path, exist_ok=True)

        # Save the model using torch
        import torch

        torch.save(model.state_dict(), os.path.join(raw_model_path, "model.pt"))

        # Save model config for loading
        model_config = {
            "model_name": "bert-base-cased",
            "num_labels": 2,
        }
        import json

        with open(os.path.join(raw_model_path, "config.json"), "w") as f:
            json.dump(model_config, f)

        # Create sample data for validation
        from transformers import AutoTokenizer

        tokenizer = AutoTokenizer.from_pretrained("bert-base-cased")
        sample_inputs = tokenizer(
            "Example sentence for testing.",
            return_tensors="np",
            padding="max_length",
            max_length=128,
            truncation=True,
        )

        sample_data = [
            {
                "input_ids": sample_inputs["input_ids"].squeeze().astype(np.int64),
                "attention_mask": sample_inputs["attention_mask"].squeeze().astype(
                    np.int64
                ),
            }
        ]

        # Package using CustomTritonPackager
        packager = CustomTritonPackager()
        packaged_model_path = packager.create_raw_model_package(
            model_path=raw_model_path,
            model_class="examples.bert_cola.bert_model.BertColaModel",
            model_schema=BERT_COLA_SCHEMA,
            sample_data=sample_data,
            dest_model_path=os.path.join(temp_dir, deployed_model_name),
            include_import_prefixes=["examples", "michelangelo"],
        )

        log.info(f"Model packaged to: {packaged_model_path}")

        # Upload to object storage
        deploy_bucket = "deploy-models"
        storage_path = f"s3://{deploy_bucket}/{deployed_model_name}"
        upload_to_storage(packaged_model_path, storage_path)

        log.info(f"Model uploaded to: {storage_path}")

    # Register model with API
    namespace = "default"
    name = deployed_model_name

    APIClient.set_caller("uniflow-client")

    try:
        retrieved_model = APIClient.ModelService.get_model(
            namespace=namespace, name=name
        )
        log.info(f"Retrieved existing model: {retrieved_model}")
    except Exception as e:
        log.info(f"Model not found, will create new: {e}")
        retrieved_model = None

    if not retrieved_model:
        # Create new model
        model_record = Model()
        model_record.metadata.namespace = namespace
        model_record.metadata.name = name
        model_record.spec.owner.name = "default-user"
        model_record.spec.description = "BERT CoLA model for linguistic acceptability"
        model_record.spec.kind = ModelKind.MODEL_KIND_BINARY_CLASSIFICATION
        model_record.spec.algorithm = "transformer"
        model_record.spec.training_framework = "pytorch"
        model_record.spec.source = "Michelangelo V2"
        model_record.spec.deployable_artifact_uri.extend(
            [deployed_model_name, model_uri]
        )

        try:
            response = APIClient.ModelService.create_model(model_record)
            log.info(f"Created model: {response}")
        except Exception as e:
            log.error(f"Error creating model: {e}")
            raise
    else:
        # Update existing model
        retrieved_model.spec.ClearField("deployable_artifact_uri")
        retrieved_model.spec.deployable_artifact_uri.extend(
            [deployed_model_name, model_uri]
        )

        try:
            response = APIClient.ModelService.update_model(retrieved_model)
            log.info(f"Updated model: {response}")
        except Exception as e:
            log.error(f"Error updating model: {e}")
            raise

    return name
