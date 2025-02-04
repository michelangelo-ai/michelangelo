import os
from typing import Optional, Any

import ray
from ray.data import Dataset

from michelangelo.uniflow.core.io_registry import IO

UF_PLUGIN_RAY_USE_FSSPEC = "UF_PLUGIN_RAY_USE_FSSPEC"
"""
UF_PLUGIN_RAY_USE_FSSPEC is an environment variable that controls whether the Ray Plugin uses fsspec instead of Ray's
default filesystem - pyarrow. Possible values:
  - 1 to use fsspec
  - 0 to use Ray's default filesystem.

Default is 0.
"""

_DEFAULT_UF_PLUGIN_RAY_USE_FSSPEC = "0"


class RayDatasetIO(IO[Dataset]):

    def write(self, url: str, ds: Dataset) -> Optional[Any]:
        fs, path = _fs_path(url)
        ds.write_parquet(path, filesystem=fs)
        metadata = None
        return metadata

    def read(self, url: str, metadata: Optional[Any]) -> Dataset:
        assert metadata is None
        fs, path = _fs_path(url)
        return ray.data.read_parquet(path, filesystem=fs)


def _fs_path(url: str) -> tuple[Any, str]:
    if os.environ.get(UF_PLUGIN_RAY_USE_FSSPEC, "0") == "1":
        import fsspec

        return fsspec.core.url_to_fs(url)

    return None, url
