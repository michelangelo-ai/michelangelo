"""Workflow transpilation and packaging for Uniflow.

This module provides functionality to transpile Python workflow definitions into
Starlark code and package them into tarball archives. The transpilation process
converts Python task and workflow functions into Starlark equivalents, resolving
dependencies and generating a self-contained executable package.

The build system supports:
- Transpiling @task and @workflow decorated functions
- Resolving and packaging Python dependencies
- Including Starlark plugin bindings
- Generating tarball packages for distribution

Example:
    Building a workflow package::

        from michelangelo.uniflow.core.build import build

        @workflow()
        def my_workflow():
            result = my_task()
            return result

        package = build(my_workflow)
        tarball_bytes = package.to_tarball_bytes()

        # Save to file
        with open("workflow.tar.gz", "wb") as f:
            f.write(tarball_bytes)
"""

import argparse
import ast
import inspect
import logging
import sys
import tarfile
from collections.abc import Iterator
from dataclasses import dataclass
from io import BytesIO
from pathlib import Path
from typing import Any, Callable, Optional

import fsspec

from michelangelo.uniflow.core.decorator import (
    TaskFunction,
    get_star_plugin_binding,
    is_star_plugin,
    is_workflow,
)
from michelangelo.uniflow.core.task_config import Dependencies, TaskConfig
from michelangelo.uniflow.core.utils import (
    LOGGING_FORMAT,
    import_attribute,
    log_attributes,
)

log = logging.getLogger(__name__)


def main(args=None):
    r"""Command-line interface for building workflow packages.

    Parses command-line arguments, builds a workflow package from the specified
    function, and writes the resulting tarball to the output destination.

    Args:
        args: Optional list of command-line arguments. If None, uses sys.argv.

    Example:
        Command-line usage::

            python -m michelangelo.uniflow.core.build \\
                my.module.workflow_function \\
                output.tar.gz

        With dry-run::

            python -m michelangelo.uniflow.core.build \\
                my.module.workflow_function \\
                - --dry-run
    """
    p = argparse.ArgumentParser()
    p.add_argument("fn", type=import_attribute)
    p.add_argument("output")
    p.add_argument("--dry-run", action="store_true")

    a = p.parse_args(args=args)

    package = build(a.fn)
    tarball = package.to_tarball_bytes()

    if a.dry_run:
        log.info("dry_run")
        return

    with (
        sys.stdout.buffer if a.output == "-" else fsspec.open(a.output, mode="wb")
    ) as out:
        out.write(tarball)


class File:
    """Represents a Starlark file in the workflow package.

    Accumulates Starlark function definitions and load statements for a single
    file in the package. Ensures no duplicate functions or conflicting load
    statements are added.

    Attributes:
        _functions: Dictionary mapping function names to AST FunctionDef nodes.
        _loads: Dictionary mapping file paths to their exported symbols.
    """

    def __init__(self):
        """Initialize an empty Starlark file."""
        self._functions: dict[str, ast.FunctionDef] = {}
        self._loads: dict[str, dict[str, str]] = {}

    def add_function(self, v: ast.FunctionDef):
        """Add a function definition to the file.

        Args:
            v: The AST FunctionDef node to add.

        Raises:
            AssertionError: If a different function with the same name exists.
        """
        functions = self._functions
        if v.name in functions:
            assert functions[v.name] == v
        else:
            functions[v.name] = v

    def add_load(self, path: str, alias: str, attr: str):
        """Add a load statement to import an attribute from another file.

        Args:
            path: The file path to load from.
            alias: The alias name to use for the import.
            attr: The attribute name to import.

        Raises:
            AssertionError: If the same alias is already used for a different attribute.
        """
        loads = self._loads
        if path in loads:
            exports = loads[path]
            if alias in exports:
                assert exports[alias] == attr
            else:
                exports[alias] = attr
        else:
            loads[path] = {alias: attr}

    def has_function(self, name) -> bool:
        """Check if a function is already defined in this file.

        Args:
            name: Function name to check.

        Returns:
            True if the function is defined, False otherwise.
        """
        return name in self._functions

    def as_ast(self) -> ast.Module:
        """Generate the complete AST Module for this file.

        Constructs an AST Module with all load statements followed by all
        function definitions.

        Returns:
            An AST Module node representing the complete Starlark file.
        """
        body = [
            self._ast_load_expr(path, exports) for path, exports in self._loads.items()
        ]
        body += self._functions.values()
        assert body
        return ast.Module(body=body, type_ignores=[])

    @staticmethod
    def _ast_load_expr(path: str, exports: dict[str, str]) -> ast.Expr:
        """Generate AST for a Starlark load statement.

        Args:
            path: Path to load from.
            exports: Dictionary mapping aliases to attribute names.

        Returns:
            An AST Expr node representing the load statement.

        Example:
            Generates AST equivalent to::

                load("path/to/file.star", alias1="attr1", alias2="attr2")
        """
        call = ast.Call(
            func=ast.Name(
                id="load",
                ctx=ast.Load(),
            ),
            args=[
                ast.Constant(value=path),
            ],
            keywords=[
                ast.keyword(arg=k, value=ast.Constant(value=v))
                for k, v in exports.items()
            ],
        )
        return ast.Expr(call)


