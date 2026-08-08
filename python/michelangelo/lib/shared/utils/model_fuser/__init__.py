"""Model fusion utilities for combining a native-transform model with a predictor.

``FusedModel`` composes a transform module and a predictor module into a
single ``nn.Module`` for serving. ``fuse`` traces/exports the composed graph
to TorchScript, ONNX, or a combined state dict for Python-backend serving;
its public functions are re-exported here for convenience.
"""

from .fuse import (
    build_fused_sample_data,
    compute_python_fuse_metadata,
    fuse_models_to_onnx,
    fuse_models_to_python,
    fuse_models_to_torchscript,
    get_predictor_output_field_order,
)
from .fuse_schema import fuse_input_schema, fuse_model_schema
from .fused_model import FusedModel

__all__ = [
    "FusedModel",
    "build_fused_sample_data",
    "compute_python_fuse_metadata",
    "fuse_input_schema",
    "fuse_model_schema",
    "fuse_models_to_onnx",
    "fuse_models_to_python",
    "fuse_models_to_torchscript",
    "get_predictor_output_field_order",
]
