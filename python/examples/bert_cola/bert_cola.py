"""BERT fine-tuning workflow for CoLA linguistic acceptability task.

Example workflow demonstrating BERT fine-tuning on the Corpus of Linguistic
Acceptability (CoLA) task from the GLUE benchmark.
Support workflow parameters via dict or Starlark-compatible parameters.
"""

import michelangelo.uniflow.core as uniflow
from examples.bert_cola.data import load_data
from examples.bert_cola.train import train
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC


@uniflow.workflow()
def train_workflow(path="glue", name="cola", tokenizer_max_length=128):
    """Training workflow for BERT model on GLUE datasets."""
    print("[train_workflow] Starting with config:")
    print("  - Dataset: " + path + "/" + name)
    print("  - Tokenizer max length: " + str(tokenizer_max_length))

    # Load data using configuration
    train_data, validation_data, test_data = load_data(
        path=path,
        name=name,
        tokenizer_max_length=tokenizer_max_length,
    )
    result = train(
        train_data,
        validation_data,
        test_data,
    )
    print("result:", result)
    print("ok.")


# For Local Run: python3 examples/bert_cola/bert_cola.py
# For Remote Run: python3 examples/bert_cola/bert_cola.py remote-run
# --storage-url <STORAGE_URL> --image <IMAGE>
if __name__ == "__main__":
    ctx = uniflow.create_context()

    # Set the environment variable DATA_SIZE to let the load_data task
    # know how much data to generate.
    ctx.environ["DATA_SIZE"] = "10"

    # Disable use of fsspec in Ray Plugin. See UF_PLUGIN_RAY_USE_FSSPEC
    # docstring for more information.
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ["PYTORCH_MPS_HIGH_WATERMARK_RATIO"] = "0"
    ctx.environ["MA_NAMESPACE"] = "default"
    # this is example docker image, we don't need to pull it from docker registry
    ctx.environ["IMAGE_PULL_POLICY"] = "Never"
    ctx.environ["S3_ALLOW_BUCKET_CREATION"] = "True"

    # Example 1: Run with default configuration (CoLA dataset)
    # please rebuild tar file if changed bert_cola.py for sandbox testing
    # ctx.run(train_workflow)

    # Example 2: Run with custom configuration (SST-2 dataset)
    ctx.run(train_workflow, path="glue", name="sst2", tokenizer_max_length=256)
