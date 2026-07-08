"""Toy PyTorch model fixtures for torch_triton packager tests."""

from __future__ import annotations

import torch


class SimpleModel(torch.nn.Module):
    """A tiny two-layer linear network used as a TorchScript/ONNX fixture.

    The network maps a fixed-size feature vector to a fixed-size output vector
    and is small enough to script, trace, and export quickly in tests.

    Attributes:
        fc1: The first linear layer.
        fc2: The second linear layer.
    """

    def __init__(self, in_features: int = 4, hidden: int = 8, out_features: int = 2):
        """Build the two-layer network.

        Args:
            in_features: Size of the input feature vector.
            hidden: Size of the hidden layer.
            out_features: Size of the output vector.
        """
        super().__init__()
        self.fc1 = torch.nn.Linear(in_features, hidden)
        self.fc2 = torch.nn.Linear(hidden, out_features)

    def forward(self, x: torch.Tensor) -> torch.Tensor:
        """Run a forward pass.

        Args:
            x: Input tensor of shape ``[batch_size, in_features]``.

        Returns:
            Output tensor of shape ``[batch_size, out_features]``.
        """
        return self.fc2(torch.relu(self.fc1(x)))


class NotAModule:
    """A plain class that is not a ``torch.nn.Module``, used for failure tests."""


def save_state_dict(path: str) -> None:
    """Save a SimpleModel state_dict to disk.

    Args:
        path: Destination ``.pt`` / ``.pth`` path.
    """
    torch.save(SimpleModel().state_dict(), path)


def save_full_model(path: str) -> None:
    """Save a pickled full SimpleModel (nn.Module) to disk.

    Args:
        path: Destination ``.pt`` path.
    """
    torch.save(SimpleModel(), path)


def save_scripted_model(path: str) -> None:
    """Save a TorchScript-scripted SimpleModel to disk.

    Args:
        path: Destination ``.pt`` path.
    """
    model = SimpleModel()
    model.eval()
    torch.jit.save(torch.jit.script(model), path)
