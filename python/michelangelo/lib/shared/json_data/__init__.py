"""JSONData base class and field helpers for structured configuration."""

from .field import field, one_of
from .json_data import JSONData

__all__ = ["JSONData", "field", "one_of"]
