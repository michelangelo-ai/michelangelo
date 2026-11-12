"""Tests for the Ray implementation of the tabular inference task."""

from __future__ import annotations

from unittest import TestCase
from unittest.mock import MagicMock, call, patch

import ray

from michelangelo.lib.artifact_manager.storage_backend import LocalStorageBackend
from michelangelo.lib.inference.ray.inference import OutputColumnMetadata
from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.inference import (
    OfflineInferenceConfig,
    ParquetReadConfig,
    RayDataContextConfig,
    SamplingConfig,
    TabularInferenceConfig,
    WriteConfig,
)
from michelangelo.workflow.tasks.tabular_inference.offline.task_impl import (
    tabular_inference_impl,
)
from michelangelo.workflow.variables import DatasetVariable

_MODULE = "michelangelo.workflow.tasks.tabular_inference.offline.task_impl"


class TabularInferenceImplTest(TestCase):
    """Tests for ``tabular_inference_impl``."""

    @classmethod
    def setUpClass(cls):
        """Start one local Ray session for the class."""
        ray.init(num_cpus=1, include_dashboard=False, ignore_reinit_error=True)

    @classmethod
    def tearDownClass(cls):
        """Stop the Ray session."""
        ray.shutdown()

    @patch(f"{_MODULE}.DatasetVariable.save_ray_dataset")
    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_tabular_inference_impl(self, _context, mock_offline, mock_save):
        """Results are saved and the model arguments reach offline_inference."""
        config = TabularInferenceConfig(
            local_models=[OfflineInferenceConfig(input_columns=["f1"])]
        )
        train_ds = ray.data.from_items([{"f1": 1}])
        datasets = {"train": DatasetVariable.create(train_ds)}
        mock_offline.return_value = (
            {"train": train_ds},
            [OutputColumnMetadata(column_name="pred", model_name="m")],
        )
        assembled, registry = MagicMock(), object()
        backend = LocalStorageBackend("/tmp")

        with self.assertLogs(_MODULE, level="INFO") as logs:
            result = tabular_inference_impl(
                config,
                datasets,
                assembled,
                storage_backend=backend,
                registry_client=registry,
            )

        self.assertIn("train", result)
        mock_save.assert_called_once_with()
        mock_offline.assert_called_once_with(
            config.local_models,
            {"train": train_ds},
            assembled,
            storage_backend=backend,
            registry_client=registry,
        )
        self.assertTrue(any("column_name='pred'" in line for line in logs.output))

    @patch(f"{_MODULE}.DatasetVariable.save_ray_dataset")
    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_forwards_write_config(self, _context, mock_offline, mock_save):
        """Row-count options reach the dataset write."""
        config = TabularInferenceConfig(
            local_models=[OfflineInferenceConfig(input_columns=["f1"])],
            write_config=WriteConfig(max_rows_per_file=100, min_rows_per_file=10),
        )
        train_ds = ray.data.from_items([{"f1": 1}])
        mock_offline.return_value = ({"train": train_ds}, [])

        tabular_inference_impl(config, {"train": DatasetVariable.create(train_ds)})

        mock_save.assert_called_once_with(max_rows_per_file=100, min_rows_per_file=10)

    @patch(f"{_MODULE}.DatasetVariable.save_ray_dataset")
    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_forwards_write_memory_as_ray_remote_args(
        self, _context, mock_offline, mock_save
    ):
        """Write memory goes through ``ray_remote_args``."""
        config = TabularInferenceConfig(
            local_models=[OfflineInferenceConfig(input_columns=["f1"])],
            write_config=WriteConfig(memory=25769803776),
        )
        train_ds = ray.data.from_items([{"f1": 1}])
        mock_offline.return_value = ({"train": train_ds}, [])

        tabular_inference_impl(config, {"train": DatasetVariable.create(train_ds)})

        mock_save.assert_called_once_with(ray_remote_args={"memory": 25769803776})

    @patch(f"{_MODULE}.DatasetVariable.save_ray_dataset")
    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_forwards_write_concurrency(self, _context, mock_offline, mock_save):
        """Write concurrency reaches the dataset write."""
        config = TabularInferenceConfig(
            local_models=[OfflineInferenceConfig(input_columns=["f1"])],
            write_config=WriteConfig(concurrency=8),
        )
        train_ds = ray.data.from_items([{"f1": 1}])
        mock_offline.return_value = ({"train": train_ds}, [])

        tabular_inference_impl(config, {"train": DatasetVariable.create(train_ds)})

        mock_save.assert_called_once_with(concurrency=8)

    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_unsupported_write_config_is_rejected(self, _context, mock_offline):
        """A write option the installed Ray cannot do raises ConfigurationError."""
        config = TabularInferenceConfig(
            local_models=[OfflineInferenceConfig(input_columns=["f1"])],
            write_config=WriteConfig(max_rows_per_file=2),
        )
        train_ds = ray.data.from_items([{"f1": 1}])
        mock_offline.return_value = ({"train": train_ds}, [])

        with patch(
            "michelangelo.workflow.tasks.functions.data.ray.write.ray"
        ) as mock_ray:
            mock_ray.__version__ = "2.40.0"
            with self.assertRaisesRegex(ConfigurationError, "max_rows_per_file"):
                tabular_inference_impl(
                    config, {"train": DatasetVariable.create(train_ds)}
                )

    @patch(f"{_MODULE}.apply_sampling")
    @patch(f"{_MODULE}.DatasetVariable.save_ray_dataset")
    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_with_sampling(self, _context, mock_offline, _save, mock_sampling):
        """Each dataset is sampled with the sampling config before scoring."""
        sampling = SamplingConfig(fractions={"train": 0.5})
        config = TabularInferenceConfig(
            local_models=[OfflineInferenceConfig(input_columns=["f1"])],
            sampling=sampling,
        )
        train_ds = ray.data.from_items([{"f1": 1}])
        sampled_ds = ray.data.from_items([{"f1": 2}])
        mock_sampling.return_value = sampled_ds
        mock_offline.return_value = ({"train": train_ds}, [])

        tabular_inference_impl(config, {"train": DatasetVariable.create(train_ds)})

        mock_sampling.assert_called_once_with(train_ds, sampling, "train")
        self.assertEqual(mock_offline.call_args.args[1], {"train": sampled_ds})

    @patch(f"{_MODULE}.apply_sampling")
    @patch(f"{_MODULE}.DatasetVariable.save_ray_dataset")
    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_without_sampling_config_does_not_sample(
        self, _context, mock_offline, _save, mock_sampling
    ):
        """No sampling config leaves the datasets alone."""
        train_ds = ray.data.from_items([{"f1": 1}])
        mock_offline.return_value = ({"train": train_ds}, [])

        tabular_inference_impl(
            TabularInferenceConfig(local_models=[OfflineInferenceConfig()]),
            {"train": DatasetVariable.create(train_ds)},
        )

        mock_sampling.assert_not_called()

    @patch(f"{_MODULE}.DatasetVariable.load_ray_dataset")
    @patch(f"{_MODULE}.DatasetVariable.save_ray_dataset")
    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_none_result_is_not_saved(self, _context, mock_offline, mock_save, _load):
        """A dataset with no result becomes an empty variable that is not saved."""
        mock_offline.return_value = ({"train": None}, [])

        result = tabular_inference_impl(
            TabularInferenceConfig(), {"train": DatasetVariable.create(None)}
        )

        self.assertIn("train", result)
        mock_save.assert_not_called()

    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.DatasetVariable.load_ray_dataset")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_forwards_read_config_and_ray_data_context(
        self, mock_context, mock_load, mock_offline
    ):
        """Read kwargs go to load_ray_dataset; context options go to Ray Data."""
        config = TabularInferenceConfig(
            local_models=[OfflineInferenceConfig(input_columns=["f1"])],
            parquet_read_config=ParquetReadConfig(
                override_num_blocks=500, concurrency=32
            ),
            ray_data_context=RayDataContextConfig(
                max_block_size=268435456,
                object_store_memory_limit=1717986918400,
                wait_for_min_actors_s=600,
                preserve_order=True,
            ),
            max_errored_blocks=25,
        )
        datasets = {
            "train": DatasetVariable.create(None),
            "validation": DatasetVariable.create(None),
        }
        mock_offline.return_value = ({}, [])

        tabular_inference_impl(config, datasets)

        mock_context.assert_called_once_with(
            min_block_size=None,
            max_block_size=268435456,
            retried_io_errors=None,
            object_store_memory_limit=1717986918400,
            wait_for_min_actors_s=600,
            max_blocks_in_streaming_gen_buffer=None,
            preserve_order=True,
            max_errored_blocks=25,
        )
        explicit_calls = [c for c in mock_load.call_args_list if c.kwargs]
        expected = call(concurrency=32, override_num_blocks=500)
        self.assertEqual(explicit_calls, [expected, expected])

    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.DatasetVariable.load_ray_dataset")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_forwards_per_dataset_override_num_blocks(
        self, _context, mock_load, mock_offline
    ):
        """``override_num_blocks_per_dataset`` picks a value per dataset name."""
        config = TabularInferenceConfig(
            local_models=[OfflineInferenceConfig(input_columns=["f1"])],
            parquet_read_config=ParquetReadConfig(
                concurrency=32,
                override_num_blocks_per_dataset={"train": 1638, "validation": 410},
            ),
        )
        datasets = {
            "train": DatasetVariable.create(None),
            "validation": DatasetVariable.create(None),
        }
        mock_offline.return_value = ({}, [])

        tabular_inference_impl(config, datasets)

        calls = {
            c.kwargs["override_num_blocks"]: c
            for c in mock_load.call_args_list
            if c.kwargs
        }
        self.assertEqual(
            calls[1638],
            call(concurrency=32, override_num_blocks=1638),
        )
        self.assertEqual(
            calls[410],
            call(concurrency=32, override_num_blocks=410),
        )

    @patch(f"{_MODULE}.DatasetVariable.save_ray_dataset")
    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_logs_ray_stats(self, _context, mock_offline, _save):
        """Ray Data operator stats are logged after the save."""
        ds = ray.data.from_items([{"f1": 1}])
        ds.stats = lambda: "Operator 1 MapBatches(Predictor): 1 tasks executed"
        mock_offline.return_value = ({"train": ds}, [])

        with self.assertLogs(_MODULE, level="INFO") as logs:
            tabular_inference_impl(
                TabularInferenceConfig(), {"train": DatasetVariable.create(ds)}
            )

        self.assertTrue(any("[PROFILER]" in line for line in logs.output))

    @patch(f"{_MODULE}.DatasetVariable.save_ray_dataset")
    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_empty_ray_stats_are_not_logged(self, _context, mock_offline, _save):
        """Empty stats produce no profiler log."""
        ds = ray.data.from_items([{"f1": 1}])
        ds.stats = lambda: ""
        mock_offline.return_value = ({"train": ds}, [])

        with self.assertLogs(_MODULE, level="INFO") as logs:
            tabular_inference_impl(
                TabularInferenceConfig(), {"train": DatasetVariable.create(ds)}
            )

        self.assertFalse(any("[PROFILER]" in line for line in logs.output))

    @patch(f"{_MODULE}.DatasetVariable.save_ray_dataset")
    @patch(f"{_MODULE}.offline_inference")
    @patch(f"{_MODULE}.set_ray_data_context")
    def test_stats_exception_is_ignored(self, _context, mock_offline, _save):
        """A failure while reading stats does not fail the task."""
        ds = ray.data.from_items([{"f1": 1}])
        ds.stats = lambda: (_ for _ in ()).throw(RuntimeError("stats failed"))
        mock_offline.return_value = ({"train": ds}, [])

        result = tabular_inference_impl(
            TabularInferenceConfig(), {"train": DatasetVariable.create(ds)}
        )

        self.assertIn("train", result)
