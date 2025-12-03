"""Tests for Lightning trainer module."""

from unittest.mock import MagicMock, patch

import pytest

from michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer import (
    LightningTrainer,
    LightningTrainerParam,
)


class TestLightningTrainerParam:
    """Test LightningTrainerParam dataclass."""

    def test_lightning_trainer_param_creation(self):
        """Test basic creation of LightningTrainerParam."""
        create_model = MagicMock()
        model_kwargs = {"param1": "value1"}
        train_data = MagicMock()
        validation_data = MagicMock()

        param = LightningTrainerParam(
            create_model=create_model,
            model_kwargs=model_kwargs,
            train_data=train_data,
            validation_data=validation_data,
            batch_size=32,
            num_epochs=10,
        )

        assert param.create_model == create_model
        assert param.model_kwargs == model_kwargs
        assert param.train_data == train_data
        assert param.validation_data == validation_data
        assert param.batch_size == 32
        assert param.num_epochs == 10
        assert param.lightning_trainer_kwargs == {}  # default from __post_init__

    def test_lightning_trainer_param_with_lightning_kwargs(self):
        """Test LightningTrainerParam with custom lightning_trainer_kwargs."""
        create_model = MagicMock()
        train_data = MagicMock()
        validation_data = MagicMock()
        lightning_kwargs = {"accelerator": "gpu", "devices": 2}

        param = LightningTrainerParam(
            create_model=create_model,
            model_kwargs={},
            train_data=train_data,
            validation_data=validation_data,
            batch_size=16,
            num_epochs=5,
            lightning_trainer_kwargs=lightning_kwargs,
        )

        assert param.lightning_trainer_kwargs == lightning_kwargs

    def test_lightning_trainer_param_post_init_none(self):
        """Test __post_init__ when lightning_trainer_kwargs is None."""
        create_model = MagicMock()
        train_data = MagicMock()
        validation_data = MagicMock()

        param = LightningTrainerParam(
            create_model=create_model,
            model_kwargs={},
            train_data=train_data,
            validation_data=validation_data,
            batch_size=8,
            num_epochs=3,
            lightning_trainer_kwargs=None,
        )

        assert param.lightning_trainer_kwargs == {}


