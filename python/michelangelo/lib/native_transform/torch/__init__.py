"""PyTorch native transform layers.

TorchScript- and ONNX-exportable ``nn.Module`` layers used to build native
feature transforms that run identically at train and serve time.
"""

from michelangelo.lib.native_transform.torch.base_layers import (
    CaseWhen,
    Cast,
    Ceil,
    Clip,
    Compare,
    Concatenate,
    Constant,
    Divide,
    Floor,
    IdentityTransform,
    IDHashTokenizer,
    LogTransform,
    PadOrCrop1D,
    Scale,
    Stack,
    Subtract,
    TensorColFillNone,
    Tile,
    TorchTransformBaseLayer,
)
from michelangelo.lib.native_transform.torch.base_transform_module import (
    TorchTransformModule,
    get_transform_module,
)
from michelangelo.lib.native_transform.torch.constants import (
    TORCH_TYPE_TO_TORCH_DTYPE_CLASS_NAME_MAP,
)
from michelangelo.lib.native_transform.torch.duration import TimeDuration
from michelangelo.lib.native_transform.torch.io import TransformSpecIO
from michelangelo.lib.native_transform.torch.scale import ClipAndScale
from michelangelo.lib.native_transform.torch.stats_layers import (
    Bucketization,
    MinMax,
    Normalization,
)
from michelangelo.lib.native_transform.torch.transform_spec import (
    TORCH_TRANSFORM_LAYERS_DICT,
    TORCH_TRANSFORM_LAYERS_SPECS_DICT,
    TransformSpec,
)
from michelangelo.lib.native_transform.torch.transform_utils import (
    generate_cast_transformation,
    generate_concatenation_transformation,
    generate_duration_transformation,
    generate_idhash_tokenization_transformation,
    generate_numerical_scaled_transformation,
    update_output_tensor_map,
)
from michelangelo.lib.native_transform.torch.utils import generate_layer_name
from michelangelo.uniflow.core.io_registry import default_io

# Register TransformSpec as a first-class IO-serializable workflow value, so
# it can be passed between Uniflow tasks the same way a DataFrame or Dataset
# is (see michelangelo.uniflow.core.io_registry.default_io).
default_io[TransformSpec] = TransformSpecIO

__all__ = [
    "TORCH_TRANSFORM_LAYERS_DICT",
    "TORCH_TRANSFORM_LAYERS_SPECS_DICT",
    "TORCH_TYPE_TO_TORCH_DTYPE_CLASS_NAME_MAP",
    "Bucketization",
    "CaseWhen",
    "Cast",
    "Ceil",
    "Clip",
    "ClipAndScale",
    "Compare",
    "Concatenate",
    "Constant",
    "Divide",
    "Floor",
    "IDHashTokenizer",
    "IdentityTransform",
    "LogTransform",
    "MinMax",
    "Normalization",
    "PadOrCrop1D",
    "Scale",
    "Stack",
    "Subtract",
    "TensorColFillNone",
    "Tile",
    "TimeDuration",
    "TorchTransformBaseLayer",
    "TorchTransformModule",
    "TransformSpec",
    "TransformSpecIO",
    "generate_cast_transformation",
    "generate_concatenation_transformation",
    "generate_duration_transformation",
    "generate_idhash_tokenization_transformation",
    "generate_layer_name",
    "generate_numerical_scaled_transformation",
    "get_transform_module",
    "update_output_tensor_map",
]
