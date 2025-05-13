from pyspark.sql.functions import to_json, struct
from pyspark.sql import DataFrame, Column


def create_sample_data_csv(
    sample_data: DataFrame,
    csv_path: str,
):
    """
    Create a CSV file from the sample data data frame.
    Only the first 5 rows of the sample data will be saved.

    Args:
        sample_data: A DataFrame containing the sample data.
        csv_path: The path to save the CSV file.

    Returns:
        None
    """
    selectors = [get_col_selector(*item) for item in sample_data.dtypes]

    df = sample_data.limit(5)
    df.select(selectors).toPandas().to_csv(csv_path, index=False)
    return csv_path


def get_col_selector(col_name: str, dtype: str) -> Column:
    """
    Convert the vector column to a json formatted column.

    Args:
        col_name: The column name.
        dtype: The column data type.

    Returns:
        The column selector.
    """
    return to_json(struct(col_name)).alias(col_name) if dtype == "vector" else col_name
