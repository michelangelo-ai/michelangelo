import ast
import dataclasses
from abc import ABC, abstractmethod
from pathlib import Path
from typing import Callable


class Dependencies:
    """A collection of dependencies is required to run a transpiled workflow function.
    These dependencies are gathered during the workflow transpilation process and are used to generate a tarball file that contains all the necessary code.

    There are three types of dependencies:

    1. Starlark Attributes: Functions or other attributes defined in Starlark code within *.star files.
    2. Starlark Plugins: Built-in Starlark functions or functions loaded via the @plugin module.
    3. Python Functions: Functions defined in Python code, including other workflow functions that the main workflow may call.
    """

    def __init__(self):
        self.star_attributes: dict[str, tuple[Path, str]] = {}
        self.star_plugins: dict[str, str] = {}
        self.py_functions: dict[str, Callable] = {}

    def add_star_attribute(self, alias: str, star_file: Path, attribute: str):
        star_file = star_file.resolve()
        self.star_attributes[alias] = (
            star_file,
            attribute,
        )

    def add_star_plugin(self, alias: str, plugin_id: str):
        self.star_plugins[alias] = plugin_id

    def add_py_function(self, alias: str, fn: Callable):
        self.py_functions[alias] = fn


@dataclasses.dataclass
class TaskBinding:
    """TaskBinding is a dataclass that represents the connection between TaskConfig and its associated Starlark function, which orchestrates task execution.
    It is utilized during the workflow function transpilation process to track dependencies and generate Starlark `load(...)` statements that import those
    dependencies.

    The format of the generated Starlark load statement is as follows: `load(star_file, export="function")`.

    For further details on usage, refer to TaskConfig.get_binding().
    """

    star_file: Path  # Path to the Starlark file where the function is defined
    function: str  # Name of the Starlark function
    export: str  # Name of the exported function in the Starlark file where the function is used. Ex: load("")


class TaskConfig(ABC):
    """TaskConfig serves as the foundational class for all task configurations. Its subclasses define the following:

    - Task configuration properties, which are represented as fields in a dataclass.
    - Pre-run and post-run hooks that are executed before and after the task function is executed.
    - TaskBinding, which represents a Starlark function that orchestrates the execution of the task.
    """

    @abstractmethod
    def get_binding(self) -> TaskBinding:
        """Returns the TaskBinding object for the TaskConfig, linking it with the Starlark function that drives
        the task orchestration logic.

        Example:
            def get_binding(self) -> TaskBinding:
                return TaskBinding(
                    star_file=Path(__file__).parent / "x_task.star",
                    function="x_task",
                    export="__x_task",
                )

        In this example, the Starlark function `x_task` is defined in the `x_task.star` file and is
        exported (loaded) as `__x_task`. A double underscore prefix for the exported function name is
        a best practice to prevent naming conflicts with user workflow code.
        """
        raise NotImplementedError

    @abstractmethod
    def pre_run(self):
        """Pre-run hook executed before task function"""
        raise NotImplementedError

    @abstractmethod
    def post_run(self):
        """Post-run hook executed after task function"""
        raise NotImplementedError

    def to_keywords(self) -> list[ast.keyword]:
        """Generates a list of AST keyword nodes from the class properties that are not None.
        The returned AST keywords are used in constructing the Starlark function call.
        """
        assert dataclasses.is_dataclass(self)
        res = []
        for f in dataclasses.fields(self):
            v = getattr(self, f.name)
            if v is not None:
                # Exclude None-valued properties from the keyword list.
                # Perhaps we should revise keyword exclusion logic. Instead of excluding None, we should exclude special "Undefined" values.
                # TODO: andrii: Consider using special "Undefined" marker object for keyword exclusion.
                k = ast.keyword(f.name, ast.Constant(v))
                res.append(k)
        return res