@dataclass
class Package:
    """A workflow package containing transpiled Starlark code.

    Represents a complete, self-contained workflow package with all necessary
    files and metadata. Can be serialized to a tarball for distribution.

    Attributes:
        files: Dictionary mapping file paths to their binary content.
        main_file: Path to the main entry point file.
        main_function: Name of the main workflow function.
    """

    files: dict[str, bytes]
    main_file: str
    main_function: str

    def to_tarball(self, file_obj):
        """Write the package to a tarball file object.

        Creates a gzipped tarball containing all package files and writes it
        to the provided file object. Logs the contents of each file for debugging.

        Args:
            file_obj: File-like object to write the tarball to.
        """
        file_log = ""
        main_file_log = None
        with tarfile.open(fileobj=file_obj, mode="w:gz") as tar:
            for path, code_bytes in self.files.items():
                info = tarfile.TarInfo(path)
                info.size = len(code_bytes)
                tar.addfile(info, BytesIO(initial_bytes=code_bytes))
                _log = f"""

#
# path: {info.path}
# size: {info.size} bytes
#

{code_bytes.decode("utf-8")}
"""
                if path == self.main_file:
                    assert main_file_log is None  # Should be unique
                    main_file_log = _log
                else:
                    file_log += _log

        assert main_file_log  # Should always be present
        # To enhance user convenience, we place the main file log at the end.
        # This arrangement makes it easier to read the main file content.
        file_log += main_file_log
        log.info("tarball: %s", file_log)

    def to_tarball_bytes(self) -> bytes:
        """Serialize the package to tarball bytes.

        Returns:
            The complete package as gzipped tarball bytes.

        Example:
            >>> package = build(my_workflow)
            >>> tarball_bytes = package.to_tarball_bytes()
            >>> with open("workflow.tar.gz", "wb") as f:
            ...     f.write(tarball_bytes)
        """
        bb = BytesIO()
        self.to_tarball(bb)
        return bb.getvalue()


class TranspilerCallback:
    """Callback interface for observing the transpilation process.

    Users can extend this class and override its methods to act on transpilation
    events, such as collecting metadata about transpiled tasks.

    Example:
        >>> class MyCallback(TranspilerCallback):
        ...     def __init__(self):
        ...         self.tasks = []
        ...
        ...     def on_task_function(self, task_fn):
        ...         self.tasks.append(task_fn)
        >>> callback = MyCallback()
        >>> package = build(my_workflow, transpiler_callback=callback)
        >>> print(f"Found {len(callback.tasks)} tasks")
    """

    def on_task_function(self, task_fn: TaskFunction):
        """Called when a @task function is encountered during transpilation.

        Args:
            task_fn: The TaskFunction instance being transpiled.
        """
        pass


