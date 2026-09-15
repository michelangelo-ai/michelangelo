"""Single-node adaptation of the BERT/CoLA training example."""

import importlib
import importlib.resources
import sys

import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.ray import UF_PLUGIN_RAY_USE_FSSPEC, RayTask


def _ensure_importlib_resources() -> None:
    """Use the Python 3.9 stdlib API when its backport is not installed."""
    try:
        importlib.import_module("importlib_resources")
    except ModuleNotFoundError:
        sys.modules["importlib_resources"] = importlib.resources


_ensure_importlib_resources()

assembler = importlib.import_module("examples.bert_cola.assembler").assembler
_load_data = importlib.import_module("examples.bert_cola.data").load_data
push_step = importlib.import_module("examples.bert_cola.push").push_step
_train = importlib.import_module("examples.bert_cola.train").train


@uniflow.workflow()
def train_workflow(path="nyu-mll/glue", name="cola", tokenizer_max_length=128):
    """Fine-tune and register BERT/CoLA without a separate Ray worker pod."""
    load_data = _load_data.with_overrides(
        alias="retrain_load_data",
        config=RayTask(head_cpu=1, head_memory="2Gi", worker_instances=0),
    )
    train = _train.with_overrides(
        alias="retrain_train",
        config=RayTask(head_cpu=1, head_memory="4Gi", worker_instances=0),
    )

    train_data, validation_data, test_data = load_data(
        path=path,
        name=name,
        tokenizer_max_length=tokenizer_max_length,
    )
    train_result, model_variable = train(
        train_data,
        validation_data,
        test_data,
    )
    print("train_result:", train_result)

    assembled = assembler(
        model_variable,
        lr=2e-5,
        eps=1e-8,
        tokenizer_max_length=tokenizer_max_length,
    )
    print("assembled model:", assembled)

    push_results = push_step(assembled)
    print("push results:", push_results)


if __name__ == "__main__":
    ctx = uniflow.create_context()
    ctx.environ["DATA_SIZE"] = "10"
    ctx.environ[UF_PLUGIN_RAY_USE_FSSPEC] = "0"
    ctx.environ["PYTORCH_MPS_HIGH_WATERMARK_RATIO"] = "0"
    ctx.environ["MA_NAMESPACE"] = "default"
    ctx.environ["IMAGE_PULL_POLICY"] = "IfNotPresent"
    ctx.environ["S3_ALLOW_BUCKET_CREATION"] = "True"
    ctx.run(train_workflow)
