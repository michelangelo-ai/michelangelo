"""Tests for michelangelo.maf.ray.train module."""

from michelangelo.maf.ray.train import create_run_config, create_scaling_config


class TestCreateScalingConfig:
    """Test create_scaling_config function."""

    def test_create_scaling_config_defaults(self):
        """Test create_scaling_config with default parameters."""
        config = create_scaling_config()

        assert config.num_workers == 4  # default when None
        assert config.use_gpu is True
        assert config.resources_per_worker == {"CPU": 4, "GPU": 1}

    def test_create_scaling_config_custom_workers(self):
        """Test create_scaling_config with custom num_workers."""
        config = create_scaling_config(num_workers=8)

        assert config.num_workers == 8
        assert config.use_gpu is True
        assert config.resources_per_worker == {"CPU": 4, "GPU": 1}

    def test_create_scaling_config_without_gpu(self):
        """Test create_scaling_config with GPU disabled."""
        config = create_scaling_config(use_gpu=False)

        assert config.num_workers == 4
        assert config.use_gpu is False
        assert config.resources_per_worker == {"CPU": 4}
        assert "GPU" not in config.resources_per_worker

    def test_create_scaling_config_custom_cpu_per_worker(self):
        """Test create_scaling_config with custom cpu_per_worker."""
        config = create_scaling_config(cpu_per_worker=8)

        assert config.num_workers == 4
        assert config.use_gpu is True
        assert config.resources_per_worker == {"CPU": 8, "GPU": 1}

    def test_create_scaling_config_custom_trainer_cpu(self):
        """Test create_scaling_config with custom trainer_cpu."""
        # trainer_cpu is a parameter but not used in the implementation
        config = create_scaling_config(trainer_cpu=1)

        assert config.num_workers == 4
        assert config.use_gpu is True
        assert config.resources_per_worker == {"CPU": 4, "GPU": 1}

    def test_create_scaling_config_custom_resources(self):
        """Test create_scaling_config with custom resources_per_worker."""
        custom_resources = {"CPU": 2, "GPU": 2, "memory": "16GiB"}
        config = create_scaling_config(resources_per_worker=custom_resources)

        assert config.num_workers == 4
        assert config.use_gpu is True
        assert config.resources_per_worker == custom_resources

    def test_create_scaling_config_all_custom(self):
        """Test create_scaling_config with all custom parameters."""
        config = create_scaling_config(
            trainer_cpu=1,
            cpu_per_worker=2,
            num_workers=6,
            use_gpu=False,
            resources_per_worker={"CPU": 3, "memory": "8GiB"}
        )

        assert config.num_workers == 6
        assert config.use_gpu is False
        assert config.resources_per_worker == {"CPU": 3, "memory": "8GiB"}


class TestCreateRunConfig:
    """Test create_run_config function."""

    def test_create_run_config_defaults(self):
        """Test create_run_config with default parameters."""
        config = create_run_config()

        # Ray automatically generates a default name and storage path
        assert config.name is not None  # Ray generates default name
        assert config.storage_path is not None  # Ray sets default storage path
        # Ray creates default checkpoint config
        assert config.checkpoint_config is not None

    def test_create_run_config_with_name(self):
        """Test create_run_config with custom name."""
        config = create_run_config(name="test_run")

        assert config.name == "test_run"
        # Ray sets default storage path even when only name is provided
        assert config.storage_path is not None
        # Ray creates default checkpoint config
        assert config.checkpoint_config is not None

    def test_create_run_config_with_storage_path(self):
        """Test create_run_config with custom storage_path."""
        config = create_run_config(storage_path="/tmp/ray_results")

        # Ray generates a default name even when only storage_path is provided
        assert config.name is not None
        assert config.storage_path == "/tmp/ray_results"
        # Ray creates default checkpoint config
        assert config.checkpoint_config is not None

    def test_create_run_config_with_checkpoint_config(self):
        """Test create_run_config with custom checkpoint_config."""
        from ray.train import CheckpointConfig
        checkpoint_config = CheckpointConfig(num_to_keep=3)

        config = create_run_config(checkpoint_config=checkpoint_config)

        # Ray generates defaults for name and storage path
        assert config.name is not None
        assert config.storage_path is not None
        assert config.checkpoint_config == checkpoint_config

    def test_create_run_config_compatibility_params(self):
        """Test create_run_config with compatibility parameters that are ignored."""
        config = create_run_config(
            name="test",
            storage_path="/tmp",
            stop={"training_iteration": 10},  # ignored for compatibility
            verbose=2  # ignored for compatibility
        )

        assert config.name == "test"
        assert config.storage_path == "/tmp"
        # Ray creates default checkpoint config even with explicit params
        assert config.checkpoint_config is not None
        # stop and verbose params are kept for compatibility but not used

    def test_create_run_config_all_params(self):
        """Test create_run_config with all parameters."""
        from ray.train import CheckpointConfig
        checkpoint_config = CheckpointConfig(num_to_keep=5)

        config = create_run_config(
            name="full_test",
            storage_path="/tmp/full_results",
            checkpoint_config=checkpoint_config,
            stop={"epochs": 20},
            verbose=1
        )

        assert config.name == "full_test"
        assert config.storage_path == "/tmp/full_results"
        assert config.checkpoint_config == checkpoint_config