def build(
    fn: Callable,
    transpiler_callback: Optional[TranspilerCallback] = None,
) -> Package:
    """Build a workflow package from a Python function.

    Transpiles the given workflow function and all its dependencies into Starlark
    code, packaging them into a self-contained tarball. The function must be
    decorated with @workflow.

    Args:
        fn: The workflow function to build. Must be decorated with @workflow.
        transpiler_callback: Optional callback to observe transpilation events.

    Returns:
        A Package containing the transpiled workflow and all dependencies.

    Example:
        >>> @workflow()
        ... def my_workflow():
        ...     result = process_data_task()
        ...     return result
        >>> package = build(my_workflow)
        >>> tarball = package.to_tarball_bytes()
    """
    # Skip build validation for YAML-generated workflows
    if hasattr(fn, '_uf_yaml_generated') and fn._uf_yaml_generated:
        # Return a minimal package for YAML workflows since they don't need transpilation
        return Package(
            files={"workflow.star": b"# YAML-generated workflow", "meta.json": b'{"main_file":"workflow.star","main_function":"yaml_workflow"}'},
            main_file="workflow.star",
            main_function="yaml_workflow"
        )

    files = {}
    fn_path = _transpile_function(fn, files, transpiler_callback)

    package_files: dict[str, bytes] = {}

    for path, file in files.items():
        if isinstance(file, bytes):
            content = file
        elif isinstance(file, ast.Module):
            content = ast.unparse(file).encode("utf-8")
        elif isinstance(file, File):
            file = file.as_ast()
            content = ast.unparse(file).encode("utf-8")
        else:
            raise TypeError(f"unsupported file type: {type(file)}: {file}")

        package_files[path.as_posix()] = content

    main_file = fn_path.as_posix()
    main_function = fn.__name__

    assert main_file in package_files

    meta_file = "meta.json"
    assert meta_file not in package_files, f"{meta_file} is a reserved file"
    package_files[meta_file] = (
        f'{{"main_file":"{main_file}","main_function":"{main_function}"}}'.encode()
    )

    return Package(
        files=package_files,
        main_file=main_file,
        main_function=main_function,
    )


def _transpile_function(
    fn: Callable,
    files: dict[Path, Any],
    transpiler_callback: Optional[TranspilerCallback],
) -> Path:
    """Transpile a Python function to Starlark and add to package files.

    Converts a Python workflow or task function into Starlark code, resolving
    all dependencies and adding them to the package. Returns the file path
    where the transpiled function is located.

    Args:
        fn: The Python function to transpile.
        files: Dictionary to accumulate package files.
        transpiler_callback: Optional callback for transpilation events.

    Returns:
        The Path where the transpiled function is located in the package.
    """
    fn = inspect.unwrap(
        fn
    )  # Get the user function by unwrapping decorators such as @workflow.
    fn_path = _fn_path(fn)

    file = files.get(fn_path)
    if not file:
        file = File()
        files[fn_path] = file

    assert isinstance(file, File)
    if file.has_function(fn.__name__):
        return fn_path

    # Get AST FunctionDef
    source = inspect.getsource(fn)
    tree = ast.parse(source)
    assert isinstance(tree, ast.Module)
    assert len(tree.body) == 1

    tree = tree.body[0]
    assert isinstance(tree, ast.FunctionDef)

    # Remove annotations and decorators
    tree.decorator_list = []
    for arg in tree.args.args:
        arg.annotation = None

    # Transform function's code using FunctionTransformer
    transformer = FunctionTransformer(fn, transpiler_callback)
    transformer.visit(tree)
    # Add transformed function to the file
    file.add_function(tree)

    # Process transformed function's dependencies and add them to the file
    deps = transformer.deps

    for alias, dep in deps.star_plugins.items():
        file.add_load("@plugin", alias, dep)

    for alias, (star_file, attribute) in deps.star_attributes.items():
        file.add_load(star_file.as_posix(), alias, attribute)
        _add_star_file(star_file, files)

    for alias, dep in deps.py_functions.items():
        dep_path = _fn_path(dep)
        if dep_path != fn_path:
            # External dependency - add it to the `load` statements
            file.add_load(dep_path.as_posix(), alias, dep.__name__)

        _transpile_function(dep, files, transpiler_callback)

    return fn_path


