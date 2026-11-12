"""Models packaged by the Ray inference tests."""

from __future__ import annotations

import numpy as np
import torch

from michelangelo.lib.model_manager.interface.custom_model import Model


class Predict(Model):
    """Custom model that echoes ``input`` as ``response``."""

    def save(self, path: str):
        assert path

    @classmethod
    def load(cls, path) -> Predict:
        assert path
        return Predict()

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        return {"response": inputs.get("input")}


class PredictWithMultiDimFeature(Model):
    """Custom model that echoes its three inputs."""

    def save(self, path: str):
        assert path

    @classmethod
    def load(cls, path) -> PredictWithMultiDimFeature:
        assert path
        return PredictWithMultiDimFeature()

    def predict(self, inputs: dict[str, np.ndarray]) -> dict[str, np.ndarray]:
        return {
            "response": inputs.get("input"),
            "response_1": inputs.get("input_1"),
            "response_2": inputs.get("input_2"),
        }


class PositionalTorchModel(torch.nn.Module):
    """Returns ``2 * a + b`` from one positional argument per input."""

    def forward(self, a: torch.Tensor, b: torch.Tensor) -> torch.Tensor:
        return 2 * a + b


class DictTorchModel(torch.nn.Module):
    """Returns ``2 * a + b`` from a dict of named inputs."""

    def forward(self, inputs: dict[str, torch.Tensor]) -> torch.Tensor:
        return 2 * inputs["a"] + inputs["b"]


from michelangelo.lib.artifact_manager.storage_backend import StorageBackend  # noqa: E402
import os, shutil, tarfile  # noqa: E402


class LocalDirStorageBackend(StorageBackend):
    """Serves a local directory as the artifact at any URI."""

    def __init__(self, source: str) -> None:
        self._source = source

    def upload(self, local_path: str, destination_key: str) -> str:
        raise NotImplementedError

    def download(self, uri: str, local_path: str) -> None:
        shutil.copytree(self._source, local_path)


class LocalTarStorageBackend(StorageBackend):
    """Serves a local directory as a single ``model.tar`` file at any URI."""

    def __init__(self, source: str, archive: str) -> None:
        self._archive = archive
        with tarfile.open(archive, "w") as tar:
            for name in os.listdir(source):
                tar.add(os.path.join(source, name), arcname=name)

    def upload(self, local_path: str, destination_key: str) -> str:
        raise NotImplementedError

    def download(self, uri: str, local_path: str) -> None:
        shutil.copy2(self._archive, local_path)
