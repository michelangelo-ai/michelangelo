from dataclasses import dataclass
import json
import pydantic
from typing import Any, Optional
from unittest import TestCase

from michelangelo.uniflow.core.codec import encoder, decoder
from enum import Enum


class Color(Enum):
    RED = 1
    GREEN = 2
    BLUE = 3


@dataclass
class Entity:
    """
    Dataclass to represent a generic entity for testing purposes.
    """

    id: str  # Unique identifier for the entity. Simple string field.
    value: Any  # Entity value. Can be any type.
    children: Optional[list["Entity"]] = None  # Optional list of child entities.
    _private_field: Optional[str] = None


class PydanticEnity(pydantic.BaseModel):
    id: str
    index: pydantic.PositiveInt
    parent: Optional["PydanticEnity"] = None


def user_defined_function():  # User-defined function for testing purposes.
    pass


class CustomClass:  # Custom non-dataclass class for testing purposes.
    def __init__(self, value):
        self.value = value


class Test(TestCase):
    def test_encode_decode(self):
        # Encode a complex data structure with nested dataclasses, type variables, etc.
        data = {
            "entity_type": Entity,  # Class reference
            "entity": Entity(  # Top-level dataclass instance
                id="root",
                value=0,  # Value can be any type. An integer in this case, or a dict like in the nested child entity.
                children=[
                    Entity(  # Nested dataclass instance
                        id="child_1",
                        value={
                            "status": 200,
                            "function_ref": user_defined_function,  # Function reference
                            "color": Color.RED,  # Enum value
                            "bytes": b"Hello World",
                        },
                    ),
                    Entity(
                        id="child_2",
                        value=PydanticEnity(
                            id="pydantic_entity",
                            index=101,
                            parent=PydanticEnity(
                                id="parent_pydantic_entity",
                                index=100,
                            ),
                        ),
                    ),
                ],
                _private_field="secret",
            ),
        }
        encoded_data = encoder.encode(data)
        self.assertIsInstance(encoded_data, str)

        # Compare the encoded result with the expected JSON object.
        expected_json = {
            "entity_type": {
                "path": "test_codec.Entity",
                "__codec__": "type",
            },
            "entity": {
                "id": "root",
                "value": 0,
                "children": [
                    {
                        "id": "child_1",
                        "value": {
                            "status": 200,
                            "function_ref": {
                                "path": "test_codec.user_defined_function",
                                "__codec__": "type",
                            },
                            "color": {
                                "name": "RED",
                                "value": 1,
                                "__codec__": "enum",
                                "__class__": "test_codec.Color",
                            },
                            "bytes": {
                                "__codec__": "bytes",
                                "value": "SGVsbG8gV29ybGQ=",
                            },
                        },
                        "children": None,
                        "_private_field": None,  # private fields are encoded/decoded in the same way as public fields
                        "__class__": "test_codec.Entity",
                        "__codec__": "dataclass",
                    },
                    {
                        "id": "child_2",
                        "value": {
                            "id": "pydantic_entity",
                            "index": 101,
                            "parent": {
                                "id": "parent_pydantic_entity",
                                "index": 100,
                                "parent": None,
                                "__class__": "test_codec.PydanticEnity",
                                "__codec__": "pydantic",
                            },
                            "__class__": "test_codec.PydanticEnity",
                            "__codec__": "pydantic",
                        },
                        "children": None,
                        "_private_field": None,  # private fields are encoded/decoded in the same way as public fields
                        "__class__": "test_codec.Entity",
                        "__codec__": "dataclass",
                    },
                ],
                "_private_field": "secret",
                "__class__": "test_codec.Entity",
                "__codec__": "dataclass",
            },
        }

        self.maxDiff = None
        self.assertEqual(expected_json, json.loads(encoded_data))

        # Decode the encoded result back into a Python object.
        decoded_data = decoder.decode(encoded_data)
        self.assertEqual(data, decoded_data)

    def test_not_json_serializable_type(self):
        data = CustomClass("test")
        with self.assertRaisesRegex(TypeError, "CustomClass is not JSON serializable"):
            encoder.encode(data)
