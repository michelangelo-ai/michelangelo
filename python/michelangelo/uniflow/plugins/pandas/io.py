from typing import Any, Optional

import pandas as pd
from pandas import DataFrame

from michelangelo.uniflow.core.io_registry import IO


class PandasIO(IO[DataFrame]):
    def __init__(self, storage_options: Optional[dict[str, Any]] = None):
        """
        :param storage_options: FSSPEC storage options. See https://filesystem-spec.readthedocs.io/en/latest/api.html
        """
        self._storage_options = storage_options

    def write(self, url: str, value: DataFrame) -> Optional[Any]:
        value.to_parquet(url, storage_options=self._storage_options)
        return None

    def read(self, path: str, _metadata) -> DataFrame:
        return pd.read_parquet(path, storage_options=self._storage_options)