def _add_star_file(path: Path, files: dict[Path, ast.Module]):
    """Add a Starlark file and its dependencies to the package.

    Reads a .star file, parses it, resolves its load dependencies recursively,
    and adds everything to the package files.

    Args:
        path: Path to the Starlark file to add.
        files: Dictionary to accumulate package files.
    """
    if path in files:
        return

    star_code: ast.Module = ast.parse(path.read_text(), mode="exec")
    files[path] = star_code

    # Resolve dependencies (`load` statements) and recursively add them to the package.
    for node in _iter_top_level_calls(star_code):
        assert isinstance(node.func, ast.Name)
        if node.func.id != "load":
            # Consider only `load` calls
            continue

        # Assumes that all `load` calls have 1st argument to be a constant string.
        path_constant = node.args[0]
        assert isinstance(path_constant, ast.Constant)
        assert isinstance(path_constant.value, str)

        if path_constant.value.startswith("@"):
            # Skip "@" dependencies (built-ins such as @plugin)
            continue

        dep_path = Path(path_constant.value)
        assert not dep_path.is_absolute()

        dep_path = path.parent / dep_path
        dep_path = dep_path.resolve()

        assert dep_path.is_file(), dep_path

        path_constant.value = dep_path.as_posix()

        _add_star_file(dep_path, files)


def _iter_top_level_calls(module: ast.Module) -> Iterator[ast.Call]:
    r"""Find and yield all top-level function calls in an AST module.

    Iterates through module-level statements and yields Call nodes,
    primarily used to find `load` statements in Starlark source code.

    Args:
        module: The AST Module to search.

    Yields:
        AST Call nodes found at the top level of the module.

    Example:
        >>> tree = ast.parse("load('file.star', foo='bar')\\nresult = func()")
        >>> calls = list(_iter_top_level_calls(tree))
        >>> len(calls)
        2
    """
    for node in module.body:
        if isinstance(node, ast.Expr):
            node = node.value

        if not isinstance(node, ast.Call):
            continue

        assert isinstance(node.func, ast.Name)
        yield node


def _fn_path(fn: Callable) -> Path:
    """Get the absolute file path where a function is defined.

    Args:
        fn: The function to locate.

    Returns:
        Resolved absolute Path to the file containing the function.

    Example:
        >>> def my_func():
        ...     pass
        >>> path = _fn_path(my_func)
        >>> path.is_absolute()
        True
    """
    return Path(inspect.getabsfile(fn)).resolve()


