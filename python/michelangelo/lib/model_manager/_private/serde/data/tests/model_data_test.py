import os
import tempfile
from unittest import TestCase

import numpy as np

from michelangelo.lib.model_manager._private.serde.data import (
    dump_model_data,
    get_model_data,
    load_model_data,
)


class ModelDataTest(TestCase):
    """Tests for model data serialization and deserialization."""

    def setUp(self):
        """Set up the test fixtures."""
        self.record = {
            "feature1": np.array([1, 2, 3]),
            "feature2": np.array(["a", "b", "c"]),
        }
        self.json_record = r'{"feature1": [1, 2, 3], "feature2": ["a", "b", "c"]}'
        self.data = [
            self.record,
            {
                "feature1": np.array([4, 5, 6]),
                "feature2": np.array(["d", "e", "f"]),
            },
        ]
        self.json_data = (
            r'[{"feature1": [1, 2, 3], "feature2": ["a", "b", "c"]}, '
            r'{"feature1": [4, 5, 6], "feature2": ["d", "e", "f"]}]'
        )

    def assert_record_equal(self, record1, record2):
        """Assert that two records are equal."""
        self.assertEqual(set(record1.keys()), set(record2.keys()))
        for key in record1:
            self.assertTrue(np.array_equal(record1[key], record2[key]))

    def assert_data_equal(self, data1, data2):
        """Assert that two data are equal."""
        self.assertEqual(len(data1), len(data2))
        for record1, record2 in zip(data1, data2):
            self.assert_record_equal(record1, record2)

    def test_dump_model_data_single_record(self):
        """Test that the model data is dumped correctly."""
        self.assertEqual(dump_model_data(self.record), self.json_record)

    def test_get_model_data_single_record(self):
        """Test that the model data is loaded correctly."""
        model_data = get_model_data(self.json_record)
        self.assert_record_equal(model_data, self.record)

    def test_dump_model_data_single_record_to_file(self):
        """Test that the model data is dumped to a file correctly."""
        with tempfile.TemporaryDirectory() as temp_dir:
            file = os.path.join(temp_dir, "data.json")
            with open(file, "w") as f:
                dump_model_data(self.record, f)

            with open(file) as f:
                content = f.read()
                self.assertEqual(content, self.json_record)

    def test_load_model_data_single_record(self):
        """Test that the model data is loaded from a file correctly."""
        with tempfile.TemporaryDirectory() as temp_dir:
            file = os.path.join(temp_dir, "data.json")
            with open(file, "w") as f:
                dump_model_data(self.record, f)

            with open(file) as f:
                model_data = load_model_data(f)
                self.assert_record_equal(model_data, self.record)

    def test_dump_model_data_multiple_records(self):
        """Test that the model data is dumped correctly."""
        self.assertEqual(dump_model_data(self.data), self.json_data)

    def test_get_model_data_multiple_records(self):
        """Test that the model data is loaded correctly."""
        model_data = get_model_data(self.json_data)
        self.assert_data_equal(model_data, self.data)

    def test_dump_model_data_multiple_records_to_file(self):
        """Test that the model data is dumped to a file correctly."""
        with tempfile.TemporaryDirectory() as temp_dir:
            file = os.path.join(temp_dir, "data.json")
            with open(file, "w") as f:
                dump_model_data(self.data, f)

            with open(file) as f:
                content = f.read()
                self.assertEqual(content, self.json_data)

    def test_load_model_data_multiple_records(self):
        """Test that the model data is loaded from a file correctly."""
        with tempfile.TemporaryDirectory() as temp_dir:
            file = os.path.join(temp_dir, "data.json")
            with open(file, "w") as f:
                dump_model_data(self.data, f)

            with open(file) as f:
                model_data = load_model_data(f)
                self.assert_data_equal(model_data, self.data)
