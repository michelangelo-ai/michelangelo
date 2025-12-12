"""Model schema item definition."""

from dataclasses import dataclass

from michelangelo.lib.model_manager.schema.data_type import DataType


@dataclass
class ModelSchemaItem:
    """Represents a single feature in a model schema.

    A ModelSchemaItem defines the metadata for one feature (input, output, or
    palette feature) in a model schema. It specifies the feature's name, data
    type, shape, and whether it's required or optional.

    The shape follows NumPy array conventions, where each dimension is specified
    as an integer. Variable-length dimensions can be indicated using -1.

    Examples:
        Scalar integer feature:
            ModelSchemaItem(name="user_id", data_type=DataType.LONG, shape=[1])

        Fixed-size vector of 10 floats:
            ModelSchemaItem(name="embeddings", data_type=DataType.FLOAT,
                          shape=[10])

        2D array (10 rows, 5 columns):
            ModelSchemaItem(name="features", data_type=DataType.DOUBLE,
                          shape=[10, 5])

        Variable-length sequence of integers:
            ModelSchemaItem(name="sequence", data_type=DataType.INT,
                          shape=[-1])

        Optional string feature:
            ModelSchemaItem(name="description", data_type=DataType.STRING,
                          shape=[1], optional=True)

    Attributes:
        name: The name of the feature. This should be a valid Python identifier
            and will be used as the key in input/output dictionaries.
        data_type: The data type of the feature, specified as a DataType enum
            value. Defaults to DataType.UNKNOWN if not specified.
        shape: The shape of the feature as a list of integers following NumPy
            conventions. For example:
            - [1] for a scalar value
            - [n] for a 1D array of length n
            - [n, m] for a 2D array with n rows and m columns
            - [-1] for a variable-length dimension
            If None, the shape is unspecified.
        optional: Whether this feature is optional. If True, the feature may be
            omitted from input data. If False or None, the feature is required.
            Defaults to None (treated as required).
    """

    name: str
    data_type: DataType = DataType.UNKNOWN
    shape: list[int] = None
    optional: bool = None
