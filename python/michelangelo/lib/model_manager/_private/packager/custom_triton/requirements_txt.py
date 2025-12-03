from typing import Union


def generate_requirements_txt(requirements: Union[list[str], str]) -> str:
    """
    Generate the requirements.txt file content

    Args:
        requirements: The requirements can be one of the following:
            - A string representing the requirements.txt file path
            - A list of strings representing the requirements, e.g
              ["numpy==1.18.5", "pandas==1.0.5"]

    Returns:
        The requirements.txt file content
    """
    if isinstance(requirements, str):
        with open(requirements) as f:
            return f.read()

    if isinstance(requirements, list):
        return "\n".join([str(r) for r in requirements])

    raise ValueError(
        "requirements must be a list of requirements or "
        f"the requirements.txt file path, but got {type(requirements).__name__}"
    )
