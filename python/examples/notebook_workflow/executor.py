"""Working notebook executor implementation."""

import json
import os
from typing import Any, Optional

import michelangelo.uniflow.core as uniflow
import nbformat
from michelangelo.uniflow.plugins.ray import RayTask

from .dbutils import DBUtils


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="4Gi",
        worker_cpu=1,
        worker_memory="4Gi",
        worker_instances=1,
    )
)
def notebook_executor(
    notebook_path: str,
    parameters: Optional[dict[str, Any]] = None,
) -> tuple[Any, dict[str, Any]]:
    """Execute a Jupyter notebook with optional input parameters.

    Args:
        notebook_path: Path to the Jupyter notebook to execute.
        parameters: Optional dictionary of parameters to pass to the notebook.

    Returns:
        Tuple of (exit_value, task_values) from notebook execution.
    """
    # Resolve the full path to the notebook
    if not os.path.isabs(notebook_path):
        # If it's a relative path, make it relative to the current script directory
        script_dir = os.path.dirname(os.path.abspath(__file__))
        full_notebook_path = os.path.join(script_dir, os.path.basename(notebook_path))
    else:
        full_notebook_path = notebook_path

    # Load the notebook
    with open(full_notebook_path) as f:
        nb = nbformat.read(f, as_version=4)

    # Convert dict parameters to JSON strings for Databricks widget compatibility
    processed_params = {}
    if parameters:
        for key, value in parameters.items():
            if isinstance(value, dict):
                processed_params[key] = json.dumps(value)
            else:
                processed_params[key] = value

    # Create Databricks compatibility layer
    dbutils_instance = DBUtils(processed_params if processed_params else parameters)

    # Execute notebook cells directly
    # Add common imports and Databricks compatibility to execution environment
    globals_dict = {
        "__builtins__": __builtins__,
        "pd": __import__("pandas"),
        "np": __import__("numpy"),
        "plt": __import__("matplotlib.pyplot"),
        "json": __import__("json"),
        # Databricks compatibility - only way to get input/output
        "dbutils": dbutils_instance,
    }
    locals_dict = {}

    # Execute each code cell and track variables
    for cell in nb.cells:
        if cell.cell_type == "code":
            exec(cell.source, globals_dict, locals_dict)

    # Extract both exit_value and task_values and ensure JSON serializable
    exit_value = dbutils_instance.get_exit_value()
    task_values = dbutils_instance.get_task_values()

    return exit_value, task_values
