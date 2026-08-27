"""PyTorch/Lightning tabular assembler.

Packages a raw trained PyTorch or Lightning model into deployable and raw
Triton packages using ``TorchTritonPackager``. When preceded by a
native-transform stage, the predictor and transform models are fused into a
single servable artifact via the ``model_fuser`` package
(``lib.shared.utils.model_fuser.fuse``).
"""

from __future__ import annotations

import io
import os
import pickle
import shutil
import tempfile
import uuid
from typing import TYPE_CHECKING

from michelangelo.lib.model_manager.constants import StorageType, TritonBackendType
from michelangelo.lib.model_manager.packager.torch_triton import TorchTritonPackager
from michelangelo.lib.shared.utils.model_fuser import fuse as _fuse
from michelangelo.lib.shared.utils.model_fuser import fuse_model_schema
from michelangelo.lib.shared.utils.model_metadata import (
    build_e2e_sample_data,
    fuse_e2e_schema,
)
from michelangelo.workflow.tasks.tabular_assembler._private.schema import (
    reorder_output_schema,
)
from michelangelo.workflow.variables.metadata import ModelMetadata
from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.workflow.schema.assembler import TabularAssemblerConfig
    from michelangelo.workflow.variables.types import FeaturePackageArtifact

__all__ = ["torch_assembler"]


