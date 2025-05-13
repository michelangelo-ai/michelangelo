from uber.ai.michelangelo.sdk.model_manager.constants import PackageType
from uber.ai.michelangelo.sdk.model_manager.utils.terrablob_paths import (
    get_deployable_model_path,
    get_deployable_model_tar_path,
    get_v2_projects_model_jar_path,
)


def get_deployable_artifact_uri(project_name: str, model_name: str, model_revision: str, package_type: str) -> str:
    """
    Get the deployable artifact uri of the model.

    Args:
        project_name (str): The name of the project.
        model_name (str): The name of the model.
        model_revision (str): The revision of the model.
        package_type (str): The package type of the model, e.g (spark | triton | raw).

    Returns:
        The deployable artifact uri of the model.
    """
    if package_type == PackageType.SPARK:
        return get_v2_projects_model_jar_path(
            project_name,
            model_name,
            model_revision,
        )

    if package_type == PackageType.MOBILE:
        tb_model_path = get_deployable_model_path(
            project_name,
            model_name,
            model_revision,
            package_type,
        )
        return f"{tb_model_path}/deploy_gz/model.gz"

    return get_deployable_model_tar_path(
        project_name,
        model_name,
        model_revision,
        package_type,
    )
