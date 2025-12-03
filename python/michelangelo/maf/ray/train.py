"""Ray training utilities - compatible with internal MAF API."""

from typing import Optional

from ray.train import CheckpointConfig, RunConfig, ScalingConfig


def create_scaling_config(
    trainer_cpu: int = 2,
    cpu_per_worker: int = 4,
    num_workers: Optional[int] = None,
    use_gpu: bool = True,
    resources_per_worker: Optional[dict] = None,
) -> ScalingConfig:
    """Create Ray ScalingConfig for distributed training.

    Compatible with internal uber.ai.michelangelo.maf.ray.train.create_scaling_config.
    """
    if num_workers is None:
        # Infer from runtime or default
        num_workers = 4

    if resources_per_worker is None:
        resources_per_worker = {"CPU": cpu_per_worker}
        if use_gpu:
            resources_per_worker["GPU"] = 1

    return ScalingConfig(
        num_workers=num_workers,
        use_gpu=use_gpu,
        resources_per_worker=resources_per_worker,
    )


def create_run_config(
    name: Optional[str] = None,
    storage_path: Optional[str] = None,
    checkpoint_config: CheckpointConfig = None,
    stop: Optional[dict] = None,  # Keep parameter for compatibility but don't use it
    verbose: int = 1,            # Keep parameter for compatibility but don't use it
) -> RunConfig:
    """Create Ray RunConfig for distributed training.

    Compatible with internal uber.ai.michelangelo.maf.ray.train.create_run_config.
    """
    return RunConfig(
        name=name,
        storage_path=storage_path,
        checkpoint_config=checkpoint_config,
    )
