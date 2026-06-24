"""Tests for type_yaml generation."""

from michelangelo.lib.model_manager._private.packager.torch_triton.type_yaml import (
    generate_type_yaml,
)
from michelangelo.lib.model_manager.constants import RawModelType


def test_generate_type_yaml_returns_torch_type():
    """generate_type_yaml produces a YAML string with the torch model type."""
    result = generate_type_yaml()
    assert isinstance(result, str)
    assert RawModelType.TORCH in result
