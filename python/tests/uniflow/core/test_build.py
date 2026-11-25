"""Tests for building uniflow workflow packages."""

import ast
import gzip
import io
import json
import shutil
import unittest
from pathlib import Path

import pytest

import tests.uniflow.core.demo_app.demo_app as demo_app
import tests.uniflow.core.demo_app.overrides_app as overrides_app
import tests.uniflow.core.demo_platform.workflows as demo_platform_workflows
from michelangelo.uniflow.core.build import (
    TranspilerCallback,
    _add_star_file,
    build,
    build_star,
)


class Test(unittest.TestCase):
    """Tests for build()."""

    def test_demo_app(self):
        """build() packages the workflow and its dependencies."""
        package = build(demo_app.main)

        # Find and assert the main file
        main_file = [
            p
            for p in package.files
            if p.endswith("/tests/uniflow/core/demo_app/demo_app.py")
        ]
        self.assertEqual(1, len(main_file))
        self.assertEqual(main_file[0], package.main_file)
        self.assertEqual("main", package.main_function)

        # Assert file paths in the package
        path_set = set()
        for path, ast_module in package.files.items():
            self.assertIsInstance(ast_module, bytes)
            if path != "meta.json":
                _, path = path.split("/tests/uniflow/core/")
            path_set.add(path)

        expected_path_set = {
            "demo_app/demo_app.py",
            "demo_platform/workflows.py",
            "demo_platform/test_conf/task_a.star",
            "demo_platform/test_conf/task_b.star",
            "demo_platform/test_conf/commons.star",
            "meta.json",
        }
        self.assertSetEqual(expected_path_set, path_set)

        meta = json.loads(package.files["meta.json"])
        expected_meta = {
            "main_file": package.main_file,
            "main_function": package.main_function,
        }
        self.assertEqual(expected_meta, meta)

    def test_build_with_transpiler_callback(self):
        """build() reports each task function to the transpiler callback."""

        class MyTranspilerCallback(TranspilerCallback):
            def __init__(self):
                self.task_functions = []

            def on_task_function(self, task_fn):
                self.task_functions.append(task_fn)

        transpiler_callback = MyTranspilerCallback()
        package0 = build(demo_app.main, transpiler_callback=transpiler_callback)
        self.assertIsNotNone(package0)

        task_functions = transpiler_callback.task_functions

        self.assertEqual(len(task_functions), 3)

        t = task_functions[0]
        self.assertEqual(t, demo_app.task_1)
        self.assertEqual(t.image_spec.container_image, "test_image:test")

        t = task_functions[1]
        self.assertEqual(t, demo_app.task_wrapped)
        self.assertIsNone(t.image_spec)

        t = task_functions[2]
        self.assertEqual(t, demo_platform_workflows._greetings_task)

        package1 = build(demo_app.main)
        self.assertEqual(package0, package1)

    def test_build_keeps_module_level_overrides(self):
        """A module-level with_overrides() copy keeps its alias and retries."""
        task_functions = []

        class MyTranspilerCallback(TranspilerCallback):
            def on_task_function(self, task_fn):
                task_functions.append(task_fn)

        package = build(overrides_app.main, transpiler_callback=MyTranspilerCallback())

        self.assertEqual(len(task_functions), 2)
        self.assertIs(task_functions[0], overrides_app.task_slow)
        self.assertIs(task_functions[1], overrides_app.task_fast)

        main_source = package.files[package.main_file].decode()
        self.assertIn("alias='task_slow'", main_source)
        self.assertIn("alias='task_fast'", main_source)
        self.assertIn("retry_attempts=3", main_source)


def _write_tree(root: Path):
    (root / "a").mkdir(parents=True)
    (root / "lib").mkdir()
    (root / "a/main.star").write_text(
        'load("../lib/x.star", "x")\n\ndef main():\n    return x()\n'
    )
    (root / "lib/x.star").write_text(
        'load("../lib/y.star", "y")\n\ndef x():\n    return y()\n'
    )
    (root / "lib/y.star").write_text("def y():\n    return 1\n")


