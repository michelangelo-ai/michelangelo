from __future__ import annotations
from uber.ai.michelangelo.sdk.model_manager.constants import PackageType
from uber.ai.michelangelo.sdk.model_manager._private.utils.model_utils import (
    get_latest_model_revision_id,
    infer_model_package_type,
)
from uber.ai.michelangelo.sdk.model_manager._private.uploader import (
    upload_generic_deployable_model,
    upload_spark_model,
    upload_mobile_model,
)


def upload_deployable_model(
    model_path: str,
    project_name: str,
    model_name: str,
    package_type: str | None = None,
    timeout: str | None = None,
    revision_id: int | None = None,
    source_entity: str | None = None,
) -> str | None:
    """
    Upload a deployable model to Michelangelo.

    Args:
        model_path (str): The path to the local dir with model file to upload
        project_name (str): The name of the project.
        model_name (str): The name of the model.
        model_revision (str, optional): The revision of the model.
        package_type (str, optional): The package type of the model, e.g (spark | triton | raw).
            If not provided, the package type will be infered from the model package.
        timeout (str, optional): The timeout for uploading the model.
            Defaults to None, which results to 1ns in Terrablob.
            Format example 100ms, 0.7s, 10m, 2h. If no unit is specified, milliseconds are used.
        revision_id (int, optional): ID of the revision.
            If not provided, the the revision ID will be the latest revision ID + 1.
        source_entity (str, optional): The source entity for terrablob command when uploading the model.
            Defaults to None, which results to "michelangelo-apiserver".
    Returns:
        The Terrablob path of the uploaded model.
    """
    tb_model_path = None

    if revision_id is None:
        revision_id = get_latest_model_revision_id(
            project_name,
            model_name,
        )
        model_revision = f"{revision_id + 1 if revision_id >= 0 else 0}"
    else:
        model_revision = str(revision_id)

    if not package_type:
        package_type = infer_model_package_type(model_path)

    tb_source_entity = source_entity or "michelangelo-apiserver"

    if package_type == PackageType.SPARK:
        tb_model_path = upload_spark_model(
            model_path,
            project_name,
            model_name,
            model_revision,
            timeout=timeout,
            source_entity=tb_source_entity,
        )

    elif package_type in {PackageType.TRITON, PackageType.RAW}:
        tb_model_path = upload_generic_deployable_model(
            model_path,
            project_name,
            model_name,
            model_revision,
            package_type,
            timeout=timeout,
            source_entity=tb_source_entity,
        )
    elif package_type == PackageType.MOBILE:
        tb_model_path = upload_mobile_model(
            model_path,
            project_name,
            model_name,
            model_revision,
            timeout=timeout,
            source_entity=tb_source_entity,
        )

    return tb_model_path
