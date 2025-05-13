import os
import shutil
import tempfile
from typing import Optional
from michelangelo.lib.model_manager._private.utils.file_utils import cd
from michelangelo.lib.model_manager._private.utils.file_utils.gzip import gzip_decompress
from michelangelo.lib.model_manager.utils.terrablob_paths import get_v2_projects_model_jar_path
from uber.ai.michelangelo.shared.gateways.terrablob_gateway import download_from_terrablob
from michelangelo.lib.model_manager._private.utils.terrablob_utils import get_download_multipart_options


def download_v2_projects_model(
    project_name: str,
    model_name: str,
    model_revision: str,
    dest_model_path: str,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
):
    """
    Download a spark pipeline model from Michelangelo that is stored at the v2_projects folder.

    Args:
        project_name (str): The name of the project.
        model_name (str): The name of the model.
        model_revision (str): The revision of the model.
        dest_model_path (str): The path to save the model files.
        timeout (str, optional): The timeout for downloading the model. Defaults to None.
        source_entity (str, optional): The source entity for terrablob command when downloading the model. Defaults to None.

    Returns:
        None
    """
    tb_model_jar_path = get_v2_projects_model_jar_path(project_name, model_name, model_revision)
    with tempfile.TemporaryDirectory() as temp_dir:
        jar_local_path = os.path.join(temp_dir, "model.jar")
        jar_gz_local_path = os.path.join(temp_dir, "model.jar.gz")
        model_files_dir = os.path.join(temp_dir, "downloaded_model")
        download_from_terrablob(
            tb_model_jar_path,
            jar_gz_local_path,
            **get_download_multipart_options(),
            timeout=timeout,
            source_entity=source_entity,
        )
        gzip_decompress(jar_gz_local_path, jar_local_path)
        if not os.path.exists(model_files_dir):
            os.makedirs(model_files_dir)
        with cd(model_files_dir):
            os.system("jar xf ../model.jar")
        model_zip_path = os.path.join(model_files_dir, f"{project_name}.zip")
        shutil.unpack_archive(model_zip_path, dest_model_path, "zip")