class TestLightningTrainer:
    """Test LightningTrainer class."""

    def setup_method(self):
        """Setup test fixtures."""
        self.mock_create_model = MagicMock()
        self.mock_train_data = MagicMock()
        self.mock_validation_data = MagicMock()

        self.param = LightningTrainerParam(
            create_model=self.mock_create_model,
            model_kwargs={"test_param": "test_value"},
            train_data=self.mock_train_data,
            validation_data=self.mock_validation_data,
            batch_size=32,
            num_epochs=10,
            lightning_trainer_kwargs={"accelerator": "cpu"},
        )

    @patch(
        "michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer.TorchTrainer"
    )
    def test_lightning_trainer_initialization(self, mock_torch_trainer):
        """Test LightningTrainer initialization."""
        trainer = LightningTrainer(self.param)

        assert trainer.param == self.param
        mock_torch_trainer.assert_called_once()

        # Check that TorchTrainer was called with correct arguments
        call_args = mock_torch_trainer.call_args
        assert "train_loop_per_worker" in call_args[1]
        assert "datasets" in call_args[1]
        assert "train_loop_config" in call_args[1]

        # Check datasets
        datasets = call_args[1]["datasets"]
        assert datasets["train"] == self.mock_train_data
        assert datasets["validation"] == self.mock_validation_data

        # Check train_loop_config
        assert call_args[1]["train_loop_config"] == {}

    @patch(
        "michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer.TorchTrainer"
    )
    def test_setup_trainer_creates_torch_trainer(self, mock_torch_trainer):
        """Test that _setup_trainer creates TorchTrainer instance."""
        trainer = LightningTrainer(self.param)

        assert hasattr(trainer, "torch_trainer")
        assert trainer.torch_trainer == mock_torch_trainer.return_value

    @patch(
        "michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer.TorchTrainer"
    )
    def test_train_method(self, mock_torch_trainer):
        """Test the train method."""
        mock_run_config = MagicMock()
        mock_scaling_config = MagicMock()
        mock_result = MagicMock()

        # Setup the mock torch trainer
        mock_trainer_instance = MagicMock()
        mock_trainer_instance.fit.return_value = mock_result
        mock_torch_trainer.return_value = mock_trainer_instance

        trainer = LightningTrainer(self.param)

        # Call train method
        result = trainer.train(mock_run_config, mock_scaling_config)

        # Verify scaling and run configs were set
        assert trainer.torch_trainer._scaling_config == mock_scaling_config
        assert trainer.torch_trainer._run_config == mock_run_config

        # Verify fit was called and result returned
        mock_trainer_instance.fit.assert_called_once()
        assert result == mock_result

    @patch(
        "michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer.TorchTrainer"
    )
    @patch("michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer.log")
    def test_train_method_with_logging(self, mock_log, mock_torch_trainer):
        """Test that the train method includes proper logging."""
        mock_run_config = MagicMock()
        mock_scaling_config = MagicMock()

        trainer = LightningTrainer(self.param)
        trainer.train(mock_run_config, mock_scaling_config)

        # Verify logging calls
        mock_log.info.assert_any_call("Starting distributed Lightning training...")
        mock_log.info.assert_any_call("Distributed Lightning training completed")

    def test_train_loop_per_worker_function_exists(self):
        """Test that train_loop_per_worker function is created in _setup_trainer."""
        mock_path = (
            "michelangelo.sdk.trainer.torch.pytorch_lightning"
            ".lightning_trainer.TorchTrainer"
        )
        with patch(mock_path) as mock_torch_trainer:
            LightningTrainer(self.param)

            # Get the train_loop_per_worker function that was passed to TorchTrainer
            call_args = mock_torch_trainer.call_args
            train_loop_func = call_args[1]["train_loop_per_worker"]

            # Verify it's a callable function
            assert callable(train_loop_func)

    @patch(
        "michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer.TorchTrainer"
    )
    def test_train_loop_per_worker_closure_captures_param(self, mock_torch_trainer):
        """Test that the train_loop_per_worker closure captures self.param."""
        LightningTrainer(self.param)

        # Get the train_loop_per_worker function
        call_args = mock_torch_trainer.call_args
        train_loop_func = call_args[1]["train_loop_per_worker"]

        # The closure should have access to self.param
        # We can't easily test the full execution without mocking many Ray components,
        # but we can verify the function was created correctly
        assert hasattr(train_loop_func, "__closure__")
        assert train_loop_func.__closure__ is not None

    @patch(
        "michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer.TorchTrainer"
    )
    def test_train_loop_per_worker_execution(self, mock_torch_trainer):
        """Test execution of the train_loop_per_worker function."""
        # Create a mock Lightning module
        mock_model = MagicMock()

        # Mock the create_model function to return our mock model
        self.mock_create_model.return_value = mock_model

        # Create trainer to get the train_loop_per_worker function
        LightningTrainer(self.param)

        # Get the train_loop_per_worker function
        call_args = mock_torch_trainer.call_args
        train_loop_func = call_args[1]["train_loop_per_worker"]

        # Mock all Ray components
        with (
            patch("ray.train.get_context"),
            patch("ray.train.lightning.RayLightningEnvironment"),
            patch("ray.train.get_dataset_shard") as mock_get_shard,
            patch("pytorch_lightning.Trainer") as mock_pl_trainer,
            patch("torch.save"),
            patch("tempfile.mkdtemp", return_value="/tmp/test"),
            patch("os.path.join", side_effect=lambda *args: "/".join(args)),
            patch("ray.train.report") as mock_ray_report,
            patch("ray.train.Checkpoint.from_directory"),
        ):
            # Setup mock dataset shards
            mock_train_shard = MagicMock()
            mock_val_shard = MagicMock()
            mock_get_shard.side_effect = lambda name: {
                "train": mock_train_shard,
                "validation": mock_val_shard,
            }[name]

            # Mock the iter_batches method to return test data
            mock_train_shard.iter_batches.return_value = [
                {"feature1": [1, 2], "label": [0, 1]}
            ]
            mock_val_shard.iter_batches.return_value = [
                {"feature1": [3, 4], "label": [1, 0]}
            ]

            # Setup Lightning trainer mock
            mock_trainer_instance = MagicMock()
            mock_pl_trainer.return_value = mock_trainer_instance
            mock_trainer_instance.logged_metrics = {"loss": 0.5, "accuracy": 0.8}
            mock_trainer_instance.save_checkpoint = MagicMock()

            # Execute the train loop function
            result = train_loop_func({})

            # Verify that the model was created with correct kwargs
            self.mock_create_model.assert_called_once_with(**self.param.model_kwargs)

            # Verify Lightning trainer was configured correctly
            mock_pl_trainer.assert_called_once()
            trainer_kwargs = mock_pl_trainer.call_args[1]
            assert trainer_kwargs["max_epochs"] == self.param.num_epochs
            assert trainer_kwargs["enable_checkpointing"] is True
            assert trainer_kwargs["logger"] is False

            # Verify trainer.fit was called
            mock_trainer_instance.fit.assert_called_once()

            # Verify checkpointing
            mock_trainer_instance.save_checkpoint.assert_called_once()

            # Verify Ray reporting
            mock_ray_report.assert_called_once()

            # Check return value
            assert result == {"metrics": {"loss": 0.5, "accuracy": 0.8}}

    @patch(
        "michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer.TorchTrainer"
    )
    def test_train_loop_per_worker_with_custom_strategy(self, mock_torch_trainer):
        """Test train_loop_per_worker with custom strategy."""
        # Update param to include custom strategy
        custom_param = LightningTrainerParam(
            create_model=self.mock_create_model,
            model_kwargs={"test_param": "test_value"},
            train_data=self.mock_train_data,
            validation_data=self.mock_validation_data,
            batch_size=32,
            num_epochs=10,
            lightning_trainer_kwargs={"strategy": "ddp", "devices": 2},
        )

        # Create a mock Lightning module
        mock_model = MagicMock()
        self.mock_create_model.return_value = mock_model

        # Create trainer to get the train_loop_per_worker function
        LightningTrainer(custom_param)

        # Get the train_loop_per_worker function
        call_args = mock_torch_trainer.call_args
        train_loop_func = call_args[1]["train_loop_per_worker"]

        # Mock all Ray components
        with (
            patch("ray.train.get_context"),
            patch("ray.train.lightning.RayLightningEnvironment"),
            patch("ray.train.get_dataset_shard") as mock_get_shard,
            patch("pytorch_lightning.Trainer") as mock_pl_trainer,
            patch("torch.save"),
            patch("tempfile.mkdtemp", return_value="/tmp/test"),
            patch("os.path.join", side_effect=lambda *args: "/".join(args)),
            patch("ray.train.report") as mock_ray_report,
            patch("ray.train.Checkpoint.from_directory"),
        ):
            # Setup mock dataset shards
            mock_train_shard = MagicMock()
            mock_val_shard = MagicMock()
            mock_get_shard.side_effect = lambda name: {
                "train": mock_train_shard,
                "validation": mock_val_shard,
            }[name]

            # Mock the iter_batches method to return test data
            mock_train_shard.iter_batches.return_value = [
                {"feature1": [1, 2], "label": [0, 1]}
            ]
            mock_val_shard.iter_batches.return_value = [
                {"feature1": [3, 4], "label": [1, 0]}
            ]

            # Setup Lightning trainer mock
            mock_trainer_instance = MagicMock()
            mock_pl_trainer.return_value = mock_trainer_instance
            mock_trainer_instance.logged_metrics = {}

            # Execute the train loop function
            result = train_loop_func({})

            # Verify Lightning trainer was configured with custom strategy
            mock_pl_trainer.assert_called_once()
            trainer_kwargs = mock_pl_trainer.call_args[1]
            assert trainer_kwargs["strategy"] == "ddp"
            assert trainer_kwargs["devices"] == 2

            # Verify Ray reporting with empty metrics
            mock_ray_report.assert_called_once()

            # Check return value has empty metrics when no logged_metrics
            assert result == {"metrics": {}}

    @patch(
        "michelangelo.sdk.trainer.torch.pytorch_lightning.lightning_trainer.TorchTrainer"
    )
    def test_train_loop_per_worker_with_pandas_batch_data(self, mock_torch_trainer):
        """Test train_loop_per_worker with pandas-style batch data."""
        # Create a mock Lightning module
        mock_model = MagicMock()
        self.mock_create_model.return_value = mock_model

        # Create trainer to get the train_loop_per_worker function
        LightningTrainer(self.param)

        # Get the train_loop_per_worker function
        call_args = mock_torch_trainer.call_args
        train_loop_func = call_args[1]["train_loop_per_worker"]

        # Mock all Ray components
        with (
            patch("ray.train.get_context"),
            patch("ray.train.lightning.RayLightningEnvironment"),
            patch("ray.train.get_dataset_shard") as mock_get_shard,
            patch("pytorch_lightning.Trainer") as mock_pl_trainer,
            patch("torch.save"),
            patch("tempfile.mkdtemp", return_value="/tmp/test"),
            patch("os.path.join", side_effect=lambda *args: "/".join(args)),
            patch("ray.train.report"),
            patch("ray.train.Checkpoint.from_directory"),
        ):
            # Setup mock dataset shards with non-dict data that needs pandas conversion
            mock_train_shard = MagicMock()
            mock_val_shard = MagicMock()
            mock_get_shard.side_effect = lambda name: {
                "train": mock_train_shard,
                "validation": mock_val_shard,
            }[name]

            # Create a mock pandas DataFrame-like batch
            mock_batch = MagicMock()
            mock_batch.to_pandas.return_value.to_dict.return_value = [
                {"feature1": 1, "label": 0},
                {"feature1": 2, "label": 1},
            ]

            # Mock iter_batches to return non-dict data (triggers pandas path)
            mock_train_shard.iter_batches.return_value = [mock_batch]
            mock_val_shard.iter_batches.return_value = [mock_batch]

            # Setup Lightning trainer mock
            mock_trainer_instance = MagicMock()
            mock_pl_trainer.return_value = mock_trainer_instance
            mock_trainer_instance.logged_metrics = {"loss": 0.3}

            # Execute the train loop function
            result = train_loop_func({})

            # Verify the pandas conversion path was used
            mock_batch.to_pandas.assert_called()
            mock_batch.to_pandas.return_value.to_dict.assert_called_with("records")

            # Verify training completed
            mock_trainer_instance.fit.assert_called_once()

            # Check return value
            assert result == {"metrics": {"loss": 0.3}}

    def test_lightning_trainer_real_integration(self):
        """Integration test with real Ray and Lightning training."""
        import contextlib
        import tempfile

        import pytorch_lightning as pl
        import ray
        import torch
        import torch.nn as nn
        from ray.data import from_items
        from ray.train import CheckpointConfig, RunConfig, ScalingConfig

        # Skip ONLY if Ray is not available for setup
        try:
            # Use real Ray with minimal resources (not local_mode - async actor issue)
            ray.init(
                num_cpus=2,
                ignore_reinit_error=True,
                configure_logging=False,
                log_to_driver=False,
            )
        except Exception as e:
            pytest.skip(f"Ray not available for integration test: {e}")

        # From here on, let real failures be test failures, not skips
        try:
            # Simple Lightning module for testing
            class SimpleModel(pl.LightningModule):
                def __init__(self):
                    super().__init__()
                    self.linear = nn.Linear(2, 1)

                def forward(self, x):
                    return self.linear(x)

                def training_step(self, batch, batch_idx):
                    x1 = torch.tensor(batch["x1"], dtype=torch.float32)
                    x2 = torch.tensor(batch["x2"], dtype=torch.float32)
                    x = torch.stack([x1, x2], dim=1)
                    y = torch.tensor(batch["y"], dtype=torch.float32).unsqueeze(1)
                    y_hat = self(x)
                    loss = nn.MSELoss()(y_hat, y)
                    self.log("train_loss", loss)
                    return loss

                def validation_step(self, batch, batch_idx):
                    x1 = torch.tensor(batch["x1"], dtype=torch.float32)
                    x2 = torch.tensor(batch["x2"], dtype=torch.float32)
                    x = torch.stack([x1, x2], dim=1)
                    y = torch.tensor(batch["y"], dtype=torch.float32).unsqueeze(1)
                    y_hat = self(x)
                    loss = nn.MSELoss()(y_hat, y)
                    self.log("val_loss", loss)

                def configure_optimizers(self):
                    return torch.optim.Adam(self.parameters(), lr=0.1)

            # Create simple synthetic datasets
            train_data = [
                {"x1": 1.0, "x2": 2.0, "y": 3.0},
                {"x1": 2.0, "x2": 3.0, "y": 5.0},
                {"x1": 3.0, "x2": 1.0, "y": 4.0},
                {"x1": 1.0, "x2": 1.0, "y": 2.0},
            ]

            val_data = [
                {"x1": 2.0, "x2": 2.0, "y": 4.0},
                {"x1": 3.0, "x2": 3.0, "y": 6.0},
            ]

            train_dataset = from_items(train_data)
            val_dataset = from_items(val_data)

            # Create Lightning trainer parameters
            param = LightningTrainerParam(
                create_model=SimpleModel,
                model_kwargs={},
                train_data=train_dataset,
                validation_data=val_dataset,
                batch_size=2,
                num_epochs=1,  # Just 1 epoch for speed
                lightning_trainer_kwargs={
                    "accelerator": "cpu",
                    "devices": 1,
                    "enable_progress_bar": False,
                    "enable_model_summary": False,
                },
            )

            # Create trainer
            trainer = LightningTrainer(param)

            # Create configs for local training
            scaling_config = ScalingConfig(
                num_workers=1,  # Single worker for simplicity
                use_gpu=False,
                resources_per_worker={"CPU": 1},
            )

            with tempfile.TemporaryDirectory() as temp_dir:
                run_config = RunConfig(
                    name="test_integration",
                    storage_path=temp_dir,
                    checkpoint_config=CheckpointConfig(num_to_keep=1),
                )

                # Run actual training
                result = trainer.train(run_config, scaling_config)

                # Verify training completed successfully
                assert result is not None
                assert hasattr(result, "metrics")

                # Check that we have some metrics (loss should be present)
                final_metrics = result.metrics
                assert isinstance(final_metrics, dict)

                # Training should have produced some loss values
                # We don't assert specific values since they can vary
                print(f"Training completed with metrics: {final_metrics}")

        finally:
            # Always cleanup Ray, regardless of success or failure
            with contextlib.suppress(Exception):
                ray.shutdown()
