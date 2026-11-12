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

    def test_prepends_defs_directory_when_it_exists(self):
        with tempfile.TemporaryDirectory() as model_root:
            defs_path = os.path.join(model_root, "defs")
            os.makedirs(defs_path)

            inject_model_defs(model_root)

            self.assertEqual(sys.path[0], defs_path)

    def test_does_not_modify_sys_path_when_defs_missing(self):
        with tempfile.TemporaryDirectory() as model_root:
            before = sys.path.copy()
            inject_model_defs(model_root)
            self.assertEqual(sys.path, before)

    def test_inserts_at_front_preserving_order_of_existing_entries(self):
        with tempfile.TemporaryDirectory() as model_root:
            defs_path = os.path.join(model_root, "defs")
            os.makedirs(defs_path)
            first_before = sys.path[0]

            inject_model_defs(model_root)

            self.assertEqual(sys.path[0], defs_path)
            self.assertEqual(sys.path[1], first_before)
