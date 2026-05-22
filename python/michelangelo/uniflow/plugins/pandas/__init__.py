"""Pandas plugin for Michelangelo Uniflow.

This package provides pandas DataFrame I/O support for Uniflow workflows.
It reads and writes DataFrames in Parquet format using PyArrow with zstd
compression, supporting local and remote filesystems via fsspec.
"""

from michelangelo.uniflow.plugins.pandas.io import PandasIO

__all__ = [
    "PandasIO",
]
