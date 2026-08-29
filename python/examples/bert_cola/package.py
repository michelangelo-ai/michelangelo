"""Packaging task for the BERT CoLA fine-tuning workflow.

Turns the locally-saved HuggingFace checkpoint from ``train()`` into a
deployable + raw Triton package via ``custom_assembler`` (the Python-backend
assembler path -- BERT's multi-input ``forward()`` isn't a good TorchScript
fit, so this doesn't use ``torch_assembler``).
"""

from __future__ import annotations

import io
import logging
import os
import pickle
import tempfile
from urllib.parse import urlparse

import numpy as np
import transformers

import michelangelo.uniflow.core as uniflow
from michelangelo.lib.model_manager.schema import DataType, ModelSchema, ModelSchemaItem
from michelangelo.uniflow.plugins.ray import RayTask
from michelangelo.workflow.schema.assembler import (
    CustomAssemblerConfig,
    TabularAssemblerConfig,
)
from michelangelo.workflow.tasks.tabular_assembler.custom.assembler import (
    custom_assembler,
)
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    ModelMetadata,
)
from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

log = logging.getLogger(__name__)

__all__ = ["package_model"]

_MODEL_CLASS = "examples.bert_cola.model.BertColaModel"
_TOKENIZER_NAME = "bert-base-cased"


def _resolve_storage_backend(tmp_prefix: str):
    """Select a storage backend based on environment configuration.

    ``AWS_ENDPOINT_URL`` set -> MinIO/S3-compatible remote storage; unset ->
    a local temp directory (development and CI).
    """
    s3_endpoint = os.environ.get("AWS_ENDPOINT_URL", "")
    if s3_endpoint:
        parsed = urlparse(s3_endpoint)
        endpoint = parsed.netloc
        if not endpoint:
            raise ValueError(
                f"AWS_ENDPOINT_URL={s3_endpoint!r} is missing a scheme. "
                "Use a full URL like http://minio:9091"
            )
        bucket = (
            os.environ.get("AWS_S3_BUCKET")
            or os.environ.get("MA_FILE_SYSTEM", "s3://default")
            .removeprefix("s3://")
            .split("/")[0]
        )
        from michelangelo.lib.artifact_manager.minio_backend import MinioStorageBackend

        storage_backend = MinioStorageBackend(
            endpoint=endpoint,
            bucket=bucket,
            access_key=os.environ.get("AWS_ACCESS_KEY_ID", ""),
            secret_key=os.environ.get("AWS_SECRET_ACCESS_KEY", ""),
            secure=parsed.scheme == "https",
            create_bucket_if_missing=True,
        )
        log.info(
            "package_model: using MinioStorageBackend (remote) -> %s",
            storage_backend.get_storage_location(),
        )
        return storage_backend

    from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend

    local_dir = tempfile.mkdtemp(prefix=tmp_prefix)
    storage_backend = LocalStorageBackend(local_dir)
    log.info("package_model: using LocalStorageBackend (local/CI) -> %s", local_dir)
    return storage_backend


def _schema(tokenizer_max_length: int) -> ModelSchema:
    token_fields = ["input_ids", "attention_mask", "token_type_ids"]
    return ModelSchema(
        input_schema=[
            ModelSchemaItem(
                name=name, data_type=DataType.LONG, shape=[tokenizer_max_length]
            )
            for name in token_fields
        ],
        output_schema=[
            ModelSchemaItem(name="logits", data_type=DataType.FLOAT, shape=[2]),
        ],
    )


def _sample_data(
    tokenizer: transformers.PreTrainedTokenizerBase, tokenizer_max_length: int
) -> list[dict[str, np.ndarray]]:
    encoded = tokenizer(
        "This sentence is grammatically acceptable.",
        max_length=tokenizer_max_length,
        truncation=True,
        padding="max_length",
        return_tensors="np",
    )
    return [
        {
            "input_ids": encoded["input_ids"][0].astype(np.int64),
            "attention_mask": encoded["attention_mask"][0].astype(np.int64),
            "token_type_ids": encoded["token_type_ids"][0].astype(np.int64),
        }
    ]


@uniflow.task(
    config=RayTask(head_cpu=1, head_memory="2Gi", worker_instances=0),
)
def package_model(
    train_output_dir: str,
    lr: float,
    eps: float,
    tokenizer_max_length: int = 128,
) -> AssembledModel:
    """Package the fine-tuned BERT checkpoint into deployable and raw Triton packages.

    ``CustomTritonPackager`` (invoked via ``custom_assembler``) only
    understands locally-resident model artifacts, and the raw checkpoint is
    already local (``train()`` writes it via ``trainer.save_model()``), so
    this uploads it once through this task's own storage backend to obtain a
    URI ``custom_assembler`` can download back from.

    Args:
        train_output_dir: Local directory ``train()`` saved the fine-tuned
            model and tokenizer to (via ``Trainer.save_model()``).
        lr: Learning rate used for training, recorded as a hyperparameter.
        eps: Adam epsilon used for training, recorded as a hyperparameter.
        tokenizer_max_length: Max sequence length used for tokenization --
            determines the packaged schema's input shape and must match what
            ``load_data()`` used.

    Returns:
        ``AssembledModel`` with the deployable and raw packaged artifacts.
    """
    storage_backend = _resolve_storage_backend("bert_cola_package_")

    tokenizer = transformers.AutoTokenizer.from_pretrained(_TOKENIZER_NAME)
    schema = _schema(tokenizer_max_length)
    sample_data = _sample_data(tokenizer, tokenizer_max_length)

    raw_model_uri = storage_backend.upload(train_output_dir, "raw_model")
    metadata = ModelMetadata(
        training_framework=TRAINING_FRAMEWORK_CUSTOM,
        model_class=_MODEL_CLASS,
        _schema=io.BytesIO(pickle.dumps(schema)),
        _sample_data=io.BytesIO(pickle.dumps(sample_data)),
    )
    metadata.hyperparameters = {"lr": lr, "eps": eps}
    raw_model = ModelArtifact(path=raw_model_uri, metadata=metadata)

    assembled = custom_assembler(
        TabularAssemblerConfig(
            custom=CustomAssemblerConfig(include_import_prefixes=["examples.bert_cola"])
        ),
        raw_model,
        storage_backend=storage_backend,
    )
    log.info(
        "package_model: deployable=%s raw=%s",
        assembled.deployable_model.path,
        assembled.raw_model.path,
    )
    return assembled
