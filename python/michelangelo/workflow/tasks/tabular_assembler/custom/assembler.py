"""Custom (Python-backend) tabular assembler.

Packages a raw trained model — one that implements the custom
``michelangelo.lib.model_manager.interface.custom_model.Model`` interface —
into a deployable Triton package and a self-contained raw package, using
``CustomTritonPackager``.
"""

from __future__ import annotations

import io
import os
import pickle
import shutil
import tempfile
import uuid
from typing import TYPE_CHECKING

from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.packager.custom_triton import CustomTritonPackager
from michelangelo.lib.shared.utils.model_fuser import fuse_model_schema
from michelangelo.lib.shared.utils.model_metadata import (
    build_e2e_sample_data,
    fuse_e2e_schema,
)
from michelangelo.workflow.tasks.tabular_assembler._private.data.fuse import (
    fuse_sample_data,
)
from michelangelo.workflow.tasks.tabular_assembler._private.model_class.resolve import (
    resolve_model_class,
)
from michelangelo.workflow.variables.metadata import (
    TRAINING_FRAMEWORK_CUSTOM,
    ModelMetadata,
)
from michelangelo.workflow.variables.types import AssembledModel, ModelArtifact

if TYPE_CHECKING:
    from michelangelo.lib.artifact_manager.storage_backend import StorageBackend
    from michelangelo.workflow.schema.assembler import TabularAssemblerConfig
    from michelangelo.workflow.variables.types import FeaturePackageArtifact

__all__ = ["custom_assembler"]


