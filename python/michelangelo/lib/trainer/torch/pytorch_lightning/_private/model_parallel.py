"""Ray-aware FSDP2 (``ModelParallelStrategy``) glue.

``ModelParallelStrategy`` requires pytorch-lightning >= 2.3, so ``util.py`` imports this module
lazily (only once FSDP2 is requested) rather than at top level.
"""

from __future__ import annotations

import logging
from typing import Any

import ray
import torch
from pytorch_lightning.strategies import ModelParallelStrategy

_logger = logging.getLogger(__name__)


class RayModelParallelStrategy(ModelParallelStrategy):
    """Ray glue for FSDP2.

    Ray Train ships RayDDPStrategy/RayFSDPStrategy/RayDeepSpeedStrategy but no
    equivalent for ModelParallelStrategy, so we supply the Ray device and rank
    wiring ourselves. Remove once Ray adds native ModelParallelStrategy support.
    """

    def __init__(self, *args, **kwargs):
        if kwargs.get("save_distributed_checkpoint") is False:
            _logger.warning(
                "save_distributed_checkpoint=False is not supported; FSDP2 always saves sharded (DCP) "
                "checkpoints. Forcing save_distributed_checkpoint=True."
            )
        kwargs["save_distributed_checkpoint"] = True

        if kwargs.get("tensor_parallel_size") is not None:
            raise ValueError(
                "Tensor parallelism is not currently supported. Leave tensor_parallel_size unset. "
                f"Got tensor_parallel_size: {kwargs.get('tensor_parallel_size')}"
            )

        if kwargs.get("data_parallel_size") is not None:
            raise ValueError(
                "Configuring data parallelism is not supported; FSDP2 always shards across the "
                f"full world (data_parallel_size = world_size). Leave data_parallel_size unset. "
                f"Got data_parallel_size: {kwargs.get('data_parallel_size')}"
            )

        if kwargs.get("process_group_backend") is not None:
            raise ValueError(
                "Configuring the process group is not supported; Ray Train sets the backend. "
                f"Leave process_group_backend unset. Got process_group_backend: {kwargs.get('process_group_backend')}"
            )

        if kwargs.get("timeout") is not None:
            raise ValueError(
                "Configuring the process group timeout is not supported; Ray Train sets the timeout. "
                f"Leave timeout unset. Got timeout: {kwargs.get('timeout')}"
            )

        super().__init__(*args, **kwargs)

    @property
    def root_device(self) -> torch.device:
        return ray.train.torch.get_device()

    @property
    def distributed_sampler_kwargs(self) -> dict[str, Any]:
        return {"num_replicas": self.world_size, "rank": self.global_rank}

    def setup_environment(self) -> None:
        self._tensor_parallel_size = 1
        self._data_parallel_size = self.world_size
        _logger.info(
            "FSDP2 parallelism resolved: tensor_parallel_size=%d, "
            "data_parallel_size=%d, world_size=%d",
            self._tensor_parallel_size,
            self._data_parallel_size,
            self.world_size,
        )
        super().setup_environment()
        _logger.info("FSDP2 device mesh initialized: %s", self._device_mesh)
