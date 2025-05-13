from typing import Optional


def get_raw_model_base_path(
    project_name: str,
    model_name: str,
) -> str:
    return f"/prod/michelangelo/raw_models/projects/{project_name}/models/{model_name}"


def get_raw_model_path(
    project_name: str,
    model_name: str,
    model_revision: Optional[str] = "0",
) -> str:
    path = get_raw_model_base_path(project_name, model_name)
    return f"{path}/revisions/{model_revision}"


def get_raw_model_main_path(
    project_name: str,
    model_name: str,
    model_revision: Optional[str] = "0",
) -> str:
    path = get_raw_model_path(project_name, model_name, model_revision)
    return f"{path}/main"
