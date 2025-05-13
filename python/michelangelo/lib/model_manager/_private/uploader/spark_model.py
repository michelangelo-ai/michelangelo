from typing import Optional
from michelangelo.lib.model_manager.utils.terrablob_paths import (
    get_v2_projects_model_path,
    get_v2_projects_model_jar_path,
)
from michelangelo._internal.gateways.terrablob_gateway import upload_to_terrablob
from michelangelo.lib.model_manager._private.utils.terrablob_utils import get_upload_multipart_options


def upload_spark_model(
    model_path: str,
    project_name: str,
    model_name: str,
    model_revision: str,
    timeout: Optional[str] = None,
    source_entity: Optional[str] = None,
) -> str:
    """
    Upload a Spark model to Michelangelo.
    These models are in the legacy MA model format,
    and only meant to be served with OPS v1.

    Args:
        model_path (str): The path to the local dir with
            the spark model package (produced by SparkModelPackager) to upload
        project_name (str): The name of the project.
        model_name (str): The name of the model.
        model_revision (str): The revision of the model.
        timeout (str, optional): The timeout for uploading the model.
            Defaults to None, which results to 1ns in Terrablob.
            Format example 100ms, 0.7s, 10m, 2h. If no unit is specified, milliseconds are used.
        source_entity (str, optional): The source entity for terrablob command when uploading the model.
            Defaults to None.

    Returns:
        The Terrablob path of the uploaded model (deployable URI).
    """
    tb_model_path = get_v2_projects_model_path(
        project_name,
        model_name,
        model_revision,
    )

    upload_to_terrablob(
        model_path,
        tb_model_path,
        use_kraken=True,
        use_threads=False,
        **get_upload_multipart_options(),
        timeout=timeout,
        source_entity=source_entity,
    )

    return get_v2_projects_model_jar_path(project_name, model_name, model_revision)
