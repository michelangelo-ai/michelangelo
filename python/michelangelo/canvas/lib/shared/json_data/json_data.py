import typing
from enum import Enum

import pydantic
from pydantic import BaseModel, ConfigDict, model_serializer, SerializationInfo
from pydantic.fields import FieldInfo
from pydantic_core import PydanticUndefined

from michelangelo.canvas.lib.shared.json_data.field import field, _OneOf


class JSONData(BaseModel):
    """
    Base class for all canvas 2.0 configuration classes.

    This class is a subclass of pydantic.BaseModel, it
        1) adds support of default values for fields based on field types
        2) adds support of "one of"
        3) only allows a subset of pydantic features that can be safely supported in uniflow go service and starlark
    """

    def __init_subclass__(cls, **kwargs):
        for base in cls.__bases__:
            if JSONData != base and not issubclass(base, JSONData):
                raise TypeError(
                    f"{base.__name__} is not a subclass of {JSONData.__name__}. "
                    f"{JSONData.__name__} classes can only inherit from {JSONData.__name__} or its subclasses."
                )

        # check field types and assign default values
        fields = {}
        annotations = typing.get_type_hints(cls)
        for field_name, field_type in annotations.items():
            if field_name.startswith("_"):
                continue
            # raise exceptions if field_type is not supported
            type_info = _get_type_info(field_name, field_type)

            if field_name in cls.__dict__:
                # field has either field info or default set
                v = getattr(cls, field_name)
                if isinstance(v, FieldInfo):
                    # user set field_info
                    field_info = v
                else:
                    # user set default value
                    field_info = field(default=v)
            else:
                # user did not set either field info or default value
                field_info = field()

            if (
                field_info.default is PydanticUndefined
                and not field_info.json_schema_extra["json_data_field"].get("required")
            ):
                field_info.default = _get_default_value(type_info, field_type)
            setattr(cls, field_name, field_info)
            field_info.json_schema_extra["json_data_field"] |= type_info
            fields[field_name] = (field_type, field_info)

        # add "one of" to json_schema_extra, we will validate "one of" in pydantic.model_validator: cls.__validate__
        json_data_info = {}
        for attr_name, v in cls.__private_attributes__.items():
            if isinstance(v.default, _OneOf):
                oneof = v.default
                for f in oneof.fields:
                    if f not in fields:
                        raise ValueError(
                            f"Field in one_of '{attr_name}' does not exist. No field named '{f}' in class {cls.__name__}."
                        )
                    f_info = fields[f][1]
                    if not f_info.json_schema_extra["json_data_field"].get("nullable"):
                        raise TypeError(
                            f"Field '{f}' in one_of '{attr_name}' is not optional. "
                            "All the fields in oneof must be optional (nullable)."
                        )
                one_of_list = json_data_info.get("oneof", [])
                one_of_list.append(oneof.model_dump())
                json_data_info["oneof"] = one_of_list

        json_schema_extra = {"json_data_object": json_data_info}
        cls.model_config = ConfigDict(json_schema_extra=json_schema_extra)
        cls.__validate__ = pydantic.model_validator(mode="after")(_validate_model_)

    @model_serializer(mode="wrap")
    def serialize_model(
        self, handler, info: SerializationInfo
    ) -> dict[str, typing.Any]:
        dump = handler(self, info)
        if info.context and info.context.get("UniflowCodec", False):
            dump |= {
                "__codec__": "dataclass",
                "__class__": f"{type(self).__module__}.{type(self).__name__}",
            }
        return dump


def _get_type_info(
    field_name: str, field_type: type, position: str = "Field"
) -> dict[str, typing.Any]:
    origin = typing.get_origin(field_type)

    # <type> | None or typing.Optional[type]
    if origin is typing.Union:
        if position in ["List item", "Dict value"]:
            raise TypeError(
                f"{position} type '{field_type}' is not supported in JSONData class. Field: '{field_name}'"
            )
        type_list = typing.get_args(field_type)
        if len(type_list) > 2 or type(None) not in type_list:
            raise TypeError(
                f"Field type '{field_type}' is not supported in JSONData class. Field: '{field_name}'"
            )
        type_info = _get_type_info(
            field_name, next(iter(t for t in type_list if t is not type(None)))
        )
        type_info["nullable"] = True
        return type_info

    # simple types
    if field_type in [bool, int, float, str]:
        return {"type": field_type.__name__}

    # enum
    if isinstance(field_type, type) and issubclass(field_type, Enum):
        # String enum
        if issubclass(field_type, str):
            return {"type": field_type.__name__}
        raise TypeError(
            f"Enum type {field_type} is not supported in JSONData class. "
            f"Only string Enum is supported. Field: '{field_name}'"
        )

    # JSONData class
    if (
        origin is None
        and isinstance(field_type, type)
        and issubclass(field_type, JSONData)
    ):
        return {"type": field_type.__name__}

    if field_type is list or origin is list:
        type_args = typing.get_args(field_type)
        if len(type_args) == 0:
            item_type = typing.Any
        elif len(type_args) == 1:
            item_type = typing.get_args(field_type)[0]
        else:
            raise TypeError(f"Invalid list type: {field_type}. Field: '{field_name}'")

        return {
            "type": "list",
            "item_type": _get_type_info(field_name, item_type, "List item"),
        }

    if field_type is dict or origin is dict:
        type_args = typing.get_args(field_type)
        if len(type_args) == 2:
            key_type = type_args[0]
            value_type = type_args[1]
        elif len(type_args) == 0:
            key_type = str
            value_type = typing.Any
        else:
            raise TypeError(
                f"Invalid dictionary type: {field_type}. Field: '{field_name}'"
            )

        if key_type is not str:
            raise TypeError(
                f"Invalid dictionary type: {field_type}. Dictionary keys must be strings. Field: '{field_name}'"
            )

        return {
            "type": "map",
            "key_type": {"type": "string"},
            "value_type": _get_type_info(field_name, value_type, "Dict value"),
        }

    if field_type is typing.Any:
        return {"type": "any"}

    raise TypeError(
        f"{position} type {field_type} is not supported in JSONData class. Field: '{field_name}'"
    )


def _get_default_value(type_info: dict[str, str], field_type: type) -> typing.Any:
    if type_info.get("nullable"):
        return None

    t = type_info["type"]
    if t == "bool":
        return False
    elif t == "int":
        return 0
    elif t == "float":
        return 0.0
    elif t == "str":
        return ""

    origin = typing.get_origin(field_type)
    if origin is None and isinstance(field_type, type):
        if issubclass(field_type, JSONData):
            return field_type()
        if issubclass(field_type, Enum):
            return next(iter(field_type))

    if field_type is list or origin is list:
        return []
    if field_type is dict or origin is dict:
        return {}

    return None


def _validate_model_(self: BaseModel):
    """
    This function will be called by pydantic.BaseModel to do the json_data specific validations.
    (currently, only oneof validation)
    """
    one_of_list = self.model_config["json_schema_extra"]["json_data_object"].get(
        "oneof", []
    )
    for i in one_of_list:
        one_of = _OneOf(**i)
        set_fields = [f for f in one_of.fields if getattr(self, f) is not None]
        if len(set_fields) == 0 and one_of.required:
            raise ValueError(f"One field in {one_of.fields} must be set (not None).")
        if len(set_fields) > 1:
            raise ValueError(
                f"More than one field in {one_of.fields} are set (not None): {set_fields}."
            )
    return self
