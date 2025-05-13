def get_deployable_model_base_path(
    project_name: str,
    model_name: str,
) -> str:
    return f"/prod/michelangelo/deployable_models/projects/{project_name}/models/{model_name}"


def get_deployable_model_path(
    project_name: str,
    model_name: str,
    model_revision: str,
    package_type: str,
) -> str:
    path = get_deployable_model_base_path(project_name, model_name)
    return f"{path}/revisions/{model_revision}/package/{package_type}"


def get_deployable_model_tar_path(
    project_name: str,
    model_name: str,
    model_revision: str,
    package_type: str,
) -> str:
    path = get_deployable_model_path(
        project_name,
        model_name,
        model_revision,
        package_type,
    )
    return f"{path}/deploy_tar/model.tar"
