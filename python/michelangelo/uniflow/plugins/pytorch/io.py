"""PyTorchStateDictIO — read/write PyTorch state dicts via torch.save/load.

State dicts are the standard way to checkpoint and restore PyTorch model weights.
This IO handler wraps ``torch.save`` / ``torch.load`` with lazy imports so the
plugin can be imported in environments that do not have PyTorch installed
(the ``ImportError`` is raised only when ``write()`` or ``read()`` is called).

Example:
    >>> import torch, torch.nn as nn, tempfile
    >>> model = nn.Linear(4, 2)
    >>> io = PyTorchStateDictIO()
    >>> path = tempfile.mktemp(suffix=".pt")
    >>> io.write(path, model.state_dict())
    >>> sd = io.read(path, None)
    >>> list(sd.keys())
    ['weight', 'bias']
"""

from __future__ import annotations

from typing import Any

from michelangelo.uniflow.core.io_registry import IO

__all__ = ["PyTorchStateDictIO"]


class PyTorchStateDictIO(IO[dict]):
    """Read/write PyTorch state dicts using ``torch.save`` / ``torch.load``.

    ``torch`` is imported lazily — the plugin is importable without PyTorch
    installed; the ``ImportError`` surfaces only when ``write`` or ``read``
    is called.

    Args:
        map_location: Passed directly to ``torch.load``. Use ``"cpu"`` to load
            weights from a GPU checkpoint in a CPU-only environment. Defaults to
            ``None`` (PyTorch's default behaviour — load to the saved device).

    Example:
        >>> io = PyTorchStateDictIO(map_location="cpu")
        >>> sd = io.read("/tmp/model.pt", None)
    """

    def __init__(self, map_location: Any = None) -> None:
        """Initialise with optional map_location forwarded to torch.load."""
        self._map_location = map_location

    def write(self, url: str, value: dict) -> None:
        """Save *value* (a state dict) to *url* using ``torch.save``.

        Args:
            url: Destination file path. Typically ends in ``.pt`` or ``.pth``.
            value: A ``dict`` mapping parameter names to tensors — the output
                of ``model.state_dict()``.

        Returns:
            ``None`` — no metadata needed for the read path.

        Raises:
            ImportError: If PyTorch is not installed.
        """
        import torch

        torch.save(value, url)
        return None

    def read(self, url: str, _metadata: Any | None) -> dict:
        """Load a state dict from *url* using ``torch.load``.

        Args:
            url: Source file path.
            _metadata: Unused; pass ``None``.

        Returns:
            A ``dict`` mapping parameter names to tensors.

        Raises:
            ImportError: If PyTorch is not installed.
        """
        import torch

        return torch.load(url, map_location=self._map_location, weights_only=True)
