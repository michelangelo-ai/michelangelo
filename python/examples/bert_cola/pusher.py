import os
import logging
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import RayTask
import fsspec
import torch
import mlflow.pytorch
from transformers import AutoTokenizer

log = logging.getLogger(__name__)

CONFIG_PBTXT = """
name: "bert_cola"
platform: "pytorch_libtorch"
max_batch_size: 32
input [
  {
    name: "input_ids"
    data_type: TYPE_INT64
    dims: [-1]
  },
  {
    name: "attention_mask"
    data_type: TYPE_INT64
    dims: [-1]
  }
]
output [
  {
    name: "output__0"
    data_type: TYPE_FP32
    dims: [2]
  }
]
"""

@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="4Gi",
        worker_instances=1,
    ),
)
def pusher(model_uri: str):
    model = mlflow.pytorch.load_model(model_uri, map_location="cpu")
    model.eval()

    tokenizer = AutoTokenizer.from_pretrained("bert-base-cased")
    inputs = tokenizer("Example input for tracing.", return_tensors="pt")
    input_ids = inputs["input_ids"].to("cpu")
    attention_mask = inputs["attention_mask"].to("cpu")

    import torch
    import torch.nn as nn
    from transformers import AutoModelForSequenceClassification

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
        triton_model,
        (input_ids, attention_mask),
        strict=False
    )

    # Prepare local Triton-compatible directory structure
    local_model_dir = "/tmp/bert_cola"
    version_dir = os.path.join(local_model_dir, "1")
    os.makedirs(version_dir, exist_ok=True)

    # Save traced model
    traced_model_path = os.path.join(version_dir, "model.pt")
    traced_model.save(traced_model_path)

    # Save config.pbtxt
    config_path = os.path.join(local_model_dir, "config.pbtxt")
    with open(config_path, "w") as config_file:
        config_file.write(CONFIG_PBTXT.strip())

    # Define deployment bucket and paths
    deploy_bucket = "deploy-models"
    # the first bert_cola is project name, the second one is the model name matched to the config.pbtxt
    base_s3_path = f"s3://{deploy_bucket}/bert_cola/bert_cola"

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

    # Clean up local files
    os.remove(traced_model_path)
    os.remove(config_path)
    os.rmdir(version_dir)
    os.rmdir(local_model_dir)

    return {
        "traced_model_s3_uri": f"{base_s3_path}/1/model.pt",
        "config_s3_uri": f"{base_s3_path}/config.pbtxt",
    }

# Example usage:
print(pusher("s3://mlflow/cf6b9898ed9e48168fc0280db114f8d1/artifacts/bert_model"))
