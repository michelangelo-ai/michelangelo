"""Generate the deployable model package content for torch-based backends."""

from __future__ import annotations

import os
import tempfile
from typing import TYPE_CHECKING, Any

import yaml

from michelangelo.lib.model_manager._private.constants.triton_backend_type import (
    TritonBackendType,
)
from michelangelo.lib.model_manager._private.packager.common import (
    generate_model_py_content,
    serialize_model_class,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.config_pbtxt import (
    generate_config_pbtxt_content,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.constants import (
    DEPLOYABLE_CONFIG_FILE_NAME,
    DEPLOYABLE_MODEL_ONNX_FILE_NAME,
    DEPLOYABLE_MODEL_PY_FILE_NAME,
    DEPLOYABLE_SKELETON_FILE_NAME,
    DEPLOYABLE_USER_MODEL_PY_FILE_NAME,
    MODEL_CLASS_FILE_NAME,
    MODEL_PT_FILE_NAME,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.model_loader import (
    serialize_torch_python_loader,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.onnx_conversion import (  # noqa: E501
    convert_to_onnx,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.raw_model_package import (  # noqa: E501
    convert_to_state_dict,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.torchscript_conversion import (  # noqa: E501
    _convert_to_torchscript,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.user_model_py import (  # noqa: E501
    generate_torch_python_user_model_content,
)
from michelangelo.lib.model_manager._private.packager.torch_triton.validation import (
    validate_deployable_model_file,
    validate_raw_model_file,
)
from michelangelo.lib.model_manager._private.serde.data import dump_model_data
from michelangelo.lib.model_manager._private.utils.asset_utils import download_assets
from michelangelo.lib.model_manager._private.utils.spec_utils import (
    collect_nested_class_paths,
)
from michelangelo.lib.model_manager._private.utils.torch_utils import tensor_to_numpy
from michelangelo.lib.model_manager.constants import StorageType

if TYPE_CHECKING:
    from michelangelo.lib.model_manager._private.packager.template_renderer import (
        TritonTemplateRenderer,
    )
    from michelangelo.lib.model_manager.schema import ModelSchema


def _download_and_prepare_state_dict(
    model_path: str,
    model_version_dir: str,
    model_path_source_type: str | None,
) -> str:
    """Download a model and store it as a state_dict under the version dir.

    The model is downloaded, validated as a raw model, converted to state_dict
    format, and moved into the version directory's model/ subdirectory.

    Args:
        model_path: Source path of the model artifact.
        model_version_dir: The Triton model version directory (e.g. ".../0").
        model_path_source_type: The source type of the model path.

    Returns:
        Path to the final state_dict model file.

    Raises:
        ValueError: If the downloaded artifact is not a valid raw model.
        RuntimeError: If the downloaded artifact is not a valid raw model.
        FileNotFoundError: If the model file does not exist.
    """
    target_model_path = os.path.join(model_version_dir, MODEL_PT_FILE_NAME)
    download_assets(model_path, target_model_path, model_path_source_type)

    is_valid, error = validate_raw_model_file(target_model_path)
    if not is_valid:
        raise error

    convert_to_state_dict(target_model_path)

    model_subdir = os.path.join(model_version_dir, "model")
    os.makedirs(model_subdir, exist_ok=True)
    final_model_path = os.path.join(model_subdir, MODEL_PT_FILE_NAME)
    os.replace(target_model_path, final_model_path)

    return final_model_path


def _serialize_model_definition(
    model_class: str,
    model_version_dir: str,
    hyperparameters: dict | None,
    include_import_prefixes: list[str] | None,
    model_class_file_name: str = MODEL_CLASS_FILE_NAME,
    skeleton_file_name: str = DEPLOYABLE_SKELETON_FILE_NAME,
) -> dict:
    """Serialize the model class and a skeleton spec for serve-time rebuild.

    Saves the model class source and its import dependencies, bundles the source
    of any classes referenced by ``_target_`` import paths in the
    hyperparameters, and writes a skeleton spec used to reconstruct the model.

    The skeleton spec is a mapping whose ``_target_`` field names the class to
    instantiate, with the remaining keys passed as constructor kwargs. If the
    caller already supplied a ``_target_`` it is used as-is; otherwise the flat
    hyperparameters are wrapped with the model class as the target.

    Args:
        model_class: The model class import path.
        model_version_dir: The Triton model version directory.
        hyperparameters: Constructor kwargs, optionally with a ``_target_``.
        include_import_prefixes: When set, only class paths with one of these
            prefixes are serialized.
        model_class_file_name: Name of the text file holding the class path.
        skeleton_file_name: Name of the skeleton spec file.

    Returns:
        Content entries for the model class file and skeleton spec.
    """
    serialize_model_class(
        model_class,
        model_version_dir,
        model_class_file_name,
        include_import_prefixes=include_import_prefixes,
        serialize_interface=False,
    )

    for nested_class in collect_nested_class_paths(hyperparameters or {}):
        if nested_class == model_class:
            continue
        if include_import_prefixes and not nested_class.startswith(
            tuple(include_import_prefixes)
        ):
            continue
        serialize_model_class(
            nested_class,
            model_version_dir,
            model_file_name="",
            include_import_prefixes=include_import_prefixes,
            serialize_interface=False,
            write_txt_file=False,
        )

    content = {
        model_class_file_name: (
            f"file://{os.path.join(model_version_dir, model_class_file_name)}"
        ),
    }

    skeleton = (
        hyperparameters
        if (hyperparameters and "_target_" in hyperparameters)
        else {"_target_": model_class, **(hyperparameters or {})}
    )
    skeleton_path = os.path.join(model_version_dir, skeleton_file_name)
    with open(skeleton_path, "w") as f:
        yaml.safe_dump(
            skeleton,
            f,
            default_flow_style=False,
            allow_unicode=True,
            sort_keys=False,
        )
    content[skeleton_file_name] = f"file://{skeleton_path}"

    return content


def _generate_python_backend_wrappers(
    gen: TritonTemplateRenderer,
    model_version_dir: str,
    model_schema: ModelSchema,
) -> dict:
    """Generate the Triton python backend wrapper files.

    Creates model.py (the TritonPythonModel entry point) and user_model.py (a
    torch forward() wrapper that converts between numpy and tensors).

    Args:
        gen: The TritonTemplateRenderer instance.
        model_version_dir: The Triton model version directory.
        model_schema: Schema providing output names for the wrapper.

    Returns:
        Content entries for model.py and user_model.py.
    """
    model_py_content = generate_model_py_content(gen)
    model_py_path = os.path.join(model_version_dir, DEPLOYABLE_MODEL_PY_FILE_NAME)
    with open(model_py_path, "w") as f:
        f.write(model_py_content)

    output_names = [item.name for item in model_schema.output_schema]
    user_model_py_content = generate_torch_python_user_model_content(gen, output_names)
    user_model_py_path = os.path.join(
        model_version_dir, DEPLOYABLE_USER_MODEL_PY_FILE_NAME
    )
    with open(user_model_py_path, "w") as f:
        f.write(user_model_py_content)

    return {
        DEPLOYABLE_MODEL_PY_FILE_NAME: model_py_content,
        DEPLOYABLE_USER_MODEL_PY_FILE_NAME: user_model_py_content,
    }


def _build_python_backend(
    gen: TritonTemplateRenderer,
    content: dict,
    model_path: str,
    model_version_dir: str,
    model_schema: ModelSchema,
    model_path_source_type: str | None,
    model_class: str | None,
    hyperparameters: dict | None,
    include_import_prefixes: list[str] | None,
) -> None:
    """Populate package content for the python backend.

    Args:
        gen: The TritonTemplateRenderer instance.
        content: The package content dict to update in place.
        model_path: Source path of the model artifact.
        model_version_dir: The Triton model version directory.
        model_schema: Schema for the python backend wrappers.
        model_path_source_type: The source type of the model path.
        model_class: The model class import path.
        hyperparameters: Constructor kwargs for the model class.
        include_import_prefixes: Import prefixes to serialize.
    """
    if not model_class:
        raise ValueError("model_class is required when using the 'python' backend")
    final_model_path = _download_and_prepare_state_dict(
        model_path, model_version_dir, model_path_source_type
    )
    content["0"].update(
        _serialize_model_definition(
            model_class,
            model_version_dir,
            hyperparameters,
            include_import_prefixes,
        )
    )
    content["0"].update(
        _generate_python_backend_wrappers(gen, model_version_dir, model_schema)
    )
    serialize_torch_python_loader(model_version_dir, include_import_prefixes)
    content["0"]["model"] = {MODEL_PT_FILE_NAME: f"file://{final_model_path}"}


def _build_onnx_backend(
    content: dict,
    model_path: str,
    model_version_dir: str,
    model_schema: ModelSchema,
    model_path_source_type: str | None,
    model_class: str | None,
    hyperparameters: dict | None,
    enable_dynamic_batching: bool,
    sample_data: list[dict[str, Any]] | None,
) -> None:
    """Populate package content for the onnxruntime backend.

    Args:
        content: The package content dict to update in place.
        model_path: Source path of the model artifact (.onnx or PyTorch).
        model_version_dir: The Triton model version directory.
        model_schema: Schema providing input and output names for export.
        model_path_source_type: The source type of the model path.
        model_class: The model class import path for state_dict sources.
        hyperparameters: Constructor kwargs for state_dict sources.
        enable_dynamic_batching: Whether to mark axis 0 dynamic.
        sample_data: Input batches; the first is used for export tracing.
    """
    with tempfile.TemporaryDirectory() as staging_dir:
        staging_path = os.path.join(staging_dir, "model")
        download_assets(model_path, staging_path, model_path_source_type)

        final_onnx_path = os.path.join(
            model_version_dir, DEPLOYABLE_MODEL_ONNX_FILE_NAME
        )
        convert_to_onnx(
            staging_path,
            final_onnx_path,
            model_schema,
            sample_data=sample_data[0] if sample_data else None,
            model_class=model_class,
            hyperparameters=hyperparameters,
            enable_dynamic_batching=enable_dynamic_batching,
        )

    content["0"][DEPLOYABLE_MODEL_ONNX_FILE_NAME] = f"file://{final_onnx_path}"


def _build_torchscript_backend(
    content: dict,
    model_path: str,
    model_version_dir: str,
    model_path_source_type: str | None,
    model_class: str | None,
    hyperparameters: dict | None,
) -> None:
    """Populate package content for the default pytorch (torchscript) backend.

    Args:
        content: The package content dict to update in place.
        model_path: Source path of the model artifact.
        model_version_dir: The Triton model version directory.
        model_path_source_type: The source type of the model path.
        model_class: The model class import path for state_dict sources.
        hyperparameters: Constructor kwargs for state_dict sources.

    Raises:
        ValueError: If the downloaded artifact is not a valid deployable model.
        RuntimeError: If the downloaded artifact is not a valid deployable model.
        FileNotFoundError: If the model file does not exist.
    """
    target_model_path = os.path.join(model_version_dir, MODEL_PT_FILE_NAME)
    download_assets(model_path, target_model_path, model_path_source_type)

    is_valid, error = validate_deployable_model_file(target_model_path)
    if not is_valid:
        raise error

    _convert_to_torchscript(target_model_path, model_class, hyperparameters)
    content["0"][MODEL_PT_FILE_NAME] = f"file://{target_model_path}"


def generate_model_package_content(
    gen: TritonTemplateRenderer,
    model_path: str,
    model_name: str,
    model_revision: str,
    model_schema: ModelSchema,
    model_path_source_type: str | None = StorageType.LOCAL,
    root_path: str | None = None,
    enable_dynamic_batching: bool = True,
    model_class: str | None = None,
    hyperparameters: dict | None = None,
    backend: str | None = None,
    include_import_prefixes: list[str] | None = None,
    sample_data: list[dict[str, Any]] | None = None,
) -> dict:
    """Generate deployable model package content for torch-based backends.

    Supports the pytorch (torchscript), python, and onnxruntime backends.

    Args:
        gen: The TritonTemplateRenderer instance.
        model_path: For pytorch and python backends, the .pt artifact path
            (state_dict or full pickled nn.Module). For onnxruntime, a .onnx
            file or a PyTorch artifact (.pt/.pth) to export.
        model_name: The name of the model in the Triton model repository.
        model_revision: The revision of the model.
        model_schema: The schema for config.pbtxt, ONNX export, and python
            backend wrappers.
        model_path_source_type: The source type of the model path.
        root_path: Root path for temporary files; a temp dir is used if omitted.
        enable_dynamic_batching: If True, enables dynamic batching with
            max_batch_size=256; if False, disables it (required for List[str]
            models).
        model_class: The model class import path, required for state_dict
            artifacts on the python backend.
        hyperparameters: Constructor kwargs for the model class.
        backend: The Triton backend type; defaults to pytorch.
        include_import_prefixes: When set, only imported modules with one of
            these prefixes are serialized.
        sample_data: For the onnxruntime backend, a list of input batches (each
            mapping input names to arrays/tensors). The first batch is used for
            PyTorch-to-ONNX tracing; unused when the artifact is already .onnx.

    Returns:
        The deployable model package content dictionary.
    """
    if not root_path:
        root_path = tempfile.mkdtemp()

    if backend is None:
        backend = TritonBackendType.TORCH

    config_pbtxt = generate_config_pbtxt_content(
        gen,
        model_name,
        model_revision,
        model_schema,
        backend=backend,
        enable_dynamic_batching=enable_dynamic_batching,
    )

    model_version_dir = os.path.join(root_path, "0")
    os.makedirs(model_version_dir, exist_ok=True)

    content: dict = {DEPLOYABLE_CONFIG_FILE_NAME: config_pbtxt, "0": {}}

    if backend == TritonBackendType.PYTHON:
        _build_python_backend(
            gen,
            content,
            model_path,
            model_version_dir,
            model_schema,
            model_path_source_type,
            model_class,
            hyperparameters,
            include_import_prefixes,
        )
    elif backend == TritonBackendType.ONNX:
        _build_onnx_backend(
            content,
            model_path,
            model_version_dir,
            model_schema,
            model_path_source_type,
            model_class,
            hyperparameters,
            enable_dynamic_batching,
            sample_data,
        )
    else:
        _build_torchscript_backend(
            content,
            model_path,
            model_version_dir,
            model_path_source_type,
            model_class,
            hyperparameters,
        )

    if sample_data is not None:
        batched = [
            {k: tensor_to_numpy(v) for k, v in sample.items()} for sample in sample_data
        ]
        content.setdefault("metadata", {})["sample_data.json"] = dump_model_data(
            batched
        )

    return content
