import logging
from typing import Optional, Callable
from uber.ai.michelangelo.shared.errors.terrablob_error import TerrablobFileNotFoundError, TerrablobFailedPreconditionError
from uber.ai.michelangelo.shared.gateways.terrablob_gateway import list_terrablob_dir, path_exists
from michelangelo.lib.model_manager._private.utils.terrablob_utils import get_terrablob_auth_mode

_logger = logging.getLogger(__name__)


def get_latest_model_revision_id(
    project_name: str,
    model_name: str,
) -> int:
    """
    Get the latest model revision id for the model

    Args:
        project_name: the name of the project
        model_name: the name of the model

    Returns:
        The latest revision id. If the model is not found, return -1.
    """
    try:
        from michelangelo.lib.model_manager._private.utils.api_client import APIClient

        model_crd = APIClient.ModelService.get_model(project_name, model_name)
    except Exception:
        return -1
    else:
        return model_crd.spec.revision_id


def get_latest_uploaded_model_revision(
    project_name: str,
    model_name: str,
    get_model_path: Callable[[int], str],
    base_model_path: str,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
) -> str:
    """
    Get the latest model revision for all the uploaded models given the project and model name

    Args:
        project_name: the name of the project
        model_name: the name of the model
        get_model_path: a function that takes a revision id and returns the path of the deployable model binary
        base_model_path: the base path of the model, e.g. v2_projects_model_base_path, deployable_model_base_path
        timeout: the timeout for downloading the model
        source_entity: the source entity for terrablob command when downloading the model

    Returns:
        The latest uploaded revision. If the model is not found, return None
    """
    revision_id = get_latest_model_revision_id(project_name, model_name)
    auth_mode = get_terrablob_auth_mode()

    if revision_id >= 0 and path_exists(get_model_path(revision_id), timeout=timeout, source_entity=source_entity, auth_mode=auth_mode):
        return str(revision_id)

    try:
        revisions = list_terrablob_dir(
            base_model_path, output_relative_path=True, include_dir=True, timeout=timeout, source_entity=source_entity, auth_mode=auth_mode
        )
    except (TerrablobFileNotFoundError, TerrablobFailedPreconditionError):
        return None

    rev_ids = [int(rev) for rev in revisions if rev.isdigit()]

    if len(rev_ids) > 0:
        return str(max(rev_ids))

    if len(revisions) > 0:
        _logger.warning(f"Found revisions {revisions} in {base_model_path} but none of them are valid revision ids. Downloading revision {revisions[0]}.")
        return revisions[0]

    return None
