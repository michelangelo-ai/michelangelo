from typing import Optional
import tempfile
from uber.ai.michelangelo.sdk.model_manager.constants import PackageType
from uber.ai.michelangelo.sdk.model_manager._private.downloader import (
    download_spark_pipeline_model,
    download_generic_deployable_model,
)


def download_deployable_model(
    project_name: str,
    model_name: str,
    model_revision: Optional[str] = None,
    dest_model_path: Optional[str] = None,
    package_type: Optional[str] = PackageType.SPARK,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
) -> str:
    """
    Download a deployable model from Michelangelo.

    Args:
        project_name (str): The name of the project.
        model_name (str): The name of the model.
        model_revision (str): The revision of the model.
        dest_model_path (str): The path to save the model files. If None, a temporary directory will be created.
        package_type (str, optional): The package type of the model, e.g (spark | triton | raw). Defaults to 'spark'.
        timeout (str, optional): The timeout for downloading the model. Defaults to None.
        source_entity (str, optional): The source entity for terrablob command when downloading the model. Defaults to None.

    Returns:
        str: The path to the downloaded model.
    """
    if dest_model_path is None:
        dest_model_path = tempfile.mkdtemp()

    if package_type == PackageType.SPARK:
        download_spark_pipeline_model(
            project_name,
            model_name,
            model_revision,
            dest_model_path,
            timeout,
            source_entity,
        )
    if package_type in {PackageType.TRITON, PackageType.RAW}:
        download_generic_deployable_model(
            project_name,
            model_name,
            model_revision,
            package_type,
            dest_model_path,
            timeout,
            source_entity,
        )

    return dest_model_path
