"""Field descriptors and one-of constraints for JSONData models."""

from typing import Any, Optional

import pydantic
from pydantic_core import PydanticUndefined


def field(
    default: Any = PydanticUndefined,
    *,
    pattern: Optional[str] = PydanticUndefined,
    gt: Optional[float] = PydanticUndefined,
    ge: Optional[float] = PydanticUndefined,
    lt: Optional[float] = PydanticUndefined,
    le: Optional[float] = PydanticUndefined,
    min_length: Optional[int] = PydanticUndefined,
    max_length: Optional[int] = PydanticUndefined,
) -> Any:
    """Specify the default value and validation rules for a field.

    Similar to ``dataclasses.field()`` and ``pydantic.Field()``.

    Args:
        default: Default value for the field. Ellipsis (``...``)
            means required. If omitted, inferred from the type.
        pattern: Regex pattern to validate a str field.
        gt: Greater than.
        ge: Greater than or equal.
        lt: Less than.
        le: Less than or equal.
        min_length: Minimum length of a list, str, or dict field.
        max_length: Maximum length of a list, str, or dict field.
    """
    json_data_info = {}
    if default is Ellipsis:
        json_data_info["required"] = True

    return pydantic.Field(
        default=default,
        pattern=pattern,
        gt=gt,
        ge=ge,
        lt=lt,
        le=le,
        min_length=min_length,
        max_length=max_length,
        json_schema_extra={"json_data_field": json_data_info},
    )


class _OneOf(pydantic.BaseModel):
    fields: list[str] = pydantic.Field(min_length=2)
    required: bool = True


def one_of(fields: list[str], required: bool = True) -> _OneOf:
    """Specify a one-of validation rule.

    At most one field in the list may be set (not None).

    Args:
        fields: A list of field names.
        required: If True, at least one field must be not None.
    """
    return _OneOf(fields=fields, required=required)
