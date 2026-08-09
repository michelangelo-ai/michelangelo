import os
import subprocess
import sys
import tempfile
import textwrap
import unittest
from dataclasses import dataclass, is_dataclass
from pathlib import Path
from typing import Any, Optional
from unittest.mock import MagicMock, patch

import fsspec
import fsspec.core

from michelangelo.uniflow.core.utils import (
    dataclass_dict,
    encode_value_to_json,
    is_dataclass_instance,
)

pwd = os.environ["PWD"]


class Test(unittest.TestCase):
    def test_fsspec_url_to_fs_no_scheme(self):
        _, absolute_path = fsspec.core.url_to_fs("/host/path/data.json")
        _, relative_path = fsspec.core.url_to_fs("host/path/data.json")

        self.assertEqual("/host/path/data.json", absolute_path)

        expected_relative_path = Path(pwd) / "host" / "path" / "data.json"
        self.assertEqual(str(expected_relative_path), relative_path)

    def test_fsspec_url_to_fs_file(self):
        _, absolute_path = fsspec.core.url_to_fs("file:///host/path/data.json")
        _, relative_path = fsspec.core.url_to_fs("file://host/path/data.json")

        self.assertEqual("/host/path/data.json", absolute_path)

        expected_relative_path = Path(pwd) / "host" / "path" / "data.json"
        self.assertEqual(str(expected_relative_path), relative_path)

    def test_fsspec_url_to_fs_memory(self):
        # memory - absolute only path
        _, path1 = fsspec.core.url_to_fs("memory:///host/path/data.json")
        _, path2 = fsspec.core.url_to_fs("memory://host/path/data.json")

        expected_path = "/host/path/data.json"
        self.assertEqual(expected_path, path1)
        self.assertEqual(expected_path, path2)

    def test_encode_value_to_json(self):
        # Mocking tempfile.NamedTemporaryFile
        mock_temp_file = MagicMock()
        mock_file = MagicMock()
        mock_temp_file.__enter__.return_value = mock_file
        with patch("tempfile.NamedTemporaryFile", mock_temp_file):
            encode_value_to_json("test_value")


@dataclass
class Resource:  # basic dataclass
    index: int
    path: str
    metadata: Optional[Any] = None


class DataclassTestCase(unittest.TestCase):
    def test_dataclass_dict_required_only_attrs(self):
        # Init resource with required only attributes
        resource = Resource(
            index=101,
            path="/resources/101",
        )
        dct = dataclass_dict(resource)
        expected = {
            "index": 101,
            "path": "/resources/101",
            "metadata": None,
        }
        self.assertEqual(expected, dct)

    def test_dataclass_dict_non_recursive(self):
        # Init resource with another inner resource in its metadata. The inner resource must not be converted to
        # dictionary because dataclass_dict supposed to be non-recursive.
        resource = Resource(
            index=101,
            path="/resources/101",
            metadata=Resource(
                index=1,
                path="/resources/1",
            ),
        )
        dct = dataclass_dict(resource)
        expected = {
            "index": 101,
            "path": "/resources/101",
            "metadata": Resource(
                index=1,
                path="/resources/1",
            ),
        }
        self.assertEqual(expected, dct)

    def test_is_dataclass_instance(self):
        instance = Resource(index=0, path="")
        self.assertTrue(is_dataclass_instance(instance))
        self.assertFalse(
            is_dataclass_instance(Resource)
        )  # Resource is not a dataclass type, not an instance

        # Standard Python dataclass.is_dataclass returns true for both type and instance.
        self.assertTrue(is_dataclass(instance))
        self.assertTrue(is_dataclass(Resource))


class DotPathMainTestCase(unittest.TestCase):
    """Regression tests for dot_path()'s `__main__` special-case.

    A function's __module__ is only ever bound to the literal string
    "__main__" when the enclosing module is actually executed as the
    program entry point -- a plain `import` of the module (as the rest of
    this test file does) never reproduces that binding. These tests
    therefore spawn a real `python -m pkg.module` subprocess, which is
    the only way to genuinely exercise the buggy/fixed code path.
    """

    def test_dot_path_python_dash_m_with_dotted_file_path(self):
        """`python -m pkg.module` must not crash on a dotted file path.

        __file__ may transit a directory segment containing a literal "."
        -- e.g. a virtualenv's own `lib/python3.11/site-packages/...`
        layout for any pip-installed package. Regression test for GH-1753.
        """
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp_dir_path = Path(tmp_dir).resolve()

            # Simulate a venv-style path segment with a literal "." in it
            # (e.g. "python3.11") sitting between the run's cwd and the
            # module's real __file__. The dotted segment only needs to be
            # part of a sys.path entry -- it must NOT be part of the
            # dotted package name itself, since directories added to
            # sys.path are not part of the import path.
            site_packages = tmp_dir_path / "lib.python3.11" / "site-packages"
            pkg_dir = site_packages / "fakepkg"
            pkg_dir.mkdir(parents=True)
            (pkg_dir / "__init__.py").write_text("")
            (pkg_dir / "mod.py").write_text(
                textwrap.dedent(
                    """
                    from michelangelo.uniflow.core.utils import dot_path


                    def foo():
                        pass


                    print(dot_path(foo))
                    """
                )
            )

            env = dict(os.environ)
            existing_pythonpath = env.get("PYTHONPATH", "")
            env["PYTHONPATH"] = os.pathsep.join(
                p for p in [str(site_packages), existing_pythonpath] if p
            )

            result = subprocess.run(
                [sys.executable, "-m", "fakepkg.mod"],
                cwd=str(tmp_dir_path),
                env=env,
                capture_output=True,
                text=True,
            )

            self.assertEqual(
                0,
                result.returncode,
                msg=f"stdout={result.stdout!r} stderr={result.stderr!r}",
            )
            self.assertEqual("fakepkg.mod.foo", result.stdout.strip())

    def test_dot_path_plain_script_invocation_still_uses_fallback(self):
        """Plain `python script.py` invocation must still use the fallback.

        `python script.py` (no `-m`) has no import spec (__spec__ is None),
        so dot_path() must still fall back to the pre-existing
        relative-path reconstruction. Confirms the fix doesn't regress this
        already-working invocation style.
        """
        with tempfile.TemporaryDirectory() as tmp_dir:
            tmp_dir_path = Path(tmp_dir).resolve()
            script_dir = tmp_dir_path / "scripts"
            script_dir.mkdir()
            script_path = script_dir / "plain_script.py"
            script_path.write_text(
                textwrap.dedent(
                    """
                    from michelangelo.uniflow.core.utils import dot_path


                    def foo():
                        pass


                    print(dot_path(foo))
                    """
                )
            )

            result = subprocess.run(
                [sys.executable, str(script_path)],
                cwd=str(tmp_dir_path),
                capture_output=True,
                text=True,
            )

            self.assertEqual(
                0,
                result.returncode,
                msg=f"stdout={result.stdout!r} stderr={result.stderr!r}",
            )
            self.assertEqual("scripts.plain_script.foo", result.stdout.strip())
