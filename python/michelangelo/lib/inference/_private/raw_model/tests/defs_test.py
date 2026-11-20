"""Tests for locating and injecting raw model defs."""

from __future__ import annotations

import os
import sys
import tempfile
import unittest

from michelangelo.lib.inference._private.raw_model.defs import (
    get_model_defs_path,
    inject_model_defs,
)


class GetModelDefsPathTest(unittest.TestCase):
    def test_returns_joined_path(self):
        base = os.path.join("var", "models", "m1")
        self.assertEqual(get_model_defs_path(base), os.path.join(base, "defs"))


class InjectModelDefsTest(unittest.TestCase):
    def setUp(self):
        self._orig_sys_path = sys.path.copy()

    def tearDown(self):
        sys.path[:] = self._orig_sys_path

    def test_appends_defs_directory_when_it_exists(self):
        """defs/ goes last, so it never shadows installed modules."""
        with tempfile.TemporaryDirectory() as model_root:
            defs_path = os.path.join(model_root, "defs")
            os.makedirs(defs_path)
            before = sys.path.copy()

            inject_model_defs(model_root)

            self.assertEqual(sys.path, [*before, os.path.abspath(defs_path)])

    def test_does_not_modify_sys_path_when_defs_missing(self):
        with tempfile.TemporaryDirectory() as model_root:
            before = sys.path.copy()
            inject_model_defs(model_root)
            self.assertEqual(sys.path, before)

    def test_loading_the_same_package_twice_adds_one_entry(self):
        """A reloaded package does not grow sys.path."""
        with tempfile.TemporaryDirectory() as model_root:
            os.makedirs(os.path.join(model_root, "defs"))

            inject_model_defs(model_root)
            inject_model_defs(model_root)

            defs_path = os.path.abspath(os.path.join(model_root, "defs"))
            self.assertEqual(sys.path.count(defs_path), 1)
