import yaml


def generate_project_yaml(project_name: str) -> str:
    """
    Generate the project.yaml file content

    Args:
        project_name: The name of the project in MA Studio

    Returns:
        The project.yaml file content
    """
    content = {
        "project": {"id": project_name},
    }
    return yaml.dump(content, sort_keys=False)
