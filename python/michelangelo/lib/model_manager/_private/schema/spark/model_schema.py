import logging
from typing import Optional
from pyspark.ml.pipeline import PipelineModel
from pyspark.sql import DataFrame
from michelangelo.lib.model_manager.schema import (
    ModelSchema,
    ModelSchemaItem,
    DataType,
)
from uber.ai.michelangelo.shared.utils.palette_utils import get_palette_expressions
from michelangelo.lib.model_manager._private.schema.spark.input_schema import (
    populate_stage_input_schema,
)
from michelangelo.lib.model_manager._private.schema.spark.feature_store_features_schema import (
    populate_stage_feature_store_features_schema,
)
from michelangelo.lib.model_manager._private.schema.spark.dtype_mapping import DTYPE_MAPPING

_logger = logging.getLogger(__name__)


def create_model_schema(
    model: PipelineModel,
    df: DataFrame,
    include_palette_features_with_derived_join_keys: Optional[bool] = False,
) -> ModelSchema:
    """
    Create a model schema from a spark pipeline model and a DataFrame.

    Args:
        model: A trained model.
        df: A DataFrame containing the input data.
        include_palette_features_with_derived_join_keys: Whether to include palette expr whose join key is a derived feature

    Returns:
        A model schema.
    """
    model_input_data_types: dict[str, DataType] = {}
    model_feature_store_feature_data_types: dict[str, DataType] = {}

    # Get the input data types from the DataFrame
    input_data_types: dict[str, DataType] = {
        col: DTYPE_MAPPING.get(dtype, DataType.UNKNOWN) for col, dtype in dict(df.dtypes).items() if not get_palette_expressions(col)
    }

    txed_df = df

    for stage in model.stages:
        stage_input_data_types = {
            **populate_stage_input_schema(stage, "getInputCols", input_data_types),
            **populate_stage_input_schema(stage, "getInputCol", input_data_types),
            **populate_stage_input_schema(stage, "getFeaturesCol", input_data_types),
        }

        if not stage_input_data_types:
            _logger.warning(f"Unable to capture input schema for stage {stage}")

        txed_df = stage.transform(txed_df)
        txed_data_types: dict[str, DataType] = {col: DTYPE_MAPPING.get(dtype, DataType.UNKNOWN) for col, dtype in dict(txed_df.dtypes).items()}
        stage_palette_data_types = populate_stage_feature_store_features_schema(stage, txed_data_types, include_palette_features_with_derived_join_keys)

        model_input_data_types.update(stage_input_data_types)
        model_feature_store_feature_data_types.update(stage_palette_data_types)

    _logger.info(f"model input schema: {model_input_data_types}")
    _logger.info(f"model feature store feature schema: {model_feature_store_feature_data_types}")

    input_schema = [ModelSchemaItem(name=col, data_type=data_type) for col, data_type in model_input_data_types.items()]

    feature_store_features_schema = [ModelSchemaItem(name=col, data_type=data_type) for col, data_type in model_feature_store_feature_data_types.items()]

    return ModelSchema(
        input_schema=input_schema,
        feature_store_features_schema=feature_store_features_schema,
    )
