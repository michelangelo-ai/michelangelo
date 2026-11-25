"""Tests for the declarative interpreter release assembly tool."""

import ast
import copy
import hashlib
import io
import json
import shutil
import tarfile
from pathlib import Path

import pytest
import yaml

from michelangelo.uniflow.declarative.assemble import (
    ReleaseSourceError,
    assemble,
    generate_main,
    parse_source,
)

REPO_ROOT = Path(__file__).resolve().parents[4]
SOURCE = "python/michelangelo/uniflow/declarative/releases/core-v0.1.0.src.yaml"
GOLDEN_TAR = REPO_ROOT / "go/worker/declarative/testdata/release/core-v0.1.0.tar.gz"

ENTRIES = [
    "/.declarative-build/core-v0.1.0/main.star",
    "/python/michelangelo/uniflow/declarative/engine.star",
    "/python/michelangelo/uniflow/commons.star",
    "/python/michelangelo/uniflow/plugins/ray/task.star",
    "meta.json",
]

LOADS = {
    "/.declarative-build/core-v0.1.0/main.star": [
        "/python/michelangelo/uniflow/declarative/engine.star",
        "/python/michelangelo/uniflow/plugins/ray/task.star",
    ],
    "/python/michelangelo/uniflow/declarative/engine.star": [
        "/python/michelangelo/uniflow/commons.star",
    ],
    "/python/michelangelo/uniflow/commons.star": [],
    "/python/michelangelo/uniflow/plugins/ray/task.star": [
        "/python/michelangelo/uniflow/commons.star",
    ],
}


def _copy_root(dst: Path) -> Path:
    """Copy the release source and every Starlark file it can load to dst."""
    src = REPO_ROOT / "python/michelangelo/uniflow"
    for star in src.rglob("*.star"):
        target = dst / "python/michelangelo/uniflow" / star.relative_to(src)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(star, target)
    (dst / SOURCE).parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(REPO_ROOT / SOURCE, dst / SOURCE)
    return dst


def _assemble(root: Path, out: Path) -> tuple[bytes, dict]:
    """Assemble the core-v0.1.0 source under root into out."""
    tar_path, release_path = assemble(root / SOURCE, root, out)
    return tar_path.read_bytes(), yaml.safe_load(release_path.read_text())


def _members(tar_bytes: bytes) -> dict[str, bytes]:
    """Return each tar member name mapped to its content, in tar order."""
    with tarfile.open(fileobj=io.BytesIO(tar_bytes), mode="r:gz") as tar:
        return {m.name: tar.extractfile(m).read() for m in tar.getmembers()}


def _loads(src: bytes) -> list[str]:
    """Return the module of each load statement that is not a plugin."""
    out = []
    for node in ast.parse(src).body:
        call = node.value if isinstance(node, ast.Expr) else None
        if isinstance(call, ast.Call) and getattr(call.func, "id", "") == "load":
            module = call.args[0].value
            if not module.startswith("@"):
                out.append(module)
    return out


def _source() -> dict:
    """Return a valid release source document."""
    return yaml.safe_load((REPO_ROOT / SOURCE).read_text())


def test_assemble_same_bytes_twice(tmp_path):
    """Two runs on one root write the same bytes with a fixed gzip header."""
    root = _copy_root(tmp_path / "root")
    first, _ = _assemble(root, tmp_path / "out1")
    second, _ = _assemble(root, tmp_path / "out2")
    assert first == second
    assert first[4:8] == b"\x00\x00\x00\x00"  # gzip MTIME
    assert first[3] & 0x08 == 0  # gzip FNAME flag


def test_assemble_same_bytes_two_roots(tmp_path):
    """The same tree under two roots gives the same bytes."""
    first, _ = _assemble(_copy_root(tmp_path / "a"), tmp_path / "out1")
    second, _ = _assemble(_copy_root(tmp_path / "b" / "nested"), tmp_path / "out2")
    assert first == second


def test_assemble_entries(tmp_path):
    """Entries and loads are root-relative and equal the golden tar."""
    tar_bytes, _ = _assemble(_copy_root(tmp_path / "root"), tmp_path / "out")
    members = _members(tar_bytes)
    assert list(members) == ENTRIES
    assert {n: _loads(src) for n, src in members.items() if n != "meta.json"} == LOADS
    assert json.loads(members["meta.json"]) == {
        "main_file": "/.declarative-build/core-v0.1.0/main.star",
        "main_function": "main",
    }

    golden = _members(GOLDEN_TAR.read_bytes())
    assert list(golden) == ENTRIES
    assert {n: _loads(src) for n, src in golden.items() if n != "meta.json"} == LOADS


