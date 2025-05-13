import logging
from collections.abc import Iterable
from uber.ai.michelangelo.sdk.model_manager.schema import DataType

_logger = logging.getLogger(__name__)


def populate_stage_input_schema(
    stage: any,
    procedure: str,
    data_types: dict[str, DataType],
) -> dict[str, DataType]:
    """
    Populate the input schema for a PipelineModel stage.
    Note, the stage needs to a generated class from java

    Args:
        stage: The stage to populate the input schema for.
        procedure: The procedure to use to populate the input schema.
        data_types: The data types of the input features.

    Returns:
        A dictionary containing the mapping from
        the input feature to the data type.
    """
    try:
        cols = stage._call_java(procedure)
        if stage.__class__.__name__ == "PaletteTransformer":
            cols = []
        _logger.info(f"captured input schema for stage {stage} from {procedure}: {cols}")
    except Exception:
        return {}

    cols = cols if is_iterable(cols) else [cols]

    return {col: data_types[col] for col in cols if col in data_types}


def is_iterable(obj: any) -> bool:
    """
    Check if an object is iterable.

    Args:
        obj: The object to check.

    Returns:
        True if the object is iterable, False otherwise.
    """
    return isinstance(obj, Iterable) and not isinstance(obj, (str, bytes, bytearray))
