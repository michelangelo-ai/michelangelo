"""TransformSpecIO — read/write :class:`TransformSpec` objects via fsspec.

Wraps :meth:`TransformSpec.to_json`/:meth:`TransformSpec.load_from_json` in
the :class:`~michelangelo.uniflow.core.io_registry.IO` protocol so a fitted
``TransformSpec`` can be passed between Uniflow tasks (or persisted to a
pipeline artifact store) as a first-class workflow value, the same way a
``pandas.DataFrame`` or ``ray.data.Dataset`` is.
"""

from __future__ import annotations

from typing import Any

import fsspec.core

from michelangelo.lib.native_transform.torch.transform_spec import TransformSpec
from michelangelo.uniflow.core.io_registry import IO

__all__ = ["TransformSpecIO"]


class TransformSpecIO(IO[TransformSpec]):
    """Read/write :class:`TransformSpec` objects as JSON text via fsspec.

    ``TransformSpec`` already serializes itself to/from JSON
    (:meth:`~TransformSpec.to_json`, :meth:`~TransformSpec.load_from_json`);
    this class only adapts that behavior to the ``IO[T]`` protocol so it can
    be registered in :data:`~michelangelo.uniflow.core.io_registry.default_io`.
    No metadata is needed for the round-trip.

    Example:

    .. code-block:: python

        from michelangelo.lib.native_transform.torch import TransformSpec
        from michelangelo.lib.native_transform.torch.io import TransformSpecIO

        spec = TransformSpec(raw_transform_specs={...})
        io = TransformSpecIO()
        io.write("/tmp/spec.json", spec)
        restored = io.read("/tmp/spec.json", None)
    """

    def write(self, url: str, value: TransformSpec) -> Any | None:
        """Serialize *value* to JSON text at *url*.

        Args:
            url: Destination path or fsspec URL (local, ``s3://``, etc.).
            value: The ``TransformSpec`` to write.

        Returns:
            ``None``. This implementation does not return metadata.
        """
        fs, path = fsspec.core.url_to_fs(url)
        with fs.open(path, "w") as f:
            f.write(value.to_json())
        return None

    def read(self, url: str, _metadata: Any | None) -> TransformSpec:
        """Deserialize a ``TransformSpec`` from JSON text at *url*.

        Args:
            url: Source path or fsspec URL.
            _metadata: Unused; pass ``None``.

        Returns:
            A ``TransformSpec`` restored from the JSON written by
            :meth:`write`.
        """
        fs, path = fsspec.core.url_to_fs(url)
        with fs.open(path, "r") as f:
            json_str = f.read()
        # load_from_json() is an instance method that replaces state in place
        # (mirroring TransformSpec's other update_* mutators), so bypass
        # __init__ -- which otherwise requires a yaml path or raw spec dict --
        # and populate the instance entirely from the serialized JSON.
        spec = TransformSpec.__new__(TransformSpec)
        spec.load_from_json(json_str)
        return spec
