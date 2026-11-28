"""Tests for the declarative interpreter release assembly tool."""

import ast
import hashlib
import io
import json
import shutil
import tarfile
from dataclasses import replace
from pathlib import Path

import pytest
import yaml

from michelangelo.uniflow.declarative.assemble import (
    ReleaseSource,
    ReleaseSourceError,
    assemble,
    generate_main,
    main,
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
    "/python/michelangelo/uniflow/plugins/spark/task.star",
    "meta.json",
]

LOADS = {
    "/.declarative-build/core-v0.1.0/main.star": [
        "/python/michelangelo/uniflow/declarative/engine.star",
        "/python/michelangelo/uniflow/plugins/ray/task.star",
        "/python/michelangelo/uniflow/plugins/spark/task.star",
    ],
    "/python/michelangelo/uniflow/declarative/engine.star": [
        "/python/michelangelo/uniflow/commons.star",
    ],
    "/python/michelangelo/uniflow/commons.star": [],
    "/python/michelangelo/uniflow/plugins/ray/task.star": [
        "/python/michelangelo/uniflow/commons.star",
    ],
    "/python/michelangelo/uniflow/plugins/spark/task.star": [
        "/python/michelangelo/uniflow/commons.star",
    ],
}


def _copy_root(dst: Path) -> Path:
    src = REPO_ROOT / "python/michelangelo/uniflow"
    for star in src.rglob("*.star"):
        target = dst / "python/michelangelo/uniflow" / star.relative_to(src)
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(star, target)
    (dst / SOURCE).parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(REPO_ROOT / SOURCE, dst / SOURCE)
    return dst


def _assemble(root: Path, out: Path) -> tuple[bytes, dict]:
    tar_path, release_path = assemble(root / SOURCE, root, out)
    return tar_path.read_bytes(), yaml.safe_load(release_path.read_text())


def _members(tar_bytes: bytes) -> dict[str, bytes]:
    with tarfile.open(fileobj=io.BytesIO(tar_bytes), mode="r:gz") as tar:
        return {m.name: tar.extractfile(m).read() for m in tar.getmembers()}


def _loads(src: bytes) -> list[str]:
    out = []
    for node in ast.parse(src).body:
        call = node.value if isinstance(node, ast.Expr) else None
        if isinstance(call, ast.Call) and getattr(call.func, "id", "") == "load":
            module = call.args[0].value
            if not module.startswith("@"):
                out.append(module)
    return out


def _source() -> dict:
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


def test_golden_tar_is_fresh(tmp_path):
    """The golden tar holds the Starlark that the repo sources assemble to.

    The digest depends on the Python and zlib versions, so the test
    compares the parsed source of each member, not the bytes.
    """
    tar_bytes, _ = _assemble(_copy_root(tmp_path / "root"), tmp_path / "out")
    fresh = _members(tar_bytes)
    golden = _members(GOLDEN_TAR.read_bytes())
    assert set(golden) == set(fresh)
    for name in sorted(set(golden) - {"meta.json"}):
        assert ast.dump(ast.parse(golden[name])) == ast.dump(ast.parse(fresh[name])), (
            f"{name} is stale; rerun the assembly tool on the release source"
        )


def test_generated_main(tmp_path):
    """main.star loads the engine and each factory, and defines REGISTRY."""
    root = _copy_root(tmp_path / "root")
    source = parse_source(_source())
    main = generate_main(source, root / ".declarative-build" / "core-v0.1.0", root)
    assert main == (
        'load("../../python/michelangelo/uniflow/declarative/engine.star", "run")\n'
        'load("../../python/michelangelo/uniflow/plugins/ray/task.star",'
        ' factory_core_ray = "task")\n'
        'load("../../python/michelangelo/uniflow/plugins/spark/task.star",'
        ' factory_core_spark = "spark_task")\n'
        "\n"
        "REGISTRY = {\n"
        '    "core/ray": {"factory": factory_core_ray, "job_specs_key": "ray",'
        ' "capabilities": ["retry"]},\n'
        '    "core/spark": {"factory": factory_core_spark, "job_specs_key": "spark",'
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
                        "head_memory": {"type": "string"},
                        "head_disk": {"type": "string"},
                        "head_gpu": {"type": "integer"},
                        "head_object_store_memory": {"type": "integer"},
                        "worker_cpu": {"type": "integer"},
                        "worker_memory": {"type": "string"},
                        "worker_disk": {"type": "string"},
                        "worker_gpu": {"type": "integer"},
                        "worker_object_store_memory": {"type": "integer"},
                        "worker_instances": {"type": "integer"},
                    }
                },
            },
            "core/spark": {
                "capabilities": ["retry"],
                "config": {
                    "entry_point": {"type": "string", "required": True},
                    "args": {"type": "list"},
                    "kwargs": {"type": "dict"},
                },
                "job_specs": {
                    "spark": {
                        "driver_cpu": {"type": "integer"},
                        "driver_memory": {"type": "string"},
                        "driver_disk": {"type": "string"},
                        "driver_gpu": {"type": "integer"},
                        "executor_cpu": {"type": "integer"},
                        "executor_memory": {"type": "string"},
                        "executor_disk": {"type": "string"},
                        "executor_gpu": {"type": "integer"},
                        "executor_instances": {"type": "integer"},
                    }
                },
            },
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


