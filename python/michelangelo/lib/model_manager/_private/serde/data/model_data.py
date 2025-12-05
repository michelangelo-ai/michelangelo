"""Model data serialization and deserialization."""

import json
import numpy as np
from typing import Optional, TextIO, Union
from .encoder import DataEncoder


def dump_model_data(
    data: Union[dict[str, np.ndarray], list[dict[str, np.ndarray]]],
    fs: Optional[TextIO] = None,
    indent: Optional[int] = None,
) -> Optional[str]:
    """Save the data to a file or return the data in json format

    Args:
        data: The model input/output data. Can be a single record or a list of records.
            Each record is a dictionary where the keys are the feature names and the values are the feature values
        fs: The file stream to save the sample data. If not specified, return the sample data in json format
        indent: The number of spaces to indent the json data

    Returns:
        The encoded data in json format if fs is not specified. Otherwise, return None
    """
    if fs:
        json.dump(data, fs, cls=DataEncoder, indent=indent)
    else:
        return json.dumps(data, cls=DataEncoder, indent=indent)


def load_model_data(
    fs: TextIO,
) -> Union[dict[str, np.ndarray], list[dict[str, np.ndarray]]]:
    """Load data of the model data from a file

    Args:
        fs: The file stream to load the data

    Returns:
        The loaded model data.
        If the data is a single record, return a dictionary where the keys are the feature names and the values are the feature values
        If the data is a list of records, return a list of the records
    """
    data = json.load(fs)
    return convert_data_items_to_numpy(data)


def get_model_data(
    json_data: str,
) -> Union[dict[str, np.ndarray], list[dict[str, np.ndarray]]]:
    """Get the a single record of the model data from the json string

    Args:
        json_data: The json string containing the model data

    Returns:
        The model data
        If the data is a single record, return a dictionary where the keys are the feature names and the values are the feature values
        If the data is a list of records, return a list of the records
    """
    data = json.loads(json_data)
    return convert_data_items_to_numpy(data)


def convert_data_items_to_numpy(
    data: Union[dict[str, list], list[dict[str, list]]],
) -> Union[dict[str, np.ndarray], list[dict[str, np.ndarray]]]:
    """Convert the data items to numpy arrays

    Args:
        data: The data to convert

    Returns:
        The data with the values converted to numpy arrays
    """
    if isinstance(data, list):
        return [
            {key: np.array(value) for key, value in record.items()} for record in data
        ]
    return {key: np.array(value) for key, value in data.items()}
