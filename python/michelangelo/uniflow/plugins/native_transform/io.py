"""Uniflow IO adapter registration for TransformSpec.

Importing this module registers TransformSpec as an IO-serializable
Uniflow workflow value, so a fitted TransformSpec can be passed between
Uniflow tasks (or persisted to a pipeline artifact store) the same way a
``pandas.DataFrame`` or ``ray.data.Dataset`` is. See
:class:`~michelangelo.lib.native_transform.torch.io.TransformSpecIO` for
the serialization implementation.
"""

from michelangelo.lib.native_transform.torch.io import TransformSpecIO
from michelangelo.lib.native_transform.torch.transform_spec import TransformSpec
from michelangelo.uniflow.core.io_registry import default_io

default_io[TransformSpec] = TransformSpecIO

__all__ = ["TransformSpecIO"]
