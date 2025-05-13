def get_v2_projects_model_base_path(
    project_name: str,
    model_name: str,
) -> str:
    return f"/prod/michelangelo/v2_projects/{project_name}/trained_models/{model_name}"


def get_v2_projects_model_path(
    project_name: str,
    model_name: str,
    model_revision: str,
) -> str:
    path = get_v2_projects_model_base_path(project_name, model_name)
    return f"{path}/{model_revision}"


def get_v2_projects_model_jar_path(
    project_name: str,
    model_name: str,
    model_revision: str,
) -> str:
    path = get_v2_projects_model_path(project_name, model_name, model_revision)
    return f"{path}/deploy_jar/model.jar.gz"
