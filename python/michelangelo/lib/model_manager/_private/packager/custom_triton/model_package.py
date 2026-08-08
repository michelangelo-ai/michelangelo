"""Generate the model package content."""

import os
import tempfile
from typing import Optional

import numpy as np
from numpy import ndarray

from michelangelo.lib.model_manager._private.packager.custom_triton.additional_imports import (  # noqa: E501
    serialize_additional_imports,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.config_pbtxt import (  # noqa: E501
    generate_config_pbtxt_content,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.constants import (
    MODEL_CLASS_FILE_NAME,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.model_class import (
    serialize_model_class,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.model_loader import (  # noqa: E501
    serialize_model_loader,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.model_py import (
    generate_model_py_content,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.pickled_model_binary import (  # noqa: E501
    serialize_pickle_dependencies,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.user_model_py import (  # noqa: E501
    generate_user_model_content,
)
from michelangelo.lib.model_manager._private.packager.custom_triton.validation import (
    validate_model_files,
)
from michelangelo.lib.model_manager._private.packager.template_renderer import (
    TritonTemplateRenderer,
)
from michelangelo.lib.model_manager._private.schema.common import schema_to_yaml
from michelangelo.lib.model_manager._private.serde.data import dump_model_data
from michelangelo.lib.model_manager._private.utils.asset_utils import download_assets
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.schema import ModelSchema


def generate_model_package_content(
    gen: TritonTemplateRenderer,
    model_path: str,
    model_name: str,
    model_revision: str,
    model_class: str,
    model_schema: ModelSchema,
    input_schema: dict,
    output_schema: dict,
    model_path_source_type: Optional[str] = StorageType.LOCAL,
    root_path: Optional[str] = None,
    include_import_prefixes: Optional[list[str]] = None,
    additional_import_prefixes: Optional[list[str]] = None,
    custom_batch_processing: Optional[bool] = False,
    triton_parameters: Optional[dict[str, str]] = None,
    sample_data: Optional[list[dict[str, ndarray]]] = None,
) -> dict:
    """Generate the model package content.

    Args:
        gen: The TritonTemplateRenderer instance
        model_path: the model dir path in the source storage
        model_name: the name of model in MA Studio
        model_revision: the revision of the model
        model_class: the import path of the model class
        model_schema: The full schema, written to ``metadata/schema.yaml``
            for reference by downstream consumers of the package.
        input_schema: the input schema of the model
        output_schema: the output schema of the model
        model_path_source_type (Optional): the source type of the model path
        root_path (Optional): the root path for tmp files to be stored,
            if not specified, use a temp dir
        include_import_prefixes (Optional): only save the imported
            modules with the given prefixes in the model package,
            e.g. ['uber', 'data.michelangelo'] only imports starting
            with 'uber' or 'data.michelangelo' will be saved in the
            model package. If not specified, save all imports
        additional_import_prefixes (Optional): additional Python module
            prefixes whose source files should be included in the model
            package. Useful when the model class uses dynamic imports
            (e.g. importlib) that are not captured by static import
            analysis. Each prefix is recursively resolved and its files
            are serialized into the package.
        custom_batch_processing (Optional): If to inject batch processing
            code to automatically handle batch. Default is False. If set to
            True, the user is responsible for handling batch in the model
            class, and the model input/output will have an additional batch
            dimension on top of the existing model schema. For example, the
            schema shape [1] will be converted to [-1, 1].
        triton_parameters (Optional): custom Triton model config parameters
            to include in config.pbtxt. Each key-value pair becomes a
            ``parameters`` block in the generated config, e.g.
            ``{"MY_CUSTOM_PARAM": "16"}``.
        sample_data (Optional): sample inputs for the model's predict
            method, written to ``metadata/sample_data.json`` for reference
            and validation. Each item is a dict mapping input feature names
            to numpy arrays. If ``custom_batch_processing`` is False, a
            leading batch dimension is added to match what Triton passes to
            the model at serve time.

    Returns:
        The model package content as a dictionary
    """
    if not root_path:
        root_path = tempfile.mkdtemp()

    model_py = generate_model_py_content(gen)
    process_batch = not custom_batch_processing
    user_model_py = generate_user_model_content(gen, process_batch=process_batch)
    config_pbtxt = generate_config_pbtxt_content(
        gen,
        model_name,
        model_revision,
        input_schema,
        output_schema,
        triton_parameters=triton_parameters,
    )

    model_0_dir = os.path.join(root_path, "0")
    os.makedirs(model_0_dir, exist_ok=True)

    serialize_model_class(
        model_class,
        model_0_dir,
        MODEL_CLASS_FILE_NAME,
        include_import_prefixes=include_import_prefixes,
    )

    serialize_additional_imports(
        additional_import_prefixes,
        model_0_dir,
        include_import_prefixes=include_import_prefixes,
    )

    target_model_path = os.path.join(model_0_dir, "model")

    download_assets(
        model_path,
        target_model_path,
        model_path_source_type,
    )

    os.makedirs(target_model_path, exist_ok=True)

    # Validate that the custom model doesn't contain invalid filenames
    validate_model_files(target_model_path)

    serialize_pickle_dependencies(
        target_model_path,
        model_0_dir,
        include_import_prefixes=include_import_prefixes,
    )

    serialize_model_loader(model_0_dir)

    content = {
        "config.pbtxt": config_pbtxt,
        "metadata": {
            "schema.yaml": schema_to_yaml(model_schema),
        },
        "0": {
            "model.py": model_py,
            "user_model.py": user_model_py,
            MODEL_CLASS_FILE_NAME: (
                f"file://{os.path.join(model_0_dir, MODEL_CLASS_FILE_NAME)}"
            ),
            "model": f"dir://{target_model_path}",
        },
    }

    if sample_data is not None:
        if not custom_batch_processing:
            sample_data = [
                {k: np.expand_dims(v, axis=0) for k, v in sample.items()}
                for sample in sample_data
            ]
        content["metadata"]["sample_data.json"] = dump_model_data(sample_data)

    return content