def custom_assembler(
    config: TabularAssemblerConfig,
    raw_model: ModelArtifact,
    native_transform_model: ModelArtifact | None = None,
    feature_package: FeaturePackageArtifact | None = None,
    *,
    storage_backend: StorageBackend,
) -> AssembledModel:
    """Package a custom model into deployable and raw model packages.

    ``CustomTritonPackager`` only understands locally-resident model
    artifacts, so ``raw_model.path`` (and, in the native-transform case,
    ``native_transform_model.path``) are downloaded via ``storage_backend``
    to a local temporary directory before packaging; the produced packages
    are then uploaded back through ``storage_backend`` and their returned
    URIs become the resulting ``ModelArtifact.path`` values.

    Args:
        config: The assembler configuration. ``config.custom`` carries
            path-specific options (``custom_batch_processing``,
            ``additional_import_prefixes``, ``include_import_prefixes``);
            ``config.model_class`` overrides the model class recorded in
            ``raw_model``'s metadata.
        raw_model: The trained model to package. ``raw_model.metadata.schema``
            and ``raw_model.metadata.sample_data`` are used unless a
            ``native_transform_model`` is supplied.
        native_transform_model: Optional native-transform model. When set,
            the packaged model's directory layout is
            ``combined_model/{predictor,native_transform}/`` and the servable
            schema and sample data are the fusion of the transform's and the
            predictor's.
        feature_package: Optional feature package preceding ``raw_model``.
            When set, its schema/sample data are fused into the deployable
            model's ``e2e_schema``/``e2e_sample_data`` (see
            ``lib.shared.utils.model_metadata``); the raw model is
            unaffected.
        storage_backend: Backend used to download the source artifact(s) and
            upload the produced packages. Required — the assembler task
            boundary is where storage access must be explicit, unlike the
            packagers themselves, which may run standalone.

    Returns:
        An ``AssembledModel`` with the deployable and raw packaged artifacts.

    Raises:
        ValueError: If ``model_class`` cannot be resolved, or if
            ``model_schema``/``sample_data`` fail packager validation.
    """
    custom_batch_processing = (
        config.custom.custom_batch_processing
        if config.custom and config.custom.custom_batch_processing is not None
        else False
    )
    additional_import_prefixes = (
        config.custom.additional_import_prefixes
        if config.custom and config.custom.additional_import_prefixes is not None
        else None
    )
    include_import_prefixes = (
        config.custom.include_import_prefixes
        if config.custom and config.custom.include_import_prefixes is not None
        else None
    )
    packager = CustomTritonPackager(custom_batch_processing=custom_batch_processing)
    model_class = resolve_model_class(
        config.model_class, raw_model.metadata.model_class
    )

    with tempfile.TemporaryDirectory() as temp_dir:
        model_package_dest = os.path.join(temp_dir, "model_package")
        raw_model_package_dest = os.path.join(temp_dir, "raw_model_package")

        if native_transform_model is not None:
            combined_model_root = os.path.join(temp_dir, "combined_model")
            predictor_dir = os.path.join(combined_model_root, "predictor")
            native_transform_dir = os.path.join(combined_model_root, "native_transform")
            os.makedirs(predictor_dir, exist_ok=True)
            os.makedirs(native_transform_dir, exist_ok=True)
            storage_backend.download(native_transform_model.path, native_transform_dir)
            storage_backend.download(raw_model.path, predictor_dir)
            package_model_path = combined_model_root
            model_schema = fuse_model_schema(
                native_transform_model.metadata.schema,
                raw_model.metadata.schema,
            )
            sample_data = fuse_sample_data(
                native_transform_model.metadata.sample_data,
                raw_model.metadata.sample_data,
                columns_to_keep=[item.name for item in model_schema.input_schema],
            )
        else:
            local_model_dir = os.path.join(temp_dir, "raw_model")
            storage_backend.download(raw_model.path, local_model_dir)
            package_model_path = local_model_dir
            model_schema = raw_model.metadata.schema
            sample_data = raw_model.metadata.sample_data

        model_package_path = packager.create_model_package(
            package_model_path,
            model_class=model_class,
            model_schema=model_schema,
            dest_model_path=model_package_dest,
            model_path_source_type=StorageType.LOCAL,
            additional_import_prefixes=additional_import_prefixes,
            include_import_prefixes=include_import_prefixes,
            sample_data=sample_data,
        )
        raw_model_package_path = packager.create_raw_model_package(
            package_model_path,
            model_class=model_class,
            model_schema=model_schema,
            sample_data=sample_data,
            dest_model_path=raw_model_package_dest,
            model_path_source_type=StorageType.LOCAL,
            additional_import_prefixes=additional_import_prefixes,
            include_import_prefixes=include_import_prefixes,
        )

        # The deployable package is archived into a single tar before upload
        # -- it's a self-contained serving bundle meant to be handed off as
        # one unit, unlike the raw model (uploaded as loose files, since
        # nothing needs to move it around as a single blob).
        deployable_tar_path = shutil.make_archive(
            os.path.join(temp_dir, "deployable_package"), "tar", model_package_path
        )

        upload_prefix = f"tabular_assembler/{uuid.uuid4().hex}"
        deployable_uri = storage_backend.upload(
            deployable_tar_path, f"{upload_prefix}/deployable"
        )
        raw_uri = storage_backend.upload(raw_model_package_path, f"{upload_prefix}/raw")

    e2e_schema = fuse_e2e_schema(
        feature_package.metadata.schema if feature_package else None,
        model_schema,
    )
    e2e_sample_data = build_e2e_sample_data(
        feature_package.metadata.sample_data if feature_package else None,
        sample_data,
        {item.name for item in e2e_schema.input_schema},
    )

    deployable_metadata = ModelMetadata(
        deployable=True,
        assembled=True,
        _schema=io.BytesIO(pickle.dumps(model_schema)),
        _sample_data=io.BytesIO(pickle.dumps(sample_data)),
        _e2e_schema=io.BytesIO(pickle.dumps(e2e_schema)),
        _e2e_sample_data=io.BytesIO(pickle.dumps(e2e_sample_data)),
    )
    raw_metadata = ModelMetadata(
        deployable=False,
        assembled=True,
        _schema=io.BytesIO(pickle.dumps(model_schema)),
        _sample_data=io.BytesIO(pickle.dumps(sample_data)),
        training_framework=TRAINING_FRAMEWORK_CUSTOM,
        model_class=model_class,
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
