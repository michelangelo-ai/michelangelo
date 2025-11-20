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
    """
    Specify the default value and / or validation rules for a field.
    Similar to dataclasses.field() and pydantic.field().

    :param default: default value for the field. If set to ellipsis(...), the field will have no default value, and
                    users must specify a value. If not set, the default value will be inferred from the field type.
                    (i.e. 0 for int, 0.0 for float, False for bool, "" for str, Class() for user defined class, etc.)
    :param pattern: regular expression pattern used to validate str field.
    :param gt: greater than
    :param ge: greater or equal than
    :param lt: less than
    :param le: less or equal than
    :param min_length: minimum length of a list, str, or dict field
    :param max_length: maximum length of a list, str, or dict field
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
    """
    Specify a one of validation rule. At most one field in the fields list may be set (not None).
    :param fields: A list of field names.
    :param required: If True, at least one field in the fields list must be not None.
    :return:
    """
    return _OneOf(fields=fields, required=required)
