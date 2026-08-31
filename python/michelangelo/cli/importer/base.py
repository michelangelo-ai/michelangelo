"""Shared types for ``ma import`` manifest converters."""

from dataclasses import dataclass


class ManifestError(ValueError):
    """Raised when the input manifest cannot be converted."""


@dataclass
class ConversionResult:
    """The generated file text plus everything that did not map."""

    scaffold: str
    warnings: list