def torch_assembler(
    config: TabularAssemblerConfig,
    raw_model: ModelArtifact,
    native_transform_model: ModelArtifact | None = None,
    feature_package: FeaturePackageArtifact | None = None,
    *,
    storage_backend: StorageBackend,
) -> AssembledModel:
    """Package a PyTorch or Lightning model into deployable and raw packages.

    ``TorchTritonPackager`` only understands locally-resident model
    artifacts, so ``raw_model.path`` (and, when present,
    ``native_transform_model.path``) are downloaded via ``storage_backend``
    to a local temporary directory before packaging. When
    ``native_transform_model`` is supplied, the predictor and transform are
    fused (see ``lib.shared.utils.model_fuser``) into a single
    servable artifact with a combined schema; otherwise the predictor is
    packaged as-is.

    Args:
        config: The assembler configuration. ``config.torch.backend``
            selects the Triton backend for the deployable package (one of
            ``"pytorch"``, ``"python"``, ``"onnxruntime"``, or ``None`` for
            the packager default). ``config.torch
            .include_import_prefixes`` scopes the packager's dependency-file
            walk (see ``TorchAssemblerConfig.include_import_prefixes``).
        raw_model: The trained predictor model to package.
            ``raw_model.metadata.model_class``, ``.hyperparameters``,
            ``.schema``, and ``.sample_data`` describe how to load and
            package it.
        native_transform_model: Optional native-transform model preceding
            ``raw_model``. When set, the predictor and transform are fused
            into a single package with a combined input/output schema.
        feature_package: Optional feature package preceding ``raw_model``.
            When set, its schema/sample data are fused into the deployable
            model's ``e2e_schema``/``e2e_sample_data`` (see
            ``lib.shared.utils.model_metadata``); the raw model is
            unaffected.
        storage_backend: Backend used to download source artifacts and
            upload produced packages. Required, keyword-only — this task
            boundary is an explicit injection point, not a place to
            silently default to throwaway local storage.

    Returns:
        An ``AssembledModel`` with the deployable and raw packaged
        artifacts. Both share the same (possibly fused) schema.
    """
    packager = TorchTritonPackager()
    backend = config.torch.backend if config and config.torch else None
    include_import_prefixes = (
        config.torch.include_import_prefixes if config and config.torch else None
    )
    archive_deployable_package = (
        config.torch.archive_deployable_package if config and config.torch else False
    )
    model_class = raw_model.metadata.model_class
    hyperparameters = raw_model.metadata.hyperparameters or {}

    with tempfile.TemporaryDirectory() as temp_dir:
        torch_local_path = os.path.join(temp_dir, "torch_model.pt")
        storage_backend.download(raw_model.path, torch_local_path)

        if native_transform_model is not None:
            fuse_models_to_onnx = _fuse.fuse_models_to_onnx
            fuse_models_to_python = _fuse.fuse_models_to_python
            fuse_models_to_torchscript = _fuse.fuse_models_to_torchscript
            get_predictor_output_field_order = _fuse.get_predictor_output_field_order
            build_fused_sample_data = _fuse.build_fused_sample_data

            tx_model_class = native_transform_model.metadata.model_class
            tx_hyperparameters = native_transform_model.metadata.hyperparameters or {}
            tx_model_schema = native_transform_model.metadata.schema
            tx_local_path = os.path.join(temp_dir, "tx_model.pt")
            storage_backend.download(native_transform_model.path, tx_local_path)

            # Raw package: always fused (combined state dict + FusedModel),
            # regardless of backend.
            raw_model_path = os.path.join(temp_dir, "fused_model.pt")
            raw_model_path, packaged_model_class, packaged_hyperparameters = (
                fuse_models_to_python(
                    torch_model_path=torch_local_path,
                    tx_model_path=tx_local_path,
                    model_class=model_class,
                    hyperparameters=hyperparameters,
                    tx_hyperparameters=tx_hyperparameters,
                    dest_path=raw_model_path,
                    tx_model_schema=tx_model_schema,
                    model_schema=raw_model.metadata.schema,
                )
            )

            if backend == TritonBackendType.PYTHON:
                deployable_model_path = raw_model_path
            elif backend == TritonBackendType.ONNX:
                deployable_model_path = os.path.join(temp_dir, "fused_model.onnx")
                fuse_models_to_onnx(
                    torch_model_path=torch_local_path,
                    tx_model_path=tx_local_path,
                    model_class=model_class,
                    hyperparameters=hyperparameters,
                    tx_model_class=tx_model_class,
                    tx_hyperparameters=tx_hyperparameters,
                    dest_path=deployable_model_path,
                    tx_model_schema=tx_model_schema,
                    model_schema=raw_model.metadata.schema,
                )
            else:
                deployable_model_path = os.path.join(temp_dir, "fused_model.pt")
                fuse_models_to_torchscript(
                    torch_model_path=torch_local_path,
                    tx_model_path=tx_local_path,
                    model_class=model_class,
                    hyperparameters=hyperparameters,
                    tx_model_class=tx_model_class,
                    tx_hyperparameters=tx_hyperparameters,
                    dest_path=deployable_model_path,
                    tx_model_schema=tx_model_schema,
                    model_schema=raw_model.metadata.schema,
                )

            # Reorder output_schema to match the predictor's output order
            # (NamedTuple _fields for TorchScript, or the same order for
            # ONNX). Export strips _fields, so Triton inference uses
            # positional mapping from config.pbtxt; aligning the schema
            # prevents silent column swaps.
            field_order = get_predictor_output_field_order(
                torch_local_path,
                model_class,
                hyperparameters,
                raw_model.metadata.schema,
            )
            predictor_schema = reorder_output_schema(
                raw_model.metadata.schema, field_order
            )
            model_schema_for_package = fuse_model_schema(
                tx_model_schema, predictor_schema
            )
            fused_input_cols = {
                item.name for item in model_schema_for_package.input_schema
            }
            packaged_sample_data = build_fused_sample_data(
                native_transform_model.metadata.sample_data,
                raw_model.metadata.sample_data,
                fused_input_cols,
            )
        else:
            deployable_model_path = torch_local_path
            raw_model_path = torch_local_path
            model_schema_for_package = raw_model.metadata.schema
            packaged_model_class = model_class
            packaged_hyperparameters = hyperparameters
            packaged_sample_data = raw_model.metadata.sample_data

        model_package_dest = os.path.join(temp_dir, "model_package")
        raw_model_package_dest = os.path.join(temp_dir, "raw_model_package")

        model_package_path = packager.create_model_package(
            deployable_model_path,
            model_schema=model_schema_for_package,
            dest_model_path=model_package_dest,
            model_path_source_type=StorageType.LOCAL,
            model_class=packaged_model_class,
            hyperparameters=packaged_hyperparameters,
            backend=backend,
            sample_data=packaged_sample_data,
            include_import_prefixes=include_import_prefixes,
        )
        raw_model_package_path = packager.create_raw_model_package(
            raw_model_path,
            model_class=packaged_model_class,
            model_schema=model_schema_for_package,
            sample_data=packaged_sample_data,
            dest_model_path=raw_model_package_dest,
            model_path_source_type=StorageType.LOCAL,
            hyperparameters=packaged_hyperparameters,
            include_import_prefixes=include_import_prefixes,
            transform_spec=(
                native_transform_model.metadata.transform_spec
                if native_transform_model is not None
                else None
            ),
            transform_feature_stats=(
                native_transform_model.metadata.feature_stats
                if native_transform_model is not None
                else None
            ),
        )

        upload_prefix = f"tabular_assembler/{uuid.uuid4().hex}"
        if archive_deployable_package:
            # A single tar -- a self-contained serving bundle -- rather than
            # the default loose files, only when explicitly requested.
            deployable_tar_path = shutil.make_archive(
                os.path.join(temp_dir, "deployable_package"), "tar", model_package_path
            )
            deployable_uri = storage_backend.upload(
                deployable_tar_path, f"{upload_prefix}/deployable/model.tar"
            )
        else:
            deployable_uri = storage_backend.upload(
                model_package_path, f"{upload_prefix}/deployable"
            )
        raw_uri = storage_backend.upload(raw_model_package_path, f"{upload_prefix}/raw")

    e2e_schema = fuse_e2e_schema(
        feature_package.metadata.schema if feature_package else None,
        model_schema_for_package,
    )
    e2e_sample_data = build_e2e_sample_data(
        feature_package.metadata.sample_data if feature_package else None,
        packaged_sample_data,
        {item.name for item in e2e_schema.input_schema},
    )

    deployable_metadata = ModelMetadata(
        deployable=True,
        assembled=True,
        _schema=io.BytesIO(pickle.dumps(model_schema_for_package)),
        _sample_data=io.BytesIO(pickle.dumps(packaged_sample_data)),
        _e2e_schema=io.BytesIO(pickle.dumps(e2e_schema)),
        _e2e_sample_data=io.BytesIO(pickle.dumps(e2e_sample_data)),
    )
    raw_metadata = ModelMetadata(
        deployable=False,
        assembled=True,
        _schema=io.BytesIO(pickle.dumps(model_schema_for_package)),
        _sample_data=io.BytesIO(pickle.dumps(packaged_sample_data)),
        training_framework=raw_model.metadata.training_framework,
        model_class=packaged_model_class,
        _hyperparameters=io.BytesIO(pickle.dumps(packaged_hyperparameters)),
        is_incremental_training=raw_model.metadata.is_incremental_training,
        baseline_model_identifier=raw_model.metadata.baseline_model_identifier,
    )

    return AssembledModel(
        raw_model=ModelArtifact(path=raw_uri, metadata=raw_metadata),
        deployable_model=ModelArtifact(
            path=deployable_uri, metadata=deployable_metadata
        ),
        feature_package=feature_package,
    )
