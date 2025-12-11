"""Tests for Placeholder constants."""

from unittest import TestCase

from michelangelo.lib.model_manager._private.constants import Placeholder


class PlaceholderTest(TestCase):
    """Tests for Placeholder constants."""

    def test_placeholder(self):
        """It exposes the expected placeholder for model name."""
        self.assertEqual(Placeholder.MODEL_NAME, "$MODEL_NAME")