class FunctionTransformer(ast.NodeTransformer):
    """AST transformer for converting Python workflow code to Starlark.

    Walks through the AST of a Python function and transforms it to be
    Starlark-compatible. Resolves references to tasks, workflows, and
    Starlark plugins, replacing them with appropriate Starlark constructs.

    Attributes:
        _code: The code object of the function being transformed.
        _module: The module containing the function.
        _transpiler_callback: Optional callback for transpilation events.
        deps: Dependencies collection tracking all external references.
    """

    def __init__(
        self,
        fn,
        transpiler_callback: Optional[TranspilerCallback],
    ):
        """Initialize the transformer.

        Args:
            fn: The function to transform.
            transpiler_callback: Optional callback for transpilation events.
        """
        self._code = fn.__code__
        self._module = inspect.getmodule(fn)
        self._transpiler_callback = transpiler_callback
        self.deps = Dependencies()

    def visit_AnnAssign(self, node):  # noqa: N802
        """Transform annotated assignments to simple assignments.

        Converts type-annotated assignments to plain assignments since
        Starlark doesn't support type annotations.

        Args:
            node: The AnnAssign node to transform.

        Returns:
            An Assign node without type annotation.

        Example:
            Transforms::

                a: dict = foo()

            To::

                a = foo()
        """
        return ast.Assign(value=node.value, targets=[node.target])

    def visit_Is(self, _node):  # noqa: N802
        """Reject 'is' operator.

        Raises:
            NameError: Always, as 'is' is not supported in Starlark.
        """
        raise NameError("[is] is not supported, use == instead")

    def visit_IsNot(self, _node):  # noqa: N802
        """Reject 'is not' operator.

        Raises:
            NameError: Always, as 'is not' is not supported in Starlark.
        """
        raise NameError("[is not] is not supported, use != instead")

    def visit_Import(self, _node):  # noqa: N802
        """Reject import statements.

        Raises:
            NameError: Always, as imports are not supported in Starlark.
        """
        raise NameError("[import] is not supported")

    def visit_ImportFrom(self, _node):  # noqa: N802
        """Reject from-import statements.

        Raises:
            NameError: Always, as imports are not supported in Starlark.
        """
        raise NameError("[import] is not supported")

    def visit_Try(self, _node):  # noqa: N802
        """Reject try-except blocks.

        Raises:
            NameError: Always, as try-except is not supported in Starlark.
        """
        raise NameError("[try] is not supported")

    def visit_Name(self, node: ast.Name):  # noqa: N802
        """Transform name references to Starlark-compatible forms.

        Handles variable and function references, converting task references
        to Starlark task calls, workflow references to function calls, and
        resolving Starlark plugins.

        Args:
            node: The Name node to transform.

        Returns:
            The transformed AST node, or the original node if no transformation needed.

        Raises:
            ValueError: If a global variable reference cannot be resolved.
            NameError: If an unsupported global variable is referenced.
        """
        if not isinstance(node.ctx, ast.Load):
            # Skip non load var context. I.e. keep var assignment code as-is.
            return node

        if node.id in self._code.co_varnames:
            # Local var - keep as-is.
            return node

        # Assumption: current variable represents a global variable load.
        if node.id not in self._code.co_names:
            log_attributes(log, logging.ERROR, self._code)
            raise ValueError(
                f"'{node.id}' is not found in the function's globals: "
                f"{self._code.co_names}",
            )

        if not hasattr(self._module, node.id):
            # Global var is not defined by the module.
            # Assumption: Global var is Starlark-compatible built-in. Keep as-is.
            assert node.id in (
                "abs",
                "any",
                "all",
                "bool",
                "bytes",
                "dict",
                "dir",
                "enumerate",
                "float",
                "fail",
                "getattr",
                "hasattr",
                "hash",
                "int",
                "len",
                "list",
                "max",
                "min",
                "print",
                "range",
                "repr",
                "reversed",
                "sorted",
                "str",
                "tuple",
                "type",
                "zip",
            )
            return node

        # Global var is defined by the module, get its value for further introspection
        v = getattr(self._module, node.id)
        v = inspect.unwrap(v)

        if task := getattr(v, "_uf_task", None):
            assert isinstance(task, TaskFunction)
            if self._transpiler_callback:
                self._transpiler_callback.on_task_function(task)
            return task._transpile(self.deps)

        if is_star_plugin(v):
            plugin_id, function = get_star_plugin_binding(v).split(".")
            alias = f"__{plugin_id}__"
            ast_node = ast.Name(id=f"{alias}.{function}", ctx=ast.Load())
            self.deps.add_star_plugin(alias, plugin_id)
            return ast_node

        if is_workflow(v):
            self.deps.add_py_function(node.id, v)
            return node

        if issubclass(v, TaskConfig):
            config_binding = v.get_config_binding()
            self.deps.add_star_attribute(
                config_binding.export,
                config_binding.star_file,
                config_binding.function,
            )
            ast_node = ast.Name(id=config_binding.export, ctx=ast.Load())
            return ast_node

        raise NameError(f"unsupported global variable: {self._module} {node.id}")


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)
    main()
