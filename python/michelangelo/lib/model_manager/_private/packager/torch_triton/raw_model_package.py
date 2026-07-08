"""Generate the raw model package content for PyTorch models."""

from __future__ import annotations

import os
import shutil
import tempfile
from typing import TYPE_CHECKING

import torch
import yaml

from michelangelo.lib.model_manager._private.packager.common import (
    generate_requirements_txt,
    serialize_model_class,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.constants import (
    MODEL_CLASS_FILE_NAME,
    MODEL_PT_FILE_NAME,
    RAW_HYPERPARAMETERS_FILE_NAME,
    RAW_REQUIREMENTS_FILE_NAME,
    RAW_SAMPLE_DATA_FILE_NAME,
    RAW_SCHEMA_FILE_NAME,
    RAW_TYPE_FILE_NAME,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.type_yaml import (
    generate_type_yaml,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.validation import (
    validate_raw_model_file,
)
from michelangelo.lib.model_manager._private.schema.common import schema_to_yaml
from michelangelo.lib.model_manager._private.serde.data import dump_model_data
from michelangelo.lib.model_manager._private.utils.asset_utils import download_assets
from michelangelo.lib.model_manager._private.utils.spec_utils import (
    collect_nested_class_paths,
)
from michelangelo.lib.model_manager._private.utils.torch_utils import is_state_dict
from michelangelo.lib.model_manager.constants import StorageType

if TYPE_CHECKING:
    from numpy import ndarray

    from michelangelo.lib.model_manager.schema import ModelSchema


def convert_to_state_dict(model_path: str) -> None:
    """Convert a model file to state_dict format in place.

    If the file already holds a state_dict it is left unchanged. Otherwise a
    full model is loaded and its state_dict is written back to model_path.

    Args:
        model_path: Path to the model file to convert in place.

    Raises:
        FileNotFoundError: If model_path does not exist.
        ValueError: If the file does not contain a convertible model.
    """
    if not os.path.exists(model_path):
        raise FileNotFoundError(f"File does not exist: {model_path}")

    try:
        state_dict = torch.load(model_path, map_location="cpu", weights_only=True)
        if is_state_dict(state_dict):
            return
    except Exception:
        pass

    try:
        obj = torch.load(
            model_path, map_location="cpu", weights_only=False
        )  # weights_only=False: need full nn.Module
    except Exception as e:
        raise ValueError(
            f"File does not contain a convertible model: {model_path}"
        ) from e

    if hasattr(obj, "state_dict"):
        torch.save(obj.state_dict(), model_path)
    else:
        raise ValueError(f"File does not contain a convertible model: {model_path}")


def _serialize_nested_classes(
    hyperparameters: dict | None,
    model_class: str,
    defs_path: str,
    include_import_prefixes: list[str] | None,
) -> None:
    """Serialize source for nested class paths referenced in hyperparameters.

    Hyperparameters may reference other classes by import path through a
    ``_target_`` field. Those classes are invisible to the static-import walk on
    the top-level model class, so their source is bundled here to make
    serve-time instantiation succeed.

    Args:
        hyperparameters: Hyperparameter mapping that may reference nested
            classes.
        model_class: The top-level model class import path, skipped if found.
        defs_path: Directory to serialize the nested class source into.
        include_import_prefixes: When set, only class paths with one of these
            prefixes are serialized.
    """
    for nested_class in collect_nested_class_paths(hyperparameters or {}):
        if nested_class == model_class:
            continue
        if include_import_prefixes and not nested_class.startswith(
            tuple(include_import_prefixes)
        ):
            continue
        serialize_model_class(
            nested_class,
            defs_path,
            model_file_name="",
            include_import_prefixes=include_import_prefixes,
            serialize_interface=False,
            write_txt_file=False,
        )


def generate_raw_model_package_content(
    model_path: str,
    model_class: str,
    model_schema: ModelSchema,
    sample_data: list[dict[str, ndarray]],
    model_path_source_type: str | None = StorageType.LOCAL,
    requirements: list[str] | str | None = None,
    root_path: str | None = None,
    include_import_prefixes: list[str] | None = None,
    hyperparameters: dict | None = None,
    transform_spec: dict | None = None,
    transform_feature_stats: dict | None = None,
) -> dict:
    """Generate the raw model package content for a PyTorch model.

    The model is downloaded, validated, and converted to a state_dict. The model
    class and any classes referenced by ``_target_`` import paths in the
    hyperparameters are serialized so the model can be reconstructed at serve
    time.

    Args:
        model_path: The PyTorch model path. May be a directory containing a
            single .pt file or a single .pt/.pth file.
        model_class: The model class import path.
        model_schema: The schema specifying input, palette, and output features.
        sample_data: A list of input records for the forward function.
        model_path_source_type: The source type of the model path.
        requirements: Model requirements as a list of specifiers or a path to a
            requirements.txt file. ``torch`` is always included.
        root_path: Root path for temporary files; a temp dir is used if omitted.
        include_import_prefixes: When set, only imported modules with one of
            these prefixes are serialized; otherwise all imports are serialized.
        hyperparameters: Constructor kwargs for the model class. May use a
            ``_target_`` import path to select the class to instantiate.
        transform_spec: Optional transform specification to bundle.
        transform_feature_stats: Optional transform feature statistics to bundle.

    Returns:
        The raw model package content dictionary.

    Raises:
        ValueError: If the model directory holds zero or multiple .pt files.
    """
    if not root_path:
        root_path = tempfile.mkdtemp()

    model_dir = os.path.join(root_path, "model")
    os.makedirs(model_dir, exist_ok=True)
    model_file_path = os.path.join(model_dir, MODEL_PT_FILE_NAME)

    with tempfile.TemporaryDirectory() as temp_dir:
        # model.pt may name either a single file or a directory.
        downloaded_model = os.path.join(temp_dir, "model")

        download_assets(
            model_path,
            downloaded_model,
            model_path_source_type,
        )

        if os.path.isdir(downloaded_model):
            pt_files = [f for f in os.listdir(downloaded_model) if f.endswith(".pt")]
            if not pt_files:
                raise ValueError(f"No .pt files found in {downloaded_model}")
            if len(pt_files) > 1:
                raise ValueError(
                    f"Multiple .pt files found in {downloaded_model}: {pt_files}"
                )
            shutil.move(os.path.join(downloaded_model, pt_files[0]), model_file_path)
        else:
            shutil.move(downloaded_model, model_file_path)

    is_valid, error = validate_raw_model_file(model_file_path)
    if not is_valid:
        raise error

    convert_to_state_dict(model_file_path)

    defs_path = os.path.join(root_path, "defs")

    serialize_model_class(
        model_class,
        defs_path,
        MODEL_CLASS_FILE_NAME,
        include_import_prefixes=include_import_prefixes,
        serialize_interface=False,
    )
    _serialize_nested_classes(
        hyperparameters, model_class, defs_path, include_import_prefixes
    )

    if requirements is None:
        requirements = ["torch"]
    elif isinstance(requirements, list) and "torch" not in requirements:
        requirements = [*requirements, "torch"]
    elif isinstance(requirements, str):
        # File-based requirements — append torch if not already present.
        with open(requirements) as f:
            content = f.read()
        if "torch" not in content.split():
            requirements = [
                line.strip() for line in content.splitlines() if line.strip()
            ] + ["torch"]

    metadata_content: dict = {
        RAW_TYPE_FILE_NAME: generate_type_yaml(),
        RAW_SCHEMA_FILE_NAME: schema_to_yaml(model_schema),
    }
    if sample_data is not None:
        metadata_content[RAW_SAMPLE_DATA_FILE_NAME] = dump_model_data(sample_data)

    if hyperparameters:
        skeleton = (
            hyperparameters
            if "_target_" in hyperparameters
            else {"_target_": model_class, **hyperparameters}
        )
        metadata_content[RAW_HYPERPARAMETERS_FILE_NAME] = yaml.safe_dump(skeleton)

    if transform_spec:
        metadata_content["transform_spec.yaml"] = yaml.safe_dump(transform_spec)
    if transform_feature_stats:
        metadata_content["transform_feature_stats.yaml"] = yaml.safe_dump(
            transform_feature_stats
        )

    return {
        "metadata": metadata_content,
        "model": f"dir://{model_dir}",
        "defs": f"dir://{defs_path}",
        "dependencies": {
            RAW_REQUIREMENTS_FILE_NAME: generate_requirements_txt(requirements),
        },
    }