def test_generated_main(tmp_path):
    """main.star loads the engine and each factory, and defines REGISTRY."""
    root = _copy_root(tmp_path / "root")
    source = parse_source(_source())
    main = generate_main(source, root / ".declarative-build" / "core-v0.1.0", root)
    assert main == (
        'load("../../python/michelangelo/uniflow/declarative/engine.star", "run")\n'
        'load("../../python/michelangelo/uniflow/plugins/ray/task.star",'
        ' factory_core_ray = "task")\n'
        "\n"
        "REGISTRY = {\n"
        '    "core/ray": {"factory": factory_core_ray, "job_specs_key": "ray",'
        ' "capabilities": ["retry"]},\n'
        "}\n"
        "\n"
        "def main(spec, params):\n"
        "    return run(spec, params, REGISTRY)\n"
    )


def test_release_file_fields(tmp_path):
    """The release file holds what ParseRelease reads, and the tar digest."""
    tar_bytes, release = _assemble(_copy_root(tmp_path / "root"), tmp_path / "out")
    assert release == {
        "version": "core-v0.1.0",
        "artifact": "s3://default/interpreters/core-v0.1.0.tar.gz",
        "digest": "sha256:" + hashlib.sha256(tar_bytes).hexdigest(),
        "task_types": {
            "core/ray": {
                "capabilities": ["retry"],
                "config": {
                    "entry_point": {"type": "string", "required": True},
                    "args": {"type": "list"},
                    "kwargs": {"type": "dict"},
                },
                "job_specs": {
                    "ray": {
                        "head_cpu": {"type": "integer"},
                        "worker_instances": {"type": "integer"},
                    }
                },
            }
        },
    }


def _ray(doc: dict) -> dict:
    return doc["task_types"]["core/ray"]


def _unknown_top(doc):
    doc["owner"] = "x"


def _unknown_type_key(doc):
    _ray(doc)["image"] = "x"


def _bad_type_name(doc):
    doc["task_types"]["core/ra-y"] = doc["task_types"].pop("core/ray")


def _bad_job_specs_key(doc):
    _ray(doc)["job_specs_key"] = "ra-y"
    _ray(doc)["job_specs"] = {"ra-y": {}}


def _other_job_specs_key(doc):
    _ray(doc)["job_specs"]["spark"] = {"cpu": {"type": "integer"}}


def _field_in_both(doc):
    _ray(doc)["config"]["head_cpu"] = {"type": "integer"}


def _alias_field(doc):
    _ray(doc)["config"]["alias"] = {"type": "string"}


def _retry_attempts_field(doc):
    _ray(doc)["job_specs"]["ray"]["retry_attempts"] = {"type": "integer"}


@pytest.mark.parametrize(
    ("mutate", "message"),
    [
        (_unknown_top, 'release source: unknown key "owner"'),
        (_unknown_type_key, 'task type "core/ray": unknown key "image"'),
        (
            _bad_type_name,
            'task type "core/ra-y": the name is not an identifier'
            ' after mapping "/" and "." to "_"',
        ),
        (
            _bad_job_specs_key,
            'task type "core/ray": job_specs_key "ra-y" is not an identifier'
            ' after mapping "/" and "." to "_"',
        ),
        (
            _other_job_specs_key,
            'task type "core/ray": job_specs key "spark" is not job_specs_key "ray"',
        ),
        (
            _field_in_both,
            'task type "core/ray": field "head_cpu" is in both config and job_specs',
        ),
        (
            _alias_field,
            'task type "core/ray": field "alias" is reserved; the engine sets it',
        ),
        (
            _retry_attempts_field,
            'task type "core/ray": field "retry_attempts" is reserved;'
            " the engine sets it",
        ),
    ],
)
def test_parse_source_rejects(mutate, message):
    """Each rule of the release source has its own error."""
    doc = _source()
    parse_source(doc)
    mutate(doc)
    with pytest.raises(ReleaseSourceError) as e:
        parse_source(doc)
    assert str(e.value) == message


def test_generated_main_quotes_strings(tmp_path):
    """A quote in a type name is rejected; other strings go through json.dumps."""
    doc = _source()
    doc["task_types"]['core/r"y'] = doc["task_types"].pop("core/ray")
    with pytest.raises(ReleaseSourceError) as e:
        parse_source(doc)
    assert str(e.value) == (
        'task type "core/r\\"y": the name is not an identifier'
        ' after mapping "/" and "." to "_"'
    )

    doc = _source()
    _ray(doc)["capabilities"] = ['re"try\n']
    root = _copy_root(tmp_path / "root")
    main = generate_main(parse_source(copy.deepcopy(doc)), root / "build", root)
    assert '["re\\"try\\n"]' in main
    ast.parse(main)