def _version_newline(doc):
    doc["version"] = "core-v0.1.0\n"


def _capabilities_not_list(doc):
    _ray(doc)["capabilities"] = "retry"


def _capabilities_empty_dict(doc):
    _ray(doc)["capabilities"] = {}


def _capabilities_not_strings(doc):
    _ray(doc)["capabilities"] = [1]


def _unknown_capability(doc):
    _ray(doc)["capabilities"] = ["retry", "cache"]


def _non_ascii_type_name(doc):
    doc["task_types"]["core/r\u00e9y"] = doc["task_types"].pop("core/ray")


def _non_ascii_job_specs_key(doc):
    _ray(doc)["job_specs_key"] = "r\u00e9y"
    _ray(doc)["job_specs"] = {"r\u00e9y": {}}


def _non_ascii_factory(doc):
    _ray(doc)["factory"] = "python/michelangelo/uniflow/plugins/ray/task.star:t\u00e9sk"


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
        (
            _version_newline,
            'release source: version "core-v0.1.0\\n"'
            " must match [A-Za-z0-9][A-Za-z0-9._-]*",
        ),
        (
            _capabilities_not_list,
            'task type "core/ray": capabilities must be a list of strings',
        ),
        (
            _capabilities_empty_dict,
            'task type "core/ray": capabilities must be a list of strings',
        ),
        (
            _capabilities_not_strings,
            'task type "core/ray": capabilities must be a list of strings',
        ),
        (
            _unknown_capability,
            'task type "core/ray": unknown capability "cache";'
            ' known: "outcome_mode", "retry"',
        ),
        (
            _non_ascii_type_name,
            'task type "core/r\\u00e9y": the name is not an identifier'
            ' after mapping "/" and "." to "_"',
        ),
        (
            _non_ascii_job_specs_key,
            'task type "core/ray": job_specs_key "r\\u00e9y" is not an identifier'
            ' after mapping "/" and "." to "_"',
        ),
        (
            _non_ascii_factory,
            'task type "core/ray": factory'
            ' "python/michelangelo/uniflow/plugins/ray/task.star:t\\u00e9sk"'
            " must be <file>.star:<function>",
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

    ray = parse_source(_source()).task_types["core/ray"]
    source = ReleaseSource(
        version="v",
        artifact="a",
        task_types={"core/ray": replace(ray, capabilities=['re"try\n'])},
    )
    root = _copy_root(tmp_path / "root")
    out = generate_main(source, root / "build", root)
    assert '["re\\"try\\n"]' in out
    ast.parse(out)


def test_assemble_same_output_is_noop(tmp_path):
    """A second run that writes the same bytes succeeds and changes nothing."""
    root = _copy_root(tmp_path / "root")
    out = tmp_path / "out"
    first, _ = _assemble(root, out)
    paths = [out / "core-v0.1.0.tar.gz", out / "core-v0.1.0.yaml"]
    before = [p.stat().st_mtime_ns for p in paths]
    second, _ = _assemble(root, out)
    assert first == second
    assert [p.stat().st_mtime_ns for p in paths] == before
    assert sorted(out.iterdir()) == paths


def test_assemble_restores_release_file(tmp_path):
    """A run that stopped after the tar write completes on the next run."""
    root = _copy_root(tmp_path / "root")
    out = tmp_path / "out"
    _, release = _assemble(root, out)
    (out / "core-v0.1.0.yaml").unlink()
    _, again = _assemble(root, out)
    assert again == release


def test_assemble_refuses_changed_release(tmp_path):
    """A release version is immutable: new bytes for it are an error."""
    root = _copy_root(tmp_path / "root")
    out = tmp_path / "out"
    _assemble(root, out)
    release = (out / "core-v0.1.0.yaml").read_text()
    engine = root / "python/michelangelo/uniflow/declarative/engine.star"
    engine.write_text(engine.read_text() + "\nEXTRA = 1\n")
    with pytest.raises(FileExistsError) as e:
        _assemble(root, out)
    assert str(e.value) == (
        f"{out / 'core-v0.1.0.tar.gz'} exists with other bytes;"
        " bump the version in the release source"
    )
    assert (out / "core-v0.1.0.yaml").read_text() == release


def test_main_reports_errors(tmp_path, capsys):
    """The CLI prints a bad source or a file error to stderr and exits 2."""
    root = _copy_root(tmp_path / "root")
    doc = _source()
    doc["owner"] = "x"
    bad = tmp_path / "bad.yaml"
    bad.write_text(yaml.safe_dump(doc))
    for source, message in [
        (bad, 'error: release source: unknown key "owner"\n'),
        (
            tmp_path / "missing.yaml",
            "error: [Errno 2] No such file or directory:"
            f" '{tmp_path / 'missing.yaml'}'\n",
        ),
    ]:
        with pytest.raises(SystemExit) as e:
            main([str(source), "--root", str(root), "--out", str(tmp_path / "out")])
        assert e.value.code == 2
        captured = capsys.readouterr()
        assert (captured.out, captured.err) == ("", message)
