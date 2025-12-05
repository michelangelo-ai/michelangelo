import os
import tempfile
from typing import Optional, Union
from numpy import ndarray
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.schema import ModelSchema
from michelangelo.lib.model_manager._private.utils.asset_utils import download_assets
from michelangelo.lib.model_manager._private.schema.common import schema_to_yaml
from michelangelo.lib.model_manager._private.serde.data import dump_model_data
from michelangelo.lib.model_manager._private.packager.custom_triton.model_class import serialize_model_class
from michelangelo.lib.model_manager._private.packager.custom_triton.type_yaml import generate_type_yaml
from michelangelo.lib.model_manager._private.packager.custom_triton.requirements_txt import generate_requirements_txt
from michelangelo.lib.model_manager._private.packager.custom_triton.pickled_model_binary import serialize_pickle_dependencies
from michelangelo.lib.model_manager._private.packager.custom_triton.constants import MODEL_CLASS_FILE_NAME
from michelangelo.lib.model_manager._private.packager.custom_triton.validation import validate_model_files


def generate_raw_model_package_content(
    model_path: str,
    model_class: str,
    model_schema: ModelSchema,
    sample_data: list[dict[str, ndarray]],
    model_path_source_type: Optional[str] = StorageType.LOCAL,
    requirements: Optional[Union[list[str], str]] = None,
    root_path: Optional[str] = None,
    include_import_prefixes: Optional[list[str]] = None,
    batch_inference: Optional[bool] = False,
) -> dict:
    """Generate the raw model package content.

    Args:
        model_path: the path of the raw model
        model_class: the model class of the model
            that contains the custom predict function
        model_schema: the schema of the model, which specifies the input/palette/output features
        sample_data: the sample data for the model. A list of input data for the predict function.
        model_path_source_type: the source type of the model path,
            e.g. 'hdfs', 'terrablob', default is 'hdfs'
        requirements: the requirements of the model, which can be one of the following:
            - a list of requirements
            - a path to the requirements.txt file
            If not specified, the requirements will not be included in the model package
        root_path (Optional): the root path for tmp files to be stored,
            if not specified, use a temp dir
        include_import_prefixes (Optional): only save the imported
            modules with the given prefixes in the model package,
            e.g. ['uber', 'data.michelangelo'] only imports starting
            with 'uber' or 'data.michelangelo' will be saved in the
            model package. Default is ['uber'],
            and if the list is empty, save all imports
        batch_inference (Optional): Specify if the prediction function in the model class handles batch inference.
            Default is False. If set to True, the model input/output will have an additional batch dimension on top of the existing model schema.
            For example, if the model schema specifies the input shape to be [n, m], the model expects the input shape to be [-1, n, m].
    Returns:
        The raw model package content
    """
    if not root_path:
        root_path = tempfile.mkdtemp()

    target_model_path = os.path.join(root_path, "model")

    os.makedirs(root_path, exist_ok=True)

    download_assets(
        model_path,
        target_model_path,
        model_path_source_type,
    )

    os.makedirs(target_model_path, exist_ok=True)

    # Validate that the model does not contain reserved files
    validate_model_files(target_model_path)

    defs_path = os.path.join(root_path, "defs")

    serialize_model_class(
        model_class,
        defs_path,
        MODEL_CLASS_FILE_NAME,
        include_import_prefixes=include_import_prefixes,
    )

    serialize_pickle_dependencies(
        target_model_path,
        defs_path,
        include_import_prefixes=include_import_prefixes,
    )

    content = {
        "metadata": {
            "type.yaml": generate_type_yaml(batch_inference),
            "schema.yaml": schema_to_yaml(model_schema),
            "sample_data.json": dump_model_data(sample_data),
        },
        "model": f"dir://{target_model_path}",
        "defs": f"dir://{defs_path}",
    }

    if requirements:
        content["dependencies"] = {
            "requirements.txt": generate_requirements_txt(requirements),
        }

    return content
