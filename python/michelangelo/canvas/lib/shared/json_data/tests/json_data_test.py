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
    c0: int = field(default=0)
    c1: str = field(default="")
    c2: float = field(default=0.0)
    _c3: int  # hidden field will not show up in json schema


class B(C):
    b0: int = field(default=0)
    b1: str = field(default="b1")
    list0: list[int] = field(default=[])


class A(JSONData):
    # Default value of an int field is 0 is not otherwise specified
    num1: int = field(default=0)
    # Optional field is annotated as '<type> | None'.
    # If not specified, the default value of optional field is None.
    num3: Optional[int] = field(default=None)
    # set default value and validation rules using field
    # a float number in [0.0, 1.0)
    num4: float = field(default=0.5, ge=0.0, lt=1.0)
    # set default value to ... requires users explicitly set this field
    num5: int = field(default=...)

    # one and only one filed in the list must be set (not None)
    _one_of_str12 = one_of(fields=["str1", "str2"], required=True)
    # optional string field <= 30 characters
    str1: Optional[str] = field(default=None, max_length=30)
    # optional string field validated with a regular expression
    str2: Optional[str] = field(default=None, pattern=r"\w+")

    _one_of_n132 = one_of(fields=["f1", "f2", "f3"], required=False)
    f1: Optional[int] = field(default=None)
    f2: Optional[float] = field(default=None)
    f3: Optional[int] = field(default=None)

    # default value of bool field is false
    bool1: bool = field(default=False)

    # Only support string enum. If not specified, the default is the 1st member of the enum.
    enum1: FruitEnum = field(default=FruitEnum.banana)
    enum2: FruitEnum = field(default=FruitEnum.pear)

    # another JSONData class. The default value is B()
    b0: B = field(default=B(c0=0, c1="", c2=0.0, b0=0, list0=[]))
    b1: Optional[B] = field(default=B(c0=1, c1="test", c2=1.0, b0=2, list0=[]))

    dict0: dict[str, int] = field(default={})
    dict1: dict = field({"one": 1, "two": 2})
    dict2: dict = field(default={})

    any0: typing.Any = field(default=None)

    list_any: list = field(default=[])


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
        self.assertEqual(a.b0.c0, 0)  # From C parent class
        self.assertEqual(a.b0.c1, "")  # From C parent class
        self.assertEqual(a.b0.c2, 0.0)  # From C parent class
        self.assertEqual(a.b0.b0, 0)
        self.assertEqual(a.b0.b1, "b1")
        self.assertEqual(a.b0.list0, [])
        self.assertIsInstance(a.b1, B)
        self.assertEqual(a.b1.b0, 2)
        self.assertEqual(a.b1.c0, 1)
        self.assertEqual(a.b1.c1, "test")
        self.assertEqual(a.b1.c2, 1.0)
        self.assertEqual(a.b1.list0, [])
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
