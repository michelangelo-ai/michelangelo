import tempfile
from typing import Optional
from michelangelo.lib.model_manager._private.downloader import download_generic_raw_model


def download_raw_model(
    project_name: str,
    model_name: str,
    model_revision: Optional[str] = None,
    dest_model_path: Optional[str] = None,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
) -> str:
    """
    Download a raw model from Michelangelo.

    Args:
        project_name (str): The name of the project.
        model_name (str): The name of the model.
        model_revision (str): The revision of the model.
        dest_model_path (str): The path to save the model files. If None, a temporary directory will be created.
        timeout (str, optional): The timeout for downloading the model. Defaults to None.
        source_entity (str, optional): The source entity for terrablob command when downloading the model. Defaults to None.

    Returns:
        str: The path to the downloaded model.
    """
    if dest_model_path is None:
        dest_model_path = tempfile.mkdtemp()

    download_generic_raw_model(
        project_name,
        model_name,
        model_revision,
        dest_model_path,
        timeout=timeout,
        source_entity=source_entity,
    )

    return dest_model_path
