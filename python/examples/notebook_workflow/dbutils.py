"""
Databricks DBUtils compatibility layer for Michelangelo notebook execution.

Provides essential data input/output functionality for Databricks notebook migration.
"""

from typing import Any, Dict, List, Optional, Union


class WidgetsAPI:
    """Databricks widgets API for input parameters."""

    def __init__(self, input_params: Optional[Dict[str, Any]] = None):
        self._params: Dict[str, Any] = input_params or {}

    def get(self, key: str, default_value: Optional[str] = None) -> str:
        """
        Get parameter value from widget.

        Args:
            key: Parameter name
            default_value: Default value if parameter not found

        Returns:
            Parameter value as string
        """
        return str(self._params.get(key, default_value or ""))

    def text(self, name: str, default_value: str = "", label: str = "") -> None:
        """
        Create a text widget.

        Args:
            name: Widget name
            default_value: Default value
            label: Widget label for display
        """
        if name not in self._params:
            self._params[name] = default_value

    def dropdown(self, name: str, default_value: str, choices: List[str], label: str = "") -> None:
        """Create a dropdown widget."""
        if name not in self._params:
            self._params[name] = default_value

    def multiselect(self, name: str, default_value: Union[str, List[str]], choices: List[str], label: str = "") -> None:
        """Create a multiselect widget."""
        if name not in self._params:
            self._params[name] = default_value

    def getAll(self) -> Dict[str, str]:
        """
        Get all widget parameters as string dictionary.

        Returns:
            Dictionary of all parameters converted to strings
        """
        return {k: str(v) for k, v in self._params.items()}


class TaskValuesAPI:
    """Databricks task values API for inter-task communication."""

    def __init__(self, dbutils_instance: 'DBUtils'):
        self._dbutils = dbutils_instance

    def set(self, key: str, value: Any) -> None:
        """
        Set a task value.

        Args:
            key: Value key
            value: Value to store (must be JSON serializable)
        """
        # Convert to JSON-serializable format
        if hasattr(value, 'dtype'):  # numpy types
            serializable_value = value.item()
        elif hasattr(value, 'to_dict'):  # pandas objects
            serializable_value = value.to_dict()
        else:
            serializable_value = value

        self._dbutils._task_values[key] = serializable_value

    def get(self, task_name: str, key: str, default_value: Any = None) -> Any:
        """
        Get a task value from upstream task.

        Args:
            task_name: Name of the upstream task
            key: Value key
            default_value: Default value if not found

        Returns:
            Task value or default
        """
        return self._dbutils._task_values.get(key, default_value)


class JobsAPI:
    """Databricks jobs API."""

    def __init__(self, dbutils_instance: 'DBUtils'):
        self.taskValues = TaskValuesAPI(dbutils_instance)


class NotebookAPI:
    """Databricks notebook API for output handling."""

    def __init__(self, dbutils_instance: 'DBUtils'):
        self._dbutils = dbutils_instance

    def exit(self, value: Any) -> None:
        """
        Exit notebook with return value.

        Args:
            value: Value to return from notebook execution
        """
        # Convert to JSON-serializable format
        if hasattr(value, 'dtype'):  # numpy types
            exit_value = value.item()
        elif hasattr(value, 'to_dict'):  # pandas objects
            exit_value = value.to_dict()
        elif hasattr(value, 'items'):  # dict with potential numpy values
            exit_value = {
                k: (v.item() if hasattr(v, 'dtype') else v)
                for k, v in value.items()
            }
        else:
            exit_value = value

        self._dbutils._exit_value = exit_value


class DBUtils:
    """
    Databricks DBUtils compatibility for Michelangelo.

    Supports essential data input/output patterns:
    - Widget-based parameterization (input)
    - Task value sharing (inter-task communication)
    - Notebook exit (output)

    Example:
        # Existing Databricks code works unchanged:
        dbutils.widgets.text("param", "default")
        value = dbutils.widgets.get("param")
        dbutils.jobs.taskValues.set("result", value)
        dbutils.notebook.exit({"status": "success"})
    """

    def __init__(self, input_params: Optional[Dict[str, Any]] = None):
        """
        Initialize DBUtils with input parameters.

        Args:
            input_params: Parameters passed to notebook execution
        """
        self._input_params = input_params or {}
        self._task_values: Dict[str, Any] = {}
        self._exit_value: Optional[Any] = None

        # Initialize core APIs for data input/output
        self.widgets = WidgetsAPI(self._input_params)
        self.jobs = JobsAPI(self)
        self.notebook = NotebookAPI(self)

    def get_exit_value(self) -> Optional[Any]:
        """Get the value passed to notebook.exit()."""
        return self._exit_value

    def get_task_values(self) -> Dict[str, Any]:
        """Get all task values set during execution."""
        return self._task_values.copy()

    def get_all_parameters(self) -> Dict[str, Any]:
        """Get all input parameters."""
        return self._input_params.copy()

    def __repr__(self) -> str:
        """String representation of DBUtils instance."""
        return f"DBUtils(params={len(self._input_params)}, task_values={len(self._task_values)})"