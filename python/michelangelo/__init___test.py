"""Tests for the michelangelo package's namespace-package declaration.

`michelangelo/__init__.py` calls :func:`pkgutil.extend_path` so that contents
from multiple ``sys.path`` entries merge into one logical package. This is
needed by bazel / PEX consumers that bundle the wheel alongside separately
generated proto stubs.
"""

from __future__ import annotations

import importlib
import sys
import tempfile
from pathlib import Path
from unittest import TestCase


class NamespacePackageTest(TestCase):
    """Verify michelangelo behaves as a shared-namespace package."""

    def test_import_succeeds(self):
        """Smoke test: the package still imports cleanly after the change."""
        import michelangelo

        self.assertTrue(hasattr(michelangelo, "__path__"))
        self.assertTrue(hasattr(michelangelo, "__file__"))

    def test_path_is_a_list_of_strings(self):
        """`__path__` is the standard list-of-paths shape after extend_path."""
        import michelangelo

        paths = list(michelangelo.__path__)
        self.assertGreaterEqual(len(paths), 1)
        for p in paths:
            self.assertIsInstance(p, str)

    def test_extends_across_sys_path(self):
        """Confirm cross-sys.path merge — the structural win.

        Drop a fake ``michelangelo/extra.py`` on a second sys.path entry and
        confirm it becomes importable as ``michelangelo.extra``.
        """
        with tempfile.TemporaryDirectory() as tmpdir:
            fake_pkg_dir = Path(tmpdir) / "michelangelo"
            fake_pkg_dir.mkdir()
            (fake_pkg_dir / "extra_for_namespace_test.py").write_text(
                "VALUE = 'merged'"
            )

            sys.path.insert(0, tmpdir)
            try:
                # Force a re-read of michelangelo.__path__ so extend_path picks up
                # the new sys.path entry on this import.
                import michelangelo

                importlib.reload(michelangelo)
                self.assertIn(str(fake_pkg_dir), list(michelangelo.__path__))

                from michelangelo import (
                    extra_for_namespace_test,  # type: ignore[attr-defined]
                )

                self.assertEqual(extra_for_namespace_test.VALUE, "merged")
            finally:
                sys.path.remove(tmpdir)
                sys.modules.pop("michelangelo.extra_for_namespace_test", None)
                # Reload one more time to drop the fake path from __path__.
                import michelangelo

                importlib.reload(michelangelo)
