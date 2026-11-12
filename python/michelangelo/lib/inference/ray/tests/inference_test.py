"""Tests for Ray batch inference."""

from __future__ import annotations

import os
import tempfile
import unittest
from pathlib import Path
from unittest import TestCase
from unittest.mock import MagicMock, patch

import numpy as np
import pyarrow as pa
import ray
import torch

from michelangelo.lib.inference.lib.model_inference import ModelInference
from michelangelo.lib.inference.ray.inference import (
    DEFAULT_ACTOR_MAX_RESTARTS,
    DEFAULT_ACTOR_MAX_TASK_RETRIES,
    BatchOptions,
    InferenceConfig,
    ModelIdentifier,
    OutputColumnMetadata,
    PaddingOptions,
    Predictor,
    _download_model_package,
    _run_predictor_inference,
    batch_model_inference,
)
from michelangelo.lib.inference.ray.tests.fixtures.predict import (
    LocalDirStorageBackend,
    LocalTarStorageBackend,
)
from michelangelo.lib.model_manager.constants import RawModelType
from michelangelo.lib.model_manager.packager.custom_triton.custom_triton_packager import (  # noqa: E501
    CustomTritonPackager,
)
from michelangelo.lib.model_manager.packager.torch_triton.torch_triton_packager import (
    TorchTritonPackager,
)
from michelangelo.lib.model_manager.schema import (
    DataType,
    ModelSchema,
    ModelSchemaItem,
)

_FIXTURES = "michelangelo.lib.inference.ray.tests.fixtures"
_PREDICT = f"{_FIXTURES}.predict"
_MODULE = "michelangelo.lib.inference.ray.inference"


class MockPredictor:
    """Stands in for ``Predictor`` so no model is loaded."""

    def __init__(
        self,
        input_columns: list[str] | None = None,
        model_name: str | None = None,
        model_path: str | None = None,
        model_type: str | None = None,
        storage_backend=None,
        column_padding_values: dict | None = None,
        columns_to_keep: list[str] | None = None,
        num_cpus: int | None = None,
    ):
        """Initialize the instance."""
        self.inference = ModelInference("/unused")
        self.input_columns = input_columns
        self.model_name = model_name
        self.model_path = model_path
        self.model_type = model_type
        self.storage_backend = storage_backend
        self.column_padding_values = column_padding_values
        self.columns_to_keep = columns_to_keep
        self.num_cpus = num_cpus

    def __call__(self, _) -> pa.Table:
        """Call  ."""
        final_res = {
            "input": np.array([b"a"]),
            "response": np.array([b"x"]),
            "model_name": np.array([self.model_name]),
        }
        return self._numpy_dict_to_pyarrow_table(final_res)

    def _numpy_dict_to_pyarrow_table(self, numpy_dict):
        arrays = {k: pa.array(arr.tolist()) for k, arr in numpy_dict.items()}
        return pa.table(arrays)


class FakeSchema:
    """Test helper."""

    def __init__(self, names):
        """Initialize the instance."""
        self.names = names


class FakeDataset:
    """Test helper."""

    def __init__(self):
        """Initialize the instance."""
        self.map_batches_calls = []

    def limit(self, _n):
        """Limit."""
        return self

    def take(self, _n):
        """Take."""
        return [{"user_id": "u1"}]

    def map_batches(self, *args, **kwargs):
        """Map batches."""
        self.map_batches_calls.append((args, kwargs))
        return self

    def schema(self):
        """Schema."""
        return FakeSchema(["user_id", "response"])


def _config(batch_options=None, **kwargs):
    return [
        InferenceConfig(
            model=ModelIdentifier(model_path="/tmp/model", model_type="torch"),
            input_columns=["feature"],
            batch_options=batch_options,
            **kwargs,
        )
    ]


