import argparse
import ast
import inspect
import logging
import sys
import tarfile
from dataclasses import dataclass
from io import BytesIO
from pathlib import Path
from typing import Any, Callable
from collections.abc import Iterator

import fsspec

from michelangelo.uniflow.core.decorator import (
    is_star_plugin,
    is_workflow,
    get_star_plugin_binding,
    TaskFunction,
)
from michelangelo.uniflow.core.task_config import Dependencies
from michelangelo.uniflow.core.utils import (
    LOGGING_FORMAT,
    import_attribute,
    log_attributes,
)

log = logging.getLogger(__name__)


def main(args=None):
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
        sys.stdout.buffer
        if a.output == "-"
        else fsspec.open(a.output, mode="wb") as out
    ):
        out.write(tarball)


class File:
    def __init__(self):
        self._functions: dict[str, ast.FunctionDef] = {}
        self._loads: dict[str, dict[str, str]] = {}

    def add_function(self, v: ast.FunctionDef):
        functions = self._functions
        if v.name in functions:
            assert functions[v.name] == v
        else:
            functions[v.name] = v

    def add_load(self, path: str, alias: str, attr: str):
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
        return name in self._functions

    def as_ast(self) -> ast.Module:
        body = [
            self._ast_load_expr(path, exports) for path, exports in self._loads.items()
        ]
        body += self._functions.values()
        assert body
        return ast.Module(body=body, type_ignores=[])

    @staticmethod
    def _ast_load_expr(path: str, exports: dict[str, str]) -> ast.Expr:
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
    files: dict[str, bytes]
    main_file: str
    main_function: str

    def to_tarball(self, file_obj):
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
        bb = BytesIO()
        self.to_tarball(bb)
        return bb.getvalue()


def build(fn: Callable) -> Package:
    # TODO: andrii: strip_path_prefix
    files = {}
    fn_path = _transpile_function(fn, files)

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


def _transpile_function(fn: Callable, files: dict[Path, Any]) -> Path:
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
    transformer = FunctionTransformer(fn)
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

        _transpile_function(dep, files)

    return fn_path


def _add_star_file(path: Path, files: dict[Path, ast.Module]):
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
    """
    Utility function that finds and yields all top-level function call statements in the given AST module.
    It is used to find `load` statements in the starlark source code.
    """
    for node in module.body:
        if isinstance(node, ast.Expr):
            node = node.value

        if not isinstance(node, ast.Call):
            continue

        assert isinstance(node.func, ast.Name)
        yield node


def _fn_path(fn: Callable) -> Path:
    """Function's definition path"""
    return Path(inspect.getabsfile(fn)).resolve()


class FunctionTransformer(ast.NodeTransformer):
    def __init__(self, fn):
        self._code = fn.__code__
        self._module = inspect.getmodule(fn)
        self.deps = Dependencies()

    def visit_AnnAssign(self, node):
        # Replace annotated assignment with just assignment. Ex:
        # a: dict = foo()  ->  a = foo()
        return ast.Assign(
            value=node.value,
            targets=[node.target],
        )

    def visit_Is(self, _node):
        raise NameError("[is] is not supported, use == instead")

    def visit_IsNot(self, _node):
        raise NameError("[is not] is not supported, use != instead")

    def visit_Import(self, _node):
        raise NameError("[import] is not supported")

    def visit_ImportFrom(self, _node):
        raise NameError("[import] is not supported")

    def visit_Try(self, _node):
        raise NameError("[try] is not supported")

    def visit_Name(self, node: ast.Name):
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
                f"'{node.id}' is not found in the function's globals: {self._code.co_names}",
            )

        if not hasattr(self._module, node.id):
            # Global var is not defined by the module.
            # Assumption: Global var is Starlark-compatible built-in functions. Keep it as-is.
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

        raise NameError(f"unsupported global variable: {self._module} {node.id}")


if __name__ == "__main__":
    logging.basicConfig(level=logging.INFO, format=LOGGING_FORMAT)
    main()
