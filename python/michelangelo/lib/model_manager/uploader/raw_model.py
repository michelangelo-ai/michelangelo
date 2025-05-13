from typing import Optional
from michelangelo._internal.gateways.terrablob_gateway import upload_to_terrablob
from michelangelo.lib.model_manager.utils.terrablob_paths import get_raw_model_main_path
from michelangelo.lib.model_manager._private.utils.model_utils import get_latest_model_revision_id
from michelangelo.lib.model_manager._private.utils.terrablob_utils import (
    get_terrablob_auth_mode,
    get_upload_multipart_options,
)


def upload_raw_model(
    model_path: str,
    project_name: str,
    model_name: str,
    timeout: Optional[str] = None,
    revision_id: Optional[int] = None,
    source_entity: Optional[str] = None,
) -> str:
    """
    Upload a raw model to Michelangelo.

    Args:
        model_path: The path to the raw model.
        project_name: The name of the project.
        model_name: The name of the model.
        timeout: The timeout for the upload.
        revision_id: The revision ID of the model.
        source_entity: The source entity for terrablob command when uploading the model.
            Defaults to None, which results to "michelangelo-apiserver".

    Returns:
        The Terrablob path to the raw model.
    """
    if revision_id is None:
        revision_id = get_latest_model_revision_id(
            project_name,
            model_name,
        )
        model_revision = f"{revision_id + 1 if revision_id >= 0 else 0}"
    else:
        model_revision = str(revision_id)

    tb_model_path = get_raw_model_main_path(project_name, model_name, model_revision)

    upload_to_terrablob(
        model_path,
        tb_model_path,
        **get_upload_multipart_options(),
        timeout=timeout,
        source_entity=source_entity or "michelangelo-apiserver",
        auth_mode=get_terrablob_auth_mode(),
    )

    return tb_model_path
