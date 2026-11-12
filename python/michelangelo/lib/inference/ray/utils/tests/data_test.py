"""Tests for the Ray dataset helpers."""

from __future__ import annotations

from unittest import TestCase

import ray

from michelangelo.lib.inference.ray.utils.data import merge_ray_datasets


class DataTest(TestCase):
    """Tests for ``merge_ray_datasets``."""

    def setUp(self):
        """Setup."""
        super().setUp()
        if not ray.is_initialized():
            ray.init(num_cpus=2)

    def tearDown(self):
        """Teardown."""
        ray.shutdown()

    def test_merge_ray_datasets(self):
        """Check merge ray datasets."""
        # test merge 2 datasets
        data_1 = [
            {"model_name": "1", "score": 95},
            {"model_name": "1", "score": 85},
        ]
        ray_data_1 = ray.data.from_items(data_1)

        data_2 = [
            {"model_name": "2", "score": 90},
            {"model_name": "2", "score": 80},
        ]
        ray_data_2 = ray.data.from_items(data_2)

        result = merge_ray_datasets([ray_data_1, ray_data_2])

        self.assertEqual(result.count(), 4)

    def test_merge_ray_datasets_one_dataset(self):
        """Check merge ray datasets one dataset."""
        # test merge 1
        data_3 = [
            {"model_name": "1", "score": 95},
            {"model_name": "1", "score": 85},
            {"model_name": "1", "score": 71},
        ]
        ray_data_3 = ray.data.from_items(data_3)
        result_2 = merge_ray_datasets([ray_data_3])

        self.assertEqual(result_2.count(), 3)

    def test_merge_ray_datasets_with_empty_list(self):
        """Check merge ray datasets with empty list."""
        # test merge none
        result_3 = merge_ray_datasets([])
        self.assertIsNone(result_3)
