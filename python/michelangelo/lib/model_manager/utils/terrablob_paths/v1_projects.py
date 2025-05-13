def get_v1_projects_model_path(
    project_id: str,
    model_id: str,
) -> str:
    return f"/prod/michelangelo/v1_projects/{project_id}/trained_models/{model_id}"


def get_v1_projects_model_zip_path(
    project_id: str,
    model_id: str,
) -> str:
    return f"{get_v1_projects_model_path(project_id, model_id)}/sparkml_proto/{project_id}-v2.zip"
