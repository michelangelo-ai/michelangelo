import os
import logging
import tempfile
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
import fsspec
import mlflow.pytorch
from transformers import AutoTokenizer
from michelangelo.gen.api.v2.model_pb2 import Model, ModelKind
from michelangelo.api.v2.client import APIClient

log = logging.getLogger(__name__)

CONFIG_PBTXT = """
name: "{model_name}"
platform: "pytorch_libtorch"
max_batch_size: 32
input [
  {{
    name: "input_ids"
    data_type: TYPE_INT64
    dims: [-1]
  }},
  {{
    name: "attention_mask"
    data_type: TYPE_INT64
    dims: [-1]
  }}
]
output [
  {{
    name: "output__0"
    data_type: TYPE_FP32
    dims: [2]
  }}
]
"""


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="4Gi",
        worker_instances=1,
        breakpoint=False,
    ),
    cache_enabled=True,
)
def pusher(model_uri: str, deployed_model_name: str):
    model = mlflow.pytorch.load_model(model_uri, map_location="cpu")
    model.eval()

    tokenizer = AutoTokenizer.from_pretrained("bert-base-cased")
    inputs = tokenizer("Example input for tracing.", return_tensors="pt")
    input_ids = inputs["input_ids"].to("cpu")
    attention_mask = inputs["attention_mask"].to("cpu")

    import torch
    import torch.nn as nn

    class TritonBertModel(nn.Module):
        def __init__(self, model):
            super().__init__()
            self.model = model

        def forward(self, input_ids, attention_mask):
            outputs = self.model(input_ids=input_ids, attention_mask=attention_mask)
            logits = outputs.logits
            return logits  # explicitly return a tensor, NOT a dict

    # Convert to TorchScript (Triton compatible)
    triton_model = TritonBertModel(model).eval()
    traced_model = torch.jit.trace(
        triton_model, (input_ids, attention_mask), strict=False
    )

    # Prepare local Triton-compatible directory structure using temporary directory
    with tempfile.TemporaryDirectory() as temp_dir:
        local_model_dir = os.path.join(temp_dir, deployed_model_name)
        version_dir = os.path.join(local_model_dir, "1")
        os.makedirs(version_dir, exist_ok=True)

        # Save traced model
        traced_model_path = os.path.join(version_dir, "model.pt")
        traced_model.save(traced_model_path)

        # Save config.pbtxt
        config_path = os.path.join(local_model_dir, "config.pbtxt")
        with open(config_path, "w") as config_file:
            config_file.write(CONFIG_PBTXT.format(model_name=deployed_model_name).strip())

        # Define deployment bucket and paths
        deploy_bucket = "deploy-models"

        # Standard Triton model repository structure: deploy-models/model_name/
        base_s3_path = f"s3://{deploy_bucket}/{deployed_model_name}"

        # Upload files to S3 using fsspec
        for local_file, s3_suffix in [
            (traced_model_path, "1/model.pt"),
            (config_path, "config.pbtxt"),
        ]:
            s3_uri = f"{base_s3_path}/{s3_suffix}"
            with fsspec.open(s3_uri, mode="wb") as s3_file:
                with open(local_file, "rb") as local_f:
                    s3_file.write(local_f.read())
            print(f"Uploaded {local_file} to {s3_uri}")

        print(f"Model files uploaded from {local_model_dir}")
        # No need to manually clean up - tempfile.TemporaryDirectory handles it automatically

    namespace = "default"
    name = deployed_model_name

    # Retrieve and verify the created model
    retrieved_model = None
    APIClient.set_caller("uniflow-client")
    try:
        retrieved_model = APIClient.ModelService.get_model(
            namespace=namespace, name=name
        )
        print("Retrieved created model:")
        print(retrieved_model)
    except Exception as e:
        print(f"Error retrieving model: {e}")

    if not retrieved_model:
        # Define model metadata
        model = Model()
        model.metadata.namespace = namespace
        model.metadata.name = name

        # Define model spec
        model.spec.owner.name = "default-user"
        model.spec.description = "Demo ML model creation"
        model.spec.kind = ModelKind.MODEL_KIND_BINARY_CLASSIFICATION
        model.spec.algorithm = "transformer model"
        model.spec.training_framework = "pytorch"
        model.spec.source = "Michelangelo V2"

        # Example package type and artifacts
        model.spec.deployable_artifact_uri.extend([deployed_model_name, model_uri])

        # Create the model
        try:
            response = APIClient.ModelService.create_model(model)
            print("Created model successfully:")
            print(response)
        except Exception as e:
            raise e
    else:
        retrieved_model.spec.ClearField("deployable_artifact_uri")
        retrieved_model.spec.deployable_artifact_uri.extend(
            [
                deployed_model_name,
                model_uri,
            ]
        )
        # Retrieve and verify the created model
        try:
            response = APIClient.ModelService.update_model(retrieved_model)
            print("Updated model:")
            print(response)
        except Exception as e:
            print(f"Error updating model: {e}")
            raise e

    return name


# Example usage:
# print(pusher("s3://mlflow/cf6b9898ed9e48168fc0280db114f8d1/artifacts/bert_model", "bert-cola-test"))
