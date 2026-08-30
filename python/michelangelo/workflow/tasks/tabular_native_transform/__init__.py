"""Tabular native transform workflow task."""

from michelangelo.workflow.schema.exceptions import ConfigurationError
from michelangelo.workflow.schema.tabular_native_transform import (
    BatchOptions,
    IncrementalTrainingConfig,
    ParquetReadConfig,
    RayDataContextConfig,
    TabularNativeTransformConfig,
    TrainingType,
    WriteConfig,
)
from michelangelo.workflow.tasks.tabular_native_transform._private import (
    incremental_training as _incremental_training,
)
from michelangelo.workflow.tasks.tabular_native_transform._private.utils import (
    convert_to_numpy_sample,
    get_sample_data_from_datasets,
    resolve_data_file_path,
)
from michelangelo.workflow.tasks.tabular_native_transform.task import (
    tabular_native_transform,
)

is_baseline = _incremental_training.is_baseline
is_incremental = _incremental_training.is_incremental
load_incremental_artifacts = _incremental_training.load_incremental_artifacts
merge_specs_for_selective_refit = _incremental_training.merge_specs_for_selective_refit

__all__ = [
    "BatchOptions",
    "ConfigurationError",
    "IncrementalTrainingConfig",
    "ParquetReadConfig",
    "RayDataContextConfig",
    "TabularNativeTransformConfig",
    "TrainingType",
    "WriteConfig",
    "convert_to_numpy_sample",
    "get_sample_data_from_datasets",
    "is_baseline",
    "is_incremental",
    "load_incremental_artifacts",
    "merge_specs_for_selective_refit",
    "resolve_data_file_path",
    "tabular_native_transform",
]
