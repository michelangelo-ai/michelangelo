"""Tests for michelangelo.lib.shared.utils.model_metadata.e2e_sample_data."""

import unittest

from michelangelo.lib.shared.utils.model_metadata.e2e_sample_data import build_e2e_sample_data


class BuildE2eSampleDataTest(unittest.TestCase):
    def test_merges_model_and_feature_samples_and_filters_to_e2e_inputs(self):
        result = build_e2e_sample_data(
            feature_sample_data=[{"raw_a": 1.0}, {"raw_b": 2}],
            model_sample_data=[{"raw_b": 99, "model_only": 3.0}],
            e2e_input_cols={"raw_a", "raw_b"},
        )

        self.assertEqual(result, [{"raw_a": 1.0, "raw_b": 2}])

    def test_feature_sample_data_wins_on_duplicate_key(self):
        result = build_e2e_sample_data(
            feature_sample_data=[{"x": "raw"}],
            model_sample_data=[{"x": "derived"}],
            e2e_input_cols={"x"},
        )

        self.assertEqual(result, [{"x": "raw"}])

    def test_none_samples_return_single_empty_batch(self):
        self.assertEqual(build_e2e_sample_data(None, None, {"unused"}), [{}])
