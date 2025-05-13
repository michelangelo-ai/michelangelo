import os
import tempfile
from typing import Optional
from michelangelo.lib.model_manager.constants import PackageType
from michelangelo.lib.model_manager._private.utils.file_utils.gzip import gzip_compress
from michelangelo.lib.model_manager.utils.terrablob_paths import (
    get_deployable_model_path,
)
from michelangelo._internal.gateways.terrablob_gateway import upload_to_terrablob
from michelangelo.lib.model_manager._private.uploader.generic_deployable_model import upload_generic_deployable_model
from michelangelo.lib.model_manager._private.utils.terrablob_utils import get_terrablob_auth_mode


def upload_mobile_model(
    model_path: str,
    project_name: str,
    model_name: str,
    model_revision: str,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
) -> str:
    """
    Upload a mobile model to Michelangelo.
    These models are meant to be downloaded by mobile devices.

    Args:
        model_path (str): The path to the local dir with the packaged model to upload.
        project_name (str): The name of the project in MA Studio.
        model_name (str): The name of the model.
        model_revision (str): The revision of the model.
        timeout (str, optional): The timeout for uploading the model.
            Defaults to None, which results to 1ns in Terrablob.
            Format example 100ms, 0.7s, 10m, 2h. If no unit is specified, milliseconds are used.
        source_entity (str, optional): The source entity for terrablob command when uploading the model.
            Defaults to None.
    """
    files = os.listdir(model_path)

    if len(files) == 1 and os.path.isfile(
        os.path.join(model_path, files[0]),
    ):
        tb_model_path = get_deployable_model_path(
            project_name,
            model_name,
            model_revision,
            PackageType.MOBILE,
        )

        tb_model_gz_path = f"{tb_model_path}/deploy_gz/model.gz"

        with tempfile.TemporaryDirectory() as temp_dir:
            model_gz_path = os.path.join(temp_dir, "model.gz")

            gzip_compress(
                os.path.join(model_path, files[0]),
                model_gz_path,
            )

            upload_to_terrablob(
                model_gz_path,
                tb_model_gz_path,
                timeout=timeout,
                source_entity=source_entity,
                auth_mode=get_terrablob_auth_mode(),
            )

        return tb_model_gz_path
    else:
        return upload_generic_deployable_model(
            model_path,
            project_name,
            model_name,
            model_revision,
            PackageType.MOBILE,
            timeout=timeout,
            source_entity=source_entity,
        )
