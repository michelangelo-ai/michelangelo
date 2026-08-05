"""Tests for michelangelo.api.v2.util — generated object names."""

from __future__ import annotations

import re
from unittest import TestCase

from michelangelo.api.v2.util import generate_random_name

_NAME_RE = re.compile(r"^(?P<prefix>.+)-(?P<date>\d{8})-(?P<time>\d{6})-[0-9a-f]{8}$")


class TestGenerateRandomName(TestCase):
    """Tests for generate_random_name — format, uniqueness, and prefix validation."""

    def test_format_is_prefix_date_time_hex(self):
        """It returns '{prefix}-{YYYYMMDD}-{HHMMSS}-{8 hex}'."""
        match = _NAME_RE.match(generate_random_name("model"))
        self.assertIsNotNone(match)
        self.assertEqual(match.group("prefix"), "model")

    def test_successive_calls_are_unique(self):
        """Two calls in the same second still differ, via the random suffix."""
        self.assertNotEqual(
            generate_random_name("model"), generate_random_name("model")
        )

    def test_names_sort_chronologically(self):
        """The timestamp leads the random part, so plain string sort is time order."""
        names = sorted(
            [
                "model-20260721-114130-ffffffff",
                "model-20260721-114129-00000000",
                "model-20260720-235959-88888888",
            ]
        )
        self.assertEqual(
            names,
            [
                "model-20260720-235959-88888888",
                "model-20260721-114129-00000000",
                "model-20260721-114130-ffffffff",
            ],
        )

    def test_prefix_is_lowercased(self):
        """Uppercase prefixes are normalized, per Kubernetes naming rules."""
        self.assertTrue(generate_random_name("MyModel").startswith("mymodel-"))

    def test_underscores_in_prefix_become_hyphens(self):
        """Underscores are not valid in Kubernetes names and are replaced."""
        self.assertTrue(generate_random_name("my_model").startswith("my-model-"))

    def test_empty_prefix_raises(self):
        """An empty prefix is rejected rather than yielding a leading-hyphen name."""
        with self.assertRaises(RuntimeError):
            generate_random_name("")

    def test_overlong_prefix_raises(self):
        """A prefix over 128 characters is rejected."""
        with self.assertRaises(RuntimeError):
            generate_random_name("x" * 129)

    def test_prefix_at_length_limit_is_accepted(self):
        """128 characters is the inclusive upper bound."""
        self.assertTrue(generate_random_name("x" * 128).startswith("x" * 128 + "-"))
