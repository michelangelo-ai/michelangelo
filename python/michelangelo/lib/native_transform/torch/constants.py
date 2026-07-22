"""Shared constants for PyTorch native transform layers.

Module-level constants, default values, and dtype-mapping dictionaries used
throughout the ``native_transform`` torch package. These are leaf definitions
with no dependencies on the rest of the package so every downstream module can
import them safely.
"""

from __future__ import annotations

import torch

SHAPE = "shape"
TENSOR_COMPILATION_TORCH_MINIMUM_VERSION = "2.3"
DEFAULT_NUM_PARTITIONS = 100
DEFAULT_PARQUET_BLOCK_SIZE = 512
DEFAULT_NUM_SAMPLES_TO_EXPLAIN = 10000
DEFAULT_NUMERICAL_OUTPUT_DTYPE = torch.float32
DEFAULT_STRING_OUTPUT_DTYPE = "string"

STRING_DATA_TYPE_TO_TORCH_TYPE_MAP: dict[str, torch.dtype] = {
    "float32": torch.float32,
    "float64": torch.float64,
    "int32": torch.int32,
    "int64": torch.int64,
    "bool": torch.bool,
}

TORCH_DTYPE_CLASS_NAME_TO_TORCH_TYPE_MAP: dict[str, torch.dtype | str] = {
    "torch.int32": torch.int32,
    "torch.int64": torch.int64,
    "torch.float32": torch.float32,
    "torch.float64": torch.float64,
    "torch.bool": torch.bool,
    "string": "string",
}

TORCH_TYPE_TO_TORCH_DTYPE_CLASS_NAME_MAP: dict[torch.dtype | str, str] = {
    torch.int32: "torch.int32",
    torch.int64: "torch.int64",
    torch.float32: "torch.float32",
    torch.float64: "torch.float64",
    torch.bool: "torch.bool",
    "string": "string",
}

NUMERICAL_TRANSFORM = "numerical_transform"
CATEGORICAL_TRANSFORM = "categorical_transform"
STRING_LENGTH_TRANSFORM = "string_length_transform"
NUM_TO_CAT_TRANSFORM = "num_to_cat_transform"
IS_NULL_TRANSFORM = "is_null_transform"
BOOL_TO_FLOAT_TRANSFORM = "bool_to_float_transform"
STRING_VALUE_CHECK_TRANSFORM = "string_value_check_transform"
STRING_VALUE_CONTAINS_TRANSFORM = "string_value_contains_transform"
STRING_VALUE_EQUAL_TRANSFORM = "string_value_equal_transform"
NUMERICAL_DIFF_TRANSFORM = "numerical_diff_transform"
GEO_DISTANCE_TRANSFORM = "geo_distance_transform"
GIBBERISH_SCORE_TRANSFORM = "gibberish_score_transform"
EDIT_DISTANCE_TRANSFORM = "edit_distance_transform"
STRING_CONTAINS_TRANSFORM = "string_contains_transform"
FEATURE_PAIR_MATCH_TRANSFORM = "feature_pair_match_transform"
FEATURE_DIVIDE_TRANSFORM = "feature_divide_transform"
STRING_SEQUENCE_TRANSFORM = "string_sequence_transform"
SIMPLE_FEATURE_ASSEMBLER = "simple_feature_assembler"
STANDARD_SCALER = "StandardScaler"
MIN_MAX_SCALER = "MinMaxScaler"
DEFAULT_VECTOR_TYPE_DICT: dict[str, str] = {
    "scalar_output_vec": STANDARD_SCALER,
    "categorical_output_vec": SIMPLE_FEATURE_ASSEMBLER,
    "ratio_output_vec": SIMPLE_FEATURE_ASSEMBLER,
    "boolean_output_vec": SIMPLE_FEATURE_ASSEMBLER,
}

DEFAULT_NULL_STRING = "null_string"
DEFAULT_NUMERICAL_VALUE = -1.0
DEFAULT_EMAIL_GIBBERISH_SCORE = 0.05
DEFAULT_NAME_GIBBERISH_SCORE = 0.113792509289
DEFAULT_STANDARD_SCALER_NAME = "StandardScaler"
DEFAULT_MIN_MAX_SCALER_NAME = "MinMaxScaler"
DEFAULT_STRING_INDEXER_NAME = "StringIndexer"
DEFAULT_TIME_DURATION_UNIT = 24 * 60 * 60 * 1000
DEFAULT_EPSILON = 1e-7
