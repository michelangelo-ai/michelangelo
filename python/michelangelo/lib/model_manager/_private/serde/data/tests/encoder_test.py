"""Tests for the data encoder."""

import json
import numpy as np
from unittest import TestCase
from michelangelo.lib.model_manager._private.serde.data import DataEncoder


class EncoderTest(TestCase):
    """Tests for the data encoder."""

    def test_encoder(self):
        """Test that the data is encoded correctly."""
        data = {
            "feature1": np.array([1, 2, 3]),
            "feature2": [4, 5, 6],
            "feature3": b"test",
            "feature4": "test",
        }

        encoded_data = json.dumps(data, cls=DataEncoder)

        self.assertEqual(
            encoded_data,
            '{"feature1": [1, 2, 3], "feature2": [4, 5, 6], "feature3": "test", "feature4": "test"}',
        )

    def test_encoder_with_error(self):
        """Test that the data is encoded correctly."""

        class Test:
            pass

        data = {"feature": Test()}

        with self.assertRaises(TypeError):
            json.dumps(data, cls=DataEncoder)
