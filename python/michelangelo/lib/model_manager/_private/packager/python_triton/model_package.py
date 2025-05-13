import os
import tempfile
from typing import Optional
from uber.ai.michelangelo.sdk.model_manager.constants import StorageType
from uber.ai.michelangelo.sdk.model_manager._private.packager.template_renderer import TritonTemplateRenderer
from uber.ai.michelangelo.sdk.model_manager._private.packager.common import download_model
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.config_pbtxt import generate_config_pbtxt_content
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.model_py import generate_model_py_content
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.user_model_py import generate_user_model_content
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.model_class import serialize_model_class
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.pickled_model_binary import serialize_pickle_dependencies
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.model_loader import serialize_model_loader
from uber.ai.michelangelo.sdk.model_manager._private.packager.python_triton.constants import MODEL_CLASS_FILE_NAME


def generate_model_package_content(
    gen: TritonTemplateRenderer,
    model_path: str,
    model_name: str,
    model_revision: str,
    model_class: str,
    input_schema: dict,
    output_schema: dict,
    model_path_source_type: Optional[str] = StorageType.HDFS,
    root_path: Optional[str] = None,
    include_import_prefixes: "Optional[list[str]]" = None,
    custom_batch_processing: Optional[bool] = False,
) -> dict:
    """
    Generate the model package content

    Args:
        gen: The TritonTemplateRenderer instance
        model_path: the model dir path in Terrablob
        model_name: the name of model in MA Studio
        model_revision: the revision of the model
        model_class: the import path of the model class
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
        custom_batch_processing (Optional): If to inject batch processing code to automatically handle batch.
            Default is False. If set to True, the user is responsible for handling batch in the model class,
            and the model input/output will have an additional batch dimension on top of the existing model schema.
            For example, the schema shape [1] will be converted to [-1, 1].
    Returns:
        The model package content
    """
    if not root_path:
        root_path = tempfile.mkdtemp()

    model_py = generate_model_py_content(gen)
    process_batch = False if custom_batch_processing else True
    user_model_py = generate_user_model_content(gen, process_batch=process_batch)
    config_pbtxt = generate_config_pbtxt_content(
        gen,
        model_name,
        model_revision,
        input_schema,
        output_schema,
    )

    model_0_dir = os.path.join(root_path, "0")
    os.makedirs(model_0_dir, exist_ok=True)

    serialize_model_class(
        model_class,
        model_0_dir,
        MODEL_CLASS_FILE_NAME,
        include_import_prefixes=include_import_prefixes,
    )

    target_model_path = os.path.join(model_0_dir, "model")

    download_model(
        model_path,
        target_model_path,
        model_path_source_type,
    )

    os.makedirs(target_model_path, exist_ok=True)

    serialize_pickle_dependencies(
        target_model_path,
        model_0_dir,
        include_import_prefixes=include_import_prefixes,
    )

    serialize_model_loader(model_0_dir)

    content = {
        "config.pbtxt": config_pbtxt,
        "0": {
            "model.py": model_py,
            "user_model.py": user_model_py,
            MODEL_CLASS_FILE_NAME: f"file://{os.path.join(model_0_dir, MODEL_CLASS_FILE_NAME)}",
            "model": f"dir://{target_model_path}",
        },
    }

    return content
