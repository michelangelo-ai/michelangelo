from enum import Enum
import json
import typing
from typing import Optional

import pydantic

from michelangelo.canvas.lib.shared.json_data import field, one_of, JSONData

from unittest import TestCase


class FruitEnum(str, Enum):
    pear = "pear"
    banana = "banana"
    apple = "apple"


class C(JSONData):
    c0: int
    c1: str
    c2: float
    _c3: int  # hidden field will not show up in json schema


class B(C):
    b0: int
    b1: str = "b1"
    list0: list[int]


class A(JSONData):
    # Default value of an int field is 0 is not otherwise specified
    num1: int
    # Optional field is annotated as '<type> | None'.
    # If not specified, the default value of optional field is None.
    num3: Optional[int]
    # set default value and validation rules using field
    # a float number in [0.0, 1.0)
    num4: float = field(default=0.5, ge=0.0, lt=1.0)
    # set default value to ... requires users explicitly set this field
    num5: int = field(default=...)

    # one and only one filed in the list must be set (not None)
    _one_of_str12 = one_of(fields=["str1", "str2"], required=True)
    # optional string field <= 30 characters
    str1: Optional[str] = field(max_length=30)
    # optional string field validated with a regular expression
    str2: Optional[str] = field(pattern=r"\w+")

    _one_of_n132 = one_of(fields=["f1", "f2", "f3"], required=False)
    f1: Optional[int]
    f2: Optional[float]
    f3: Optional[int]

    # default value of bool field is false
    bool1: bool

    # Only support string enum. If not specified, the default is the 1st member of the enum.
    enum1: FruitEnum = field(default=FruitEnum.banana)
    enum2: FruitEnum

    # another JSONData class. The default value is B()
    b0: B
    b1: Optional[B] = field(default=B(b0=2))

    dict0: dict[str, int]
    dict1: dict = field({"one": 1, "two": 2})
    dict2: dict

    any0: typing.Any

    list_any: list


class TestJSONData(TestCase):
    def test__json_data(self):
        # Test basic functionality
        a = A(num5=100, str1="hello")
        self.assertEqual(a.num1, 0)
        self.assertEqual(a.num3, None)
        self.assertEqual(a.num4, 0.5)
        self.assertEqual(a.num5, 100)
        self.assertEqual(a.str1, "hello")
        self.assertEqual(a.str2, None)
        self.assertFalse(a.bool1)
        self.assertEqual(a.enum1, FruitEnum.banana)
        self.assertEqual(a.enum2, FruitEnum.pear)  # First enum value as default
        self.assertIsInstance(a.b0, B)
        self.assertEqual(a.b0.b0, 0)
        self.assertEqual(a.b0.b1, "b1")
        self.assertEqual(a.b0.list0, [])
        self.assertIsInstance(a.b1, B)
        self.assertEqual(a.b1.b0, 2)
        self.assertEqual(a.dict0, {})
        self.assertEqual(a.dict1, {"one": 1, "two": 2})
        self.assertEqual(a.dict2, {})
        self.assertEqual(a.any0, None)
        self.assertEqual(a.list_any, [])

        # Test validation
        with self.assertRaises(pydantic.ValidationError):
            A(num5=100)  # Missing required one_of field

    def test__one_of(self):
        # Test one_of validation
        a = A(num5=100, str1="hello")
        self.assertEqual(a.str1, "hello")
        self.assertEqual(a.str2, None)

        # Test with str2 instead
        a2 = A(num5=100, str2="world")
        self.assertEqual(a2.str1, None)
        self.assertEqual(a2.str2, "world")

        # Test validation error when both are set
        with self.assertRaises(pydantic.ValidationError):
            A(num5=100, str1="hello", str2="world")

    def test__type_errors(self):
        # Test various type validation errors
        with self.assertRaises(TypeError):
            class A:
                a: int

            class Test(JSONData):
                a: A

        with self.assertRaises(TypeError):
            class A1:
                a: int

            class Test1(JSONData, A1):
                x: int

        with self.assertRaises(TypeError):
            class Test2(JSONData):
                x: typing.Union[int, str]

        with self.assertRaises(TypeError):
            class IntEnum(int, Enum):
                a = 1
                b = 2

            class Test3(JSONData):
                a: IntEnum

    def test__serialize_model(self):
        # Test serialization
        a = A(num5=100, str1="hello")
        data = a.model_dump()
        self.assertIn("num5", data)
        self.assertEqual(data["num5"], 100)
        self.assertEqual(data["str1"], "hello")

        # Test JSON serialization
        json_str = json.dumps(data)
        self.assertIn('"num5": 100', json_str) 