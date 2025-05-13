import os
import tempfile
import shutil
from typing import Optional
from michelangelo.lib.model_manager.constants import StorageType
from michelangelo.lib.model_manager.utils.terrablob_paths import (
    get_deployable_model_tar_path,
    get_raw_model_main_path,
)
from michelangelo._internal.gateways.terrablob_gateway import upload_to_terrablob
from michelangelo.lib.model_manager._private.utils.terrablob_utils import (
    get_terrablob_auth_mode,
    get_upload_multipart_options,
)
from michelangelo.lib.model_manager._private.utils.model_utils import (
    replace_model_name_placeholder,
    convert_download_yamls_to_deployable,
    validate_deployable_model_assets,
    convert_assets_to_download_yaml,
)


def upload_generic_deployable_model(
    model_path: str,
    project_name: str,
    model_name: str,
    model_revision: str,
    package_type: str,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
) -> str:
    """
    Upload a generic deployable model (non-spark) to Michelangelo.
    These models are meant to be served with OPS v2.

    Args:
        model_path (str): The path to the local dir with the packaged model to upload.
        project_name (str): The name of the project in MA Studio.
        model_name (str): The name of the model.
        model_revision (str): The revision of the model.
        package_type (str): The package type of the model, e.g (triton | raw).
        timeout (str, optional): The timeout for uploading the model.
            Defaults to None, which results to 1ns in Terrablob.
            Format example 100ms, 0.7s, 10m, 2h. If no unit is specified, milliseconds are used.
        source_entity (str, optional): The source entity for terrablob command when uploading the model.
            Defaults to None.

    Returns:
        The Terrablob path of the uploaded model.
    """
    tb_model_path = get_deployable_model_tar_path(
        project_name,
        model_name,
        model_revision,
        package_type,
    )

    # Replace the model name placeholder in the config.pbtxt file if the placeholder exists
    replace_model_name_placeholder(model_path, model_name)

    # Convert assets in the model to download.yaml
    source_prefix = f"{get_raw_model_main_path(project_name, model_name, model_revision)}/model/"
    convert_assets_to_download_yaml(model_path, package_type, source_type=StorageType.TERRABLOB, source_prefix=source_prefix)

    # Convert all download.yaml files in the model to deployable format
    convert_download_yamls_to_deployable(model_path, project_name, model_name, model_revision)

    # Validate all the remote assets in the model is downloadable
    validate_deployable_model_assets(model_path, timeout=timeout, source_entity=source_entity)

    with tempfile.TemporaryDirectory() as temp_dir:
        model_tar_path = shutil.make_archive(
            os.path.join(temp_dir, "model"),
            "tar",
            model_path,
        )

        upload_to_terrablob(
            model_tar_path,
            tb_model_path,
            **get_upload_multipart_options(),
            timeout=timeout,
            source_entity=source_entity,
            auth_mode=get_terrablob_auth_mode(),
        )

    return tb_model_path
