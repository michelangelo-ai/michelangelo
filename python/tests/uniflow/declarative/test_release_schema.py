"""The release schema of each task type matches its Python task config."""

import ast
from pathlib import Path

import pytest
import yaml

REPO_ROOT = Path(__file__).resolve().parents[4]
SOURCE = "python/michelangelo/uniflow/declarative/releases/core-v0.1.0.src.yaml"
PLUGINS = REPO_ROOT / "python/michelangelo/uniflow/plugins"

# The schema leaves out breakpoint because it holds a cluster for debugging,
# and runtime_env because it can add packages and env vars to the cluster.
EXCLUDED = {
    "core/ray": {"breakpoint", "runtime_env"},
    "core/spark": set(),
}

CONFIGS = {
    "core/ray": (PLUGINS / "ray/task.py", "RayTask"),
    "core/spark": (PLUGINS / "spark/task.py", "SparkTask"),
}

_SCHEMA_TYPES = {
    "int": "integer",
    "str": "string",
    "float": "number",
    "bool": "boolean",
    "list": "list",
    "dict": "dict",
}


def _schema_type(annotation: ast.expr) -> str:
    # Optional[X] is X: the factory default applies when the field is unset.
    if isinstance(annotation, ast.Subscript) and ast.unparse(annotation.value) in (
        "Optional",
        "typing.Optional",
    ):
        annotation = annotation.slice
    return _SCHEMA_TYPES[ast.unparse(annotation)]


def _dataclass_fields(path: Path, name: str) -> dict:
    # Parse the source: importing SparkTask needs pyspark.
    for node in ast.parse(path.read_text()).body:
        if isinstance(node, ast.ClassDef) and node.name == name:
            assert [ast.unparse(b) for b in node.bases] == ["TaskConfig"]
            return {
                stmt.target.id: _schema_type(stmt.annotation)
                for stmt in node.body
                if isinstance(stmt, ast.AnnAssign)
            }
    raise AssertionError(f"{path} has no class {name}")


@pytest.mark.parametrize("task_type", sorted(CONFIGS))
def test_job_specs_match_task_config(task_type):
    """Each config field has a job_specs field of the same type."""
    source = yaml.safe_load((REPO_ROOT / SOURCE).read_text())
    t = source["task_types"][task_type]
    fields = t["job_specs"][t["job_specs_key"]]
    schema = {field: decl["type"] for field, decl in fields.items()}

    config = _dataclass_fields(*CONFIGS[task_type])
    assert EXCLUDED[task_type] <= config.keys()
    want = {f: kind for f, kind in config.items() if f not in EXCLUDED[task_type]}
    assert schema == want


def test_every_task_type_has_a_config():
    """A new task type in the release needs an entry in CONFIGS."""
    source = yaml.safe_load((REPO_ROOT / SOURCE).read_text())
    assert sorted(source["task_types"]) == sorted(CONFIGS)
