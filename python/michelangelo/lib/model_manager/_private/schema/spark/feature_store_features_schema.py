from typing import Optional
from uber.ai.michelangelo.shared.utils.palette_utils import get_palette_expressions
from michelangelo.lib.model_manager.schema import DataType

DSL_PREFIX = "derived_"


def populate_stage_feature_store_features_schema(
    stage: any,
    data_types: dict[str, DataType],
    include_palette_features_with_derived_join_keys: Optional[bool] = False,
) -> dict[str, DataType]:
    """
    Populate the feature store features schema for a PipelineModel stage.
    Note, the stage needs to a generated class from java

    Args:
        stage: The stage to populate the feature store features schema for.
        data_types: The data types of the feature store features.
        include_palette_features_with_derived_join_keys: Whether to include palette expr whose join key is a derived feature

    Returns:
        A dictionary containing the mapping from
        the feature store feature to the data type.
    """
    palette_exprs = set()

    # if palette feature is provided with basis features, PaletteTransformer may not exist
    # it is also possible that palette feature is not consumed by DSL
    # so we need to get palette features from both transformers
    if stage.__class__.__name__ == "PaletteTransformer":
        palette_map = stage._java_obj.getPaletteMap()
        palette_exprs.update(palette_map.values())

    elif stage.__class__.__name__ == "MichelangeloDSLModel":
        lambdas = stage._java_obj.getLambdas()
        for expr in lambdas.values():
            palette_exprs.update(get_palette_expressions(expr))

    return {
        palette_expr: data_types[palette_expr]
        for palette_expr in palette_exprs
        # if palette expr has join key as a derived feature,
        # by default, it should not be part of feature store features
        if palette_expr in data_types and (include_palette_features_with_derived_join_keys or DSL_PREFIX not in palette_expr)
    }