class InferenceTest(TestCase):
    """Tests for Inference."""

    def setUp(self):
        """Setup."""
        if not ray.is_initialized():
            ray.init(num_cpus=2)

    def tearDown(self):
        """Teardown."""
        ray.shutdown()

    @patch(f"{_MODULE}.merge_ray_datasets")
    def test_batch_options_pass_ray_actor_controls_to_map_batches(
        self, mock_merge_ray_datasets
    ):
        """Check batch options pass ray actor controls to map batches."""
        dataset = FakeDataset()
        mock_merge_ray_datasets.return_value = dataset
        config = _config(
            BatchOptions(
                batch_size=8192,
                num_gpus=1,
                concurrency=8,
                num_cpus=8,
                max_concurrency=8,
                max_tasks_in_flight_per_actor=16,
                max_restarts=5,
                max_task_retries=2,
            ),
            columns_to_keep=["user_id"],
        )

        batch_model_inference(config, {"dataset": dataset})

        self.assertEqual(len(dataset.map_batches_calls), 2)
        for _, kwargs in dataset.map_batches_calls:
            self.assertEqual(kwargs["batch_size"], 8192)
            self.assertEqual(kwargs["num_gpus"], 1)
            self.assertEqual(kwargs["num_cpus"], 8)
            self.assertEqual(kwargs["max_concurrency"], 8)
            self.assertEqual(kwargs["concurrency"], 8)
            self.assertEqual(kwargs["compute"].min_size, 8)
            self.assertEqual(kwargs["compute"].max_size, 8)
            self.assertEqual(kwargs["compute"].max_tasks_in_flight_per_actor, 16)
            self.assertEqual(kwargs["max_restarts"], 5)
            self.assertEqual(kwargs["max_task_retries"], 2)
            ctor = kwargs["fn_constructor_kwargs"]
            self.assertEqual(ctor["columns_to_keep"], ["user_id"])
            self.assertEqual(ctor["num_cpus"], 8)

    @patch(f"{_MODULE}.merge_ray_datasets")
    def test_unset_max_concurrency_is_omitted_from_map_batches(
        self, mock_merge_ray_datasets
    ):
        """Check unset max concurrency is omitted from map batches."""
        # `max_concurrency` flows into Ray's `**ray_remote_args`, where passing
        # `None` overrides the actor default of 1 and crashes
        # ActorPoolMapOperator. When unset it must be omitted entirely.
        dataset = FakeDataset()
        mock_merge_ray_datasets.return_value = dataset

        batch_model_inference(_config(BatchOptions(concurrency=4)), {"d": dataset})

        self.assertGreater(len(dataset.map_batches_calls), 0)
        for _, kwargs in dataset.map_batches_calls:
            self.assertNotIn("max_concurrency", kwargs)

    @patch(f"{_MODULE}.merge_ray_datasets")
    def test_max_tasks_in_flight_per_actor_without_concurrency_leaves_pool_unbounded(
        self, mock_merge_ray_datasets
    ):
        """Check max tasks in flight without concurrency leaves the pool unbounded."""
        dataset = FakeDataset()
        mock_merge_ray_datasets.return_value = dataset
        config = _config(BatchOptions(max_tasks_in_flight_per_actor=16))

        batch_model_inference(config, {"dataset": dataset})

        for _, kwargs in dataset.map_batches_calls:
            self.assertEqual(kwargs["compute"].min_size, 1)
            self.assertEqual(kwargs["compute"].max_size, float("inf"))
            self.assertEqual(kwargs["compute"].max_tasks_in_flight_per_actor, 16)

    @patch(f"{_MODULE}.merge_ray_datasets")
    def test_actor_fault_tolerance_defaults_applied_when_unset(
        self, mock_merge_ray_datasets
    ):
        """Check actor fault tolerance defaults applied when unset."""
        # A dead actor (transient GPU fault) must not fail the job by default:
        # the defaults apply when BatchOptions doesn't set them.
        dataset = FakeDataset()
        mock_merge_ray_datasets.return_value = dataset

        batch_model_inference(_config(BatchOptions(concurrency=4)), {"d": dataset})

        self.assertGreater(len(dataset.map_batches_calls), 0)
        for _, kwargs in dataset.map_batches_calls:
            self.assertEqual(kwargs["max_restarts"], DEFAULT_ACTOR_MAX_RESTARTS)
            self.assertEqual(kwargs["max_task_retries"], DEFAULT_ACTOR_MAX_TASK_RETRIES)

    @patch(f"{_MODULE}.merge_ray_datasets")
    def test_actor_fault_tolerance_zero_disables_restarts(
        self, mock_merge_ray_datasets
    ):
        """Check actor fault tolerance zero disables restarts."""
        # Explicit 0 must be honored, not replaced by the defaults.
        dataset = FakeDataset()
        mock_merge_ray_datasets.return_value = dataset
        config = _config(
            BatchOptions(concurrency=4, max_restarts=0, max_task_retries=0)
        )

        batch_model_inference(config, {"dataset": dataset})

        self.assertGreater(len(dataset.map_batches_calls), 0)
        for _, kwargs in dataset.map_batches_calls:
            self.assertEqual(kwargs["max_restarts"], 0)
            self.assertEqual(kwargs["max_task_retries"], 0)

    def test_no_batch_options_uses_defaults(self):
        """Check no batch options uses defaults."""
        dataset = FakeDataset()

        _run_predictor_inference(
            _config(padding_options=PaddingOptions({"feature": -1}))[0], dataset
        )

        _, kwargs = dataset.map_batches_calls[0]
        self.assertIsNone(kwargs["compute"])
        self.assertIsNone(kwargs["concurrency"])
        self.assertEqual(
            kwargs["fn_constructor_kwargs"]["column_padding_values"], {"feature": -1}
        )

    def test_storage_backend_is_forwarded_to_the_predictor(self):
        """Check storage backend is forwarded to the predictor."""
        dataset = FakeDataset()
        backend = MagicMock()

        _run_predictor_inference(_config()[0], dataset, backend)

        _, kwargs = dataset.map_batches_calls[0]
        self.assertIs(kwargs["fn_constructor_kwargs"]["storage_backend"], backend)

    @patch(f"{_MODULE}.Predictor", MockPredictor)
    def test_offline_inference(self):
        """Check offline inference."""
        config = [
            InferenceConfig(
                model=ModelIdentifier(name="test-model-1", revision_id=1),
                batch_options=BatchOptions(concurrency=1),
            ),
            InferenceConfig(
                model=ModelIdentifier(name="test-model-2", revision_id=1),
                batch_options=BatchOptions(batch_size=1000, concurrency=1),
            ),
        ]

        datasets = {
            "a": ray.data.from_items([{"input": b"a"}]),
            "b": ray.data.from_items([{"input": b"a"}]),
            "c": None,
        }

        results, output_column_metadata = batch_model_inference(config, datasets)
        output_a = results["a"].take_all()
        self.assertEqual(
            results["a"].schema().names, ["input", "response", "model_name"]
        )
        self.assertEqual(output_a[0]["response"], b"x")
        self.assertEqual(output_a[0]["input"], b"a")

        output_b = results["b"].take_all()
        self.assertEqual(
            results["b"].schema().names, ["input", "response", "model_name"]
        )
        self.assertEqual(output_b[0]["response"], b"x")
        self.assertEqual(output_b[0]["input"], b"a")

        self.assertIsNone(results["c"])

        self.assertEqual(len(output_column_metadata), 2)
        by_model = sorted(output_column_metadata, key=lambda x: x.model_name)
        self.assertEqual(
            by_model,
            [
                OutputColumnMetadata(column_name="response", model_name="test-model-1"),
                OutputColumnMetadata(column_name="response", model_name="test-model-2"),
            ],
        )

    def test_offline_inference_no_config(self):
        """Check offline inference no config."""
        config = []
        datasets = {
            "a": ray.data.from_items([{"input": b"a"}]),
        }

        results, output_column_metadata = batch_model_inference(config, datasets)
        self.assertEqual(datasets["a"], results["a"])
        self.assertEqual(output_column_metadata, [])

    @patch(f"{_MODULE}.Predictor", MockPredictor)
    def test_offline_inference_with_empty_dataset(self):
        """Check offline inference with empty dataset."""
        config = [
            InferenceConfig(
                model=ModelIdentifier(name="m"),
                batch_options=BatchOptions(concurrency=1),
            )
        ]
        empty = ray.data.from_items([{"input": b"a"}]).filter(lambda _: False)

        results, metadata = batch_model_inference(config, {"empty": empty})

        self.assertIs(results["empty"], empty)
        self.assertEqual(metadata, [])

    @patch(f"{_MODULE}.merge_ray_datasets", return_value=None)
    def test_offline_inference_falls_back_to_dataset_when_nothing_merged(self, _merge):
        """Check offline inference falls back to dataset when nothing merged."""
        dataset = FakeDataset()

        results, _ = batch_model_inference(_config(), {"d": dataset})

        self.assertIs(results["d"], dataset)

    def test_predictor(self):
        """Check predictor."""
        batch = pa.table({"input": ["a", "b"], "non_input": [1, 2]})

        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = create_model_package(temp_dir)
            predictor = Predictor(
                input_columns=["input"],
                model_name="test-model",
                model_path=raw_model_package,
            )
            result = predictor(batch)

        self.assertEqual(
            sorted(result.column_names),
            ["input", "model_name", "non_input", "response"],
        )
        self.assertEqual(result["input"].to_pylist(), ["a", "b"])
        self.assertEqual(result["non_input"].to_pylist(), [1, 2])
        self.assertEqual(result["response"].to_pylist(), ["a", "b"])
        self.assertEqual(result["model_name"].to_pylist(), ["test-model", "test-model"])

    def test_predictor_without_model_name_adds_no_metadata_column(self):
        """Check predictor without model name adds no metadata column."""
        with tempfile.TemporaryDirectory() as temp_dir:
            predictor = Predictor(model_path=create_model_package(temp_dir))
            result = predictor(pa.table({"input": ["a"]}))

        self.assertEqual(sorted(result.column_names), ["input", "response"])

    def test_predictor_requires_model_path(self):
        """Check predictor requires model path."""
        with self.assertRaisesRegex(ValueError, "model_path"):
            Predictor(input_columns=["input"])

    def test_predictor_with_threaded_conversion_matches_serial(self):
        """A num_cpus > 1 conversion thread pool must not change output."""
        batch = pa.table(
            {
                "input": ["a", "b", "c"],
                "extra_1": [1, 2, 3],
                "extra_2": [4.0, 5.0, 6.0],
                "non_input": [10, 20, 30],
            }
        )
        input_columns = ["input", "extra_1", "extra_2"]

        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = create_model_package(temp_dir)
            serial_predictor = Predictor(
                input_columns=input_columns,
                model_name="test-model",
                model_path=raw_model_package,
            )
            serial_result = serial_predictor(batch)

            threaded_predictor = Predictor(
                input_columns=input_columns,
                model_name="test-model",
                model_path=raw_model_package,
                num_cpus=4,
            )
            threaded_result = threaded_predictor(batch)

        self.assertIsNotNone(threaded_predictor._conversion_executor)
        self.assertIsNone(serial_predictor._conversion_executor)
        self.assertEqual(
            sorted(threaded_result.column_names), sorted(serial_result.column_names)
        )
        for col in serial_result.column_names:
            self.assertEqual(
                threaded_result[col].to_pylist(), serial_result[col].to_pylist()
            )

    def test_predictor_with_batch_inference(self):
        """Check predictor with batch inference."""
        batch = pa.table({"input": ["a", "b"], "non_input": [1, 2]})

        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = create_model_package(
                temp_dir, custom_batch_processing=True
            )
            predictor = Predictor(
                input_columns=["input"],
                model_name="test-model",
                model_path=raw_model_package,
            )
            result = predictor(batch)

        self.assertEqual(
            sorted(result.column_names),
            ["input", "model_name", "non_input", "response"],
        )
        self.assertEqual(result["input"].to_pylist(), ["a", "b"])
        self.assertEqual(result["non_input"].to_pylist(), [1, 2])
        self.assertEqual(result["response"].to_pylist(), ["a", "b"])
        self.assertEqual(result["model_name"].to_pylist(), ["test-model", "test-model"])

    def test_predictor_with_non_scalar_output(self):
        """Check predictor with non scalar output."""
        batch = pa.table({"input": ["a", "b"], "non_input": [1, 2]})

        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = create_model_package(
                temp_dir, custom_batch_processing=True, use_scalar=False
            )
            predictor = Predictor(
                input_columns=["input"],
                model_name="test-model",
                model_path=raw_model_package,
            )
            result = predictor(batch)

        self.assertEqual(
            sorted(result.column_names),
            ["input", "model_name", "non_input", "response"],
        )
        self.assertEqual(result["response"].to_pylist(), [["a"], ["b"]])

    def test_predictor_with_multi_dim_input(self):
        """Check predictor with multi dim input."""
        batch = pa.table(
            {
                "input": ["a", "b"],
                "input_1": [[1, 2], [3, 4]],
                "input_2": [[[1, 2], [3, 4]], [[5, 6], [7, 8]]],
                "non_input": [1, 2],
                "non_input_1": [[1, 2], [3, 4]],
                "non_input_2": [[[1, 2], [3, 4]], [[5, 6], [7, 8]]],
                "non_input_3": [[["a", "b"], ["c", "d"]], [["e", "f"], ["g", "h"]]],
            }
        )

        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = create_model_package(
                temp_dir, custom_batch_processing=True, with_multi_dim_feature=True
            )
            predictor = Predictor(
                input_columns=["input", "input_1", "input_2"],
                model_name="test-model",
                model_path=raw_model_package,
            )
            result = predictor(batch)

        self.assertEqual(
            sorted(result.column_names),
            [
                "input",
                "input_1",
                "input_2",
                "model_name",
                "non_input",
                "non_input_1",
                "non_input_2",
                "non_input_3",
                "response",
                "response_1",
                "response_2",
            ],
        )
        self.assertEqual(result["input"].to_pylist(), ["a", "b"])
        self.assertEqual(result["input_1"].to_pylist(), [[1, 2], [3, 4]])
        self.assertEqual(
            result["input_2"].to_pylist(), [[[1, 2], [3, 4]], [[5, 6], [7, 8]]]
        )
        self.assertEqual(result["response"].to_pylist(), ["a", "b"])
        self.assertEqual(result["response_1"].to_pylist(), [[1, 2], [3, 4]])
        self.assertEqual(
            result["response_2"].to_pylist(), [[[1, 2], [3, 4]], [[5, 6], [7, 8]]]
        )
        self.assertEqual(result["model_name"].to_pylist(), ["test-model", "test-model"])

    def test_predictor_with_model_path(self):
        """Check predictor with model path."""
        batch = pa.table({"input": ["a", "b"], "non_input": [1, 2]})

        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = create_model_package(
                temp_dir, custom_batch_processing=True
            )
            predictor = Predictor(input_columns=["input"], model_path=raw_model_package)
            result = predictor(batch)

        self.assertEqual(
            sorted(result.column_names), ["input", "non_input", "response"]
        )
        self.assertEqual(result["response"].to_pylist(), ["a", "b"])

    def test_predictor_downloads_a_directory_package(self):
        """Check predictor downloads a directory package."""
        batch = pa.table({"input": ["a", "b"]})

        with tempfile.TemporaryDirectory() as temp_dir:
            package = create_model_package(temp_dir, custom_batch_processing=True)
            predictor = Predictor(
                model_path="s3://bucket/models/m",
                model_type=RawModelType.CUSTOM_PYTHON,
                storage_backend=LocalDirStorageBackend(package),
            )
            result = predictor(batch)

        self.assertNotEqual(predictor.inference.model_path, "s3://bucket/models/m")
        self.assertEqual(result["response"].to_pylist(), ["a", "b"])

    def test_predictor_extracts_a_single_tar_package(self):
        """Check predictor extracts a single tar package."""
        batch = pa.table({"input": ["a", "b"]})

        with tempfile.TemporaryDirectory() as temp_dir:
            package = create_model_package(temp_dir, custom_batch_processing=True)
            backend = LocalTarStorageBackend(
                package, os.path.join(temp_dir, "model.tar")
            )
            predictor = Predictor(
                model_path="s3://bucket/models/m/model.tar",
                storage_backend=backend,
            )
            result = predictor(batch)

        self.assertTrue(os.path.isdir(predictor.inference.model_path))
        self.assertEqual(result["response"].to_pylist(), ["a", "b"])

    def test_download_model_package_returns_directory_result_as_is(self):
        """Check download model package returns directory result as is."""
        with tempfile.TemporaryDirectory() as temp_dir:
            package = create_model_package(temp_dir)
            local = _download_model_package(LocalDirStorageBackend(package), "uri")
        self.assertTrue(local.endswith("package"))

    def test_predictor_uses_model_path_directly_without_storage_backend(self):
        """Check predictor uses model path directly without storage backend."""
        with tempfile.TemporaryDirectory() as temp_dir:
            package = create_model_package(temp_dir)
            predictor = Predictor(model_path=package)
        self.assertEqual(predictor.inference.model_path, package)

    def test_get_input_columns_infers_from_model_schema_when_not_provided(self):
        """When input_columns is not provided, they come from the model schema."""
        batch = pa.table({"input": ["a", "b"], "non_input": [1, 2]})
        with tempfile.TemporaryDirectory() as temp_dir:
            raw_model_package = create_model_package(
                temp_dir, custom_batch_processing=True
            )
            predictor = Predictor(model_path=raw_model_package)
            self.assertEqual(predictor.get_input_columns(None), ["input"])
            self.assertEqual(predictor.input_columns, ["input"])
            result = predictor(batch)
        self.assertEqual(
            sorted(result.column_names), ["input", "non_input", "response"]
        )
        self.assertEqual(result["response"].to_pylist(), ["a", "b"])

    def test_columns_to_keep_restricts_output(self):
        """Check columns to keep restricts output."""
        batch = pa.table({"input": ["a", "b"], "non_input": [1, 2], "other": [3, 4]})
        with tempfile.TemporaryDirectory() as temp_dir:
            predictor = Predictor(
                model_path=create_model_package(temp_dir, custom_batch_processing=True),
                model_name="m",
                columns_to_keep=["non_input"],
            )
            result = predictor(batch)
        self.assertEqual(result.column_names, ["non_input", "response", "model_name"])

    def test_batch_to_records(self):
        """Check batch to records."""
        with tempfile.TemporaryDirectory() as temp_dir:
            predictor = Predictor(
                input_columns=["input"],
                model_name="test-model",
                model_path=create_model_package(temp_dir),
            )
            batch = {"a": np.array([1, 2]), "b": np.array([[b"a", b"b"], [b"c", b"d"]])}
            records = predictor.batch_to_records(batch)
            self.assertEqual(len(records), 2)
            self.assertEqual(records[0]["a"].tolist(), [1])
            self.assertEqual(records[0]["b"].tolist(), [[b"a", b"b"]])
            self.assertEqual(records[1]["a"].tolist(), [2])
            self.assertEqual(records[1]["b"].tolist(), [[b"c", b"d"]])
            self.assertEqual(predictor.batch_to_records({"a": np.array([])}), [])

    def test_predict_with_colliding_columns(self):
        """Check predict with colliding columns."""
        batch = pa.table(
            {"input": [b"a", b"b"], "non_input": [1, 2], "response": [1, 2]}
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            predictor = Predictor(
                input_columns=["input"],
                model_name="test-model",
                model_path=create_model_package(temp_dir),
            )
            with self.assertRaisesRegex(ValueError, "The output columns"):
                predictor(batch)

    def test_predict_with_colliding_metadata_column(self):
        """Check predict with colliding metadata column."""
        batch = pa.table(
            {"input": [b"a", b"b"], "model_name": ["existing", "existing"]}
        )
        with tempfile.TemporaryDirectory() as temp_dir:
            predictor = Predictor(
                input_columns=["input"],
                model_name="test-model",
                model_path=create_model_package(temp_dir),
            )
            with self.assertRaisesRegex(ValueError, "The output columns"):
                predictor(batch)

    def test_infer_dtype_delegates_to_shared_util(self):
        """Predictor.infer_dtype delegates to the shared util."""
        with tempfile.TemporaryDirectory() as temp_dir:
            predictor = Predictor(
                input_columns=["test"], model_path=create_model_package(temp_dir)
            )
        arr = np.array([[1, 2], [3, 4, 5], [6]], dtype=object)
        self.assertEqual(predictor.infer_dtype(arr), np.int64)

    def test_get_scalar_output_columns_without_schema(self):
        """Check get scalar output columns without schema."""
        with tempfile.TemporaryDirectory() as temp_dir:
            predictor = Predictor(model_path=create_model_package(temp_dir))
        inference = MagicMock(model_schema=None)
        self.assertEqual(predictor.get_scalar_output_columns(inference), set())


def _item(name, data_type, shape):
    return ModelSchemaItem(name=name, data_type=data_type, shape=shape)


def create_model_package(
    directory: str,
    custom_batch_processing: bool = False,
    use_scalar: bool = True,
    with_multi_dim_feature: bool = False,
):
    """Package a custom model as a raw model package."""
    model_schema = ModelSchema(
        input_schema=[
            _item("input", DataType.STRING, [1] if use_scalar else [-1]),
            *(
                [
                    _item("input_1", DataType.FLOAT, [-1]),
                    _item("input_2", DataType.FLOAT, [-1, -1]),
                ]
                if with_multi_dim_feature
                else []
            ),
        ],
        output_schema=[
            _item("response", DataType.STRING, [1] if use_scalar else [-1]),
            *(
                [
                    _item("response_1", DataType.FLOAT, [-1]),
                    _item("response_2", DataType.FLOAT, [-1, -1]),
                ]
                if with_multi_dim_feature
                else []
            ),
        ],
    )

    if with_multi_dim_feature:
        sample_data = [
            {
                "input": np.array([b"a"]),
                "input_1": np.array([1, 2]),
                "input_2": np.array([[1, 2], [3, 4]]),
            }
        ]
        batch_sample_data = [
            {
                "input": np.array([[b"a"], [b"b"]]),
                "input_1": np.array([[1, 2], [3, 4]]),
                "input_2": np.array([[[1, 2], [3, 4]], [[5, 6], [7, 8]]]),
            }
        ]
        model_class = f"{_PREDICT}.PredictWithMultiDimFeature"
    else:
        sample_data = [{"input": np.array([b"a"])}]
        batch_sample_data = [{"input": np.array([[b"a"], [b"b"]])}]
        model_class = f"{_PREDICT}.Predict"

    src_model_path = os.path.join(directory, "model")
    dest_model_path = os.path.join(directory, "model_package")
    os.makedirs(src_model_path)
    os.makedirs(dest_model_path)

    with open(os.path.join(src_model_path, "test_file.txt"), "w") as f:
        f.write("test_content")

    return CustomTritonPackager(custom_batch_processing).create_raw_model_package(
        model_path=src_model_path,
        model_class=model_class,
        model_schema=model_schema,
        sample_data=batch_sample_data if custom_batch_processing else sample_data,
        dest_model_path=dest_model_path,
        include_import_prefixes=[_FIXTURES],
    )


def create_torch_deployable_package(directory: str, backend: str | None) -> str:
    """Package ``2 * a + b`` as a deployable package of the given backend."""
    schema = ModelSchema(
        input_schema=[
            _item("a", DataType.FLOAT, [1]),
            _item("b", DataType.FLOAT, [1]),
        ],
        output_schema=[_item("y", DataType.FLOAT, [1])],
    )
    root = Path(directory)
    source = root / "model.pt"
    torch.save(_positional_model(), str(source))
    return TorchTritonPackager().create_model_package(
        str(source),
        schema,
        model_class=f"{_PREDICT}.PositionalTorchModel",
        backend=backend,
        dest_model_path=str(root / "package"),
        sample_data=[{"a": torch.ones(1, 1), "b": torch.ones(1, 1)}],
        include_import_prefixes=[_FIXTURES],
    )


def _positional_model():
    from michelangelo.lib.inference.ray.tests.fixtures.predict import (
        PositionalTorchModel,
    )

    return PositionalTorchModel()


class RealPackageInferenceTest(TestCase):
    """Runs real Ray Data inference on packages built by the packagers."""

    def setUp(self):
        """Setup."""
        if not ray.is_initialized():
            ray.init(num_cpus=2)

    def tearDown(self):
        """Teardown."""
        ray.shutdown()

    def _run(self, model_path, dataset, storage_backend=None, **model_kwargs):
        config = [
            InferenceConfig(
                model=ModelIdentifier(
                    name="real", model_path=model_path, **model_kwargs
                ),
                batch_options=BatchOptions(batch_size=4, concurrency=1),
            )
        ]
        results, metadata = batch_model_inference(
            config, {"ds": dataset}, storage_backend
        )
        return results["ds"].take_all(), metadata

    def test_custom_raw_package_from_local_directory(self):
        """Check custom raw package from local directory."""
        with tempfile.TemporaryDirectory() as temp_dir:
            package = create_model_package(temp_dir, custom_batch_processing=True)
            rows, metadata = self._run(
                package, ray.data.from_items([{"input": "a"}, {"input": "b"}])
            )

        self.assertEqual(sorted(r["response"] for r in rows), ["a", "b"])
        self.assertEqual({r["model_name"] for r in rows}, {"real"})
        self.assertEqual(
            metadata, [OutputColumnMetadata(column_name="response", model_name="real")]
        )

    def test_custom_raw_package_through_directory_storage_backend(self):
        """Check custom raw package through directory storage backend."""
        with tempfile.TemporaryDirectory() as temp_dir:
            package = create_model_package(temp_dir, custom_batch_processing=True)
            rows, _ = self._run(
                "s3://bucket/m",
                ray.data.from_items([{"input": "a"}]),
                LocalDirStorageBackend(package),
                model_type=RawModelType.CUSTOM_PYTHON,
            )

        self.assertEqual([r["response"] for r in rows], ["a"])

    def test_custom_raw_package_through_tar_storage_backend(self):
        """Check custom raw package through tar storage backend."""
        with tempfile.TemporaryDirectory() as temp_dir:
            package = create_model_package(temp_dir, custom_batch_processing=True)
            backend = LocalTarStorageBackend(package, os.path.join(temp_dir, "m.tar"))
            rows, _ = self._run(
                "s3://bucket/m/model.tar",
                ray.data.from_items([{"input": "a"}]),
                backend,
            )

        self.assertEqual([r["response"] for r in rows], ["a"])

    def test_torchscript_deployable_package(self):
        """Check torchscript deployable package."""
        with tempfile.TemporaryDirectory() as temp_dir:
            package = create_torch_deployable_package(temp_dir, backend=None)
            rows, metadata = self._run(
                package,
                ray.data.from_items([{"a": 1.0, "b": 10.0}, {"a": 2.0, "b": 20.0}]),
                model_type=RawModelType.TORCH,
            )

        self.assertEqual(sorted(r["y"] for r in rows), [12.0, 24.0])
        self.assertEqual(
            metadata, [OutputColumnMetadata(column_name="y", model_name="real")]
        )

    def test_onnx_deployable_package_through_tar_storage_backend(self):
        """Check onnx deployable package through tar storage backend."""
        with tempfile.TemporaryDirectory() as temp_dir:
            package = create_torch_deployable_package(temp_dir, backend="onnxruntime")
            backend = LocalTarStorageBackend(package, os.path.join(temp_dir, "m.tar"))
            rows, _ = self._run(
                "s3://bucket/m/model.tar",
                ray.data.from_items([{"a": 1.0, "b": 10.0}]),
                backend,
            )

        self.assertEqual([r["y"] for r in rows], [12.0])


class RunPredictorInferenceValidationTest(TestCase):
    """The result of ``_run_predictor_inference`` forwards model fields."""

    def test_model_fields_are_forwarded(self):
        """Check model fields are forwarded."""
        dataset = FakeDataset()
        config = InferenceConfig(
            model=ModelIdentifier(name="m", model_path="p", model_type="torch"),
            input_columns=["x"],
        )

        _run_predictor_inference(config, dataset)

        ctor = dataset.map_batches_calls[0][1]["fn_constructor_kwargs"]
        self.assertEqual(ctor["model_name"], "m")
        self.assertEqual(ctor["model_path"], "p")
        self.assertEqual(ctor["model_type"], "torch")
        self.assertEqual(ctor["input_columns"], ["x"])


class EncodeDecodeColumnTest(TestCase):
    """Tests for Predictor._encode_column / _decode_column.

    Static methods, callable directly - no ray.init() or Predictor instance needed.
    """

    def test_encode_empty_object_array_returns_unchanged(self):
        """Check encode empty object array returns unchanged."""
        b = np.array([], dtype=object)
        result = Predictor._encode_column(b)
        self.assertEqual(result.dtype, np.dtype(object))
        self.assertEqual(len(result), 0)

    def test_decode_empty_object_array_returns_unchanged(self):
        """Check decode empty object array returns unchanged."""
        b = np.array([], dtype=object)
        result = Predictor._decode_column(b)
        self.assertEqual(result.dtype, np.dtype(object))
        self.assertEqual(len(result), 0)

    def test_encode_empty_unicode_array(self):
        """Check encode empty unicode array."""
        # The dtype of an empty np.char.encode result is a numpy-version detail;
        # only assert this doesn't crash and stays empty.
        b = np.array([], dtype="<U1")
        result = Predictor._encode_column(b)
        self.assertEqual(len(result), 0)

    def test_decode_empty_bytes_array(self):
        """Check decode empty bytes array."""
        b = np.array([], dtype="|S1")
        result = Predictor._decode_column(b)
        self.assertEqual(len(result), 0)

    def test_encode_homogeneous_object_array_collapses_to_fixed_width(self):
        """Check encode homogeneous object array collapses to fixed width."""
        b = np.array(["a", "bb"], dtype=object)
        result = Predictor._encode_column(b)
        self.assertEqual(result.dtype.kind, "S")
        self.assertEqual(result.tolist(), [b"a", b"bb"])

    def test_decode_homogeneous_object_array_collapses_to_fixed_width(self):
        """Check decode homogeneous object array collapses to fixed width."""
        b = np.array([b"a", b"bb"], dtype=object)
        result = Predictor._decode_column(b)
        self.assertEqual(result.dtype.kind, "U")
        self.assertEqual(result.tolist(), ["a", "bb"])

    def test_decode_mixed_object_array_with_none_preserves_none(self):
        """Check decode mixed object array with none preserves none."""
        b = np.array([b"a", None], dtype=object)
        result = Predictor._decode_column(b)
        self.assertEqual(result.dtype, np.dtype(object))
        self.assertEqual(result.tolist(), ["a", None])

    def test_encode_and_decode_leave_other_dtypes_unchanged(self):
        """Check encode and decode leave other dtypes unchanged."""
        b = np.array([1, 2])
        self.assertIs(Predictor._encode_column(b), b)
        self.assertIs(Predictor._decode_column(b), b)


class FatalGpuErrorTest(TestCase):
    """Unrecoverable CUDA faults must kill the worker process.

    Ray then restarts the actor on a fresh CUDA context and resubmits the block,
    instead of the job failing. No Ray cluster is needed: the Predictor is built
    via __new__ with a mocked inference backend.
    """

    def _make_predictor(self) -> Predictor:
        predictor = Predictor.__new__(Predictor)
        predictor.input_columns = ["feature"]
        predictor.inference = MagicMock(batch_inference=True)
        predictor.model_name = "test-model"
        predictor.columns_to_keep = None
        predictor._conversion_executor = None
        return predictor

    @patch(f"{_MODULE}.logging.shutdown")
    @patch(f"{_MODULE}.os._exit", side_effect=SystemExit)
    def test_predictor_exits_process_on_fatal_gpu_error(
        self, mock_exit, _mock_shutdown
    ):
        """Check predictor exits process on fatal gpu error."""
        predictor = self._make_predictor()
        error = RuntimeError(
            "CUDA error: Invalid access of peer GPU memory over nvlink "
            "or a hardware error"
        )

        with (
            patch.object(Predictor, "inference_on_batch", side_effect=error),
            self.assertRaises(SystemExit),
        ):
            predictor(pa.table({"feature": [1.0]}))

        mock_exit.assert_called_once_with(1)

    @patch(f"{_MODULE}.os._exit")
    def test_predictor_reraises_non_fatal_errors(self, mock_exit):
        """Check predictor reraises non fatal errors."""
        predictor = self._make_predictor()

        with (
            patch.object(
                Predictor, "inference_on_batch", side_effect=ValueError("bad batch")
            ),
            self.assertRaises(ValueError),
        ):
            predictor(pa.table({"feature": [1.0]}))

        mock_exit.assert_not_called()

    def test_per_record_path_is_used_when_model_is_not_batched(self):
        """Check per record path is used when model is not batched."""
        predictor = self._make_predictor()
        predictor.inference = MagicMock(batch_inference=False)

        with patch.object(
            Predictor,
            "inference_on_records",
            return_value={"y": np.array([1.0])},
        ) as records:
            result = predictor(pa.table({"feature": [1.0]}))

        records.assert_called_once()
        self.assertEqual(result.column_names, ["feature", "y", "model_name"])


if __name__ == "__main__":
    unittest.main()