def _load_modules(source: bytes) -> list:
    return [
        node.value.args[0].value
        for node in ast.parse(source).body
        if isinstance(node, ast.Expr) and node.value.func.id == "load"
    ]


def test_build_star_relative_names(tmp_path):
    """build_star names files and loads relative to the root."""
    _write_tree(tmp_path)
    pkg = build_star(tmp_path / "a/main.star", root=tmp_path)
    assert sorted(pkg.files) == [
        "/a/main.star",
        "/lib/x.star",
        "/lib/y.star",
        "meta.json",
    ]
    assert "load('/lib/x.star', 'x')" in pkg.files["/a/main.star"].decode()
    assert _load_modules(pkg.files["/lib/x.star"]) == ["/lib/y.star"]
    assert json.loads(pkg.files["meta.json"]) == {
        "main_file": "/a/main.star",
        "main_function": "main",
    }


def test_build_star_meta_json(tmp_path):
    """meta.json is compact JSON, and a quote in a file name stays valid."""
    _write_tree(tmp_path)
    pkg = build_star(tmp_path / "a/main.star", root=tmp_path)
    assert pkg.files["meta.json"] == (
        b'{"main_file":"/a/main.star","main_function":"main"}'
    )

    (tmp_path / 'a/ma"in.star').write_text("def main():\n    return 1\n")
    pkg = build_star(tmp_path / 'a/ma"in.star', root=tmp_path)
    assert pkg.files["meta.json"] == (
        b'{"main_file":"/a/ma\\"in.star","main_function":"main"}'
    )


def test_build_star_rejects_load_outside_root(tmp_path):
    """A load that leaves the root raises ValueError naming the path."""
    root = tmp_path / "root"
    (root / "a").mkdir(parents=True)
    (root / "a/main.star").write_text('load("../../outside.star", "o")\n')
    (tmp_path / "outside.star").write_text("def o():\n    return 1\n")
    with pytest.raises(ValueError, match="outside.star"):
        build_star(root / "a/main.star", root=root)


def test_build_star_two_roots_same_bytes(tmp_path):
    """The same tree under two roots gives the same tarball bytes."""
    _write_tree(tmp_path / "one")
    shutil.copytree(tmp_path / "one", tmp_path / "two")
    out = []
    for name in ("one", "two"):
        buf = io.BytesIO()
        build_star(tmp_path / name / "a/main.star", root=tmp_path / name).to_tarball(
            buf, gzip_mtime=0
        )
        out.append(buf.getvalue())
    assert out[0] == out[1]


def test_to_tarball_default_unchanged(tmp_path):
    """Without gzip_mtime the gzip header keeps a timestamp, same inner tar."""
    _write_tree(tmp_path)
    pkg = build_star(tmp_path / "a/main.star", root=tmp_path)
    default, pinned = io.BytesIO(), io.BytesIO()
    pkg.to_tarball(default)
    pkg.to_tarball(pinned, gzip_mtime=0)
    assert int.from_bytes(default.getvalue()[4:8], "little") != 0
    assert int.from_bytes(pinned.getvalue()[4:8], "little") == 0
    assert gzip.decompress(default.getvalue()) == gzip.decompress(pinned.getvalue())


def test_add_star_file_without_root_unchanged(tmp_path):
    """Without root, load constants hold absolute filesystem paths."""
    _write_tree(tmp_path)
    main = (tmp_path / "a/main.star").resolve()
    files = {}
    _add_star_file(main, files)
    lib = main.parent.parent / "lib"
    assert sorted(files) == [main, lib / "x.star", lib / "y.star"]
    assert _load_modules(ast.unparse(files[main])) == [(lib / "x.star").as_posix()]
    assert _load_modules(ast.unparse(files[lib / "x.star"])) == [
        (lib / "y.star").as_posix()
    ]
