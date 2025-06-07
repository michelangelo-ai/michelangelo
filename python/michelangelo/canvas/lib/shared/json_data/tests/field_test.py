import pydantic
from unittest import TestCase

from michelangelo.canvas.lib.shared.json_data.field import field, one_of


class TestField(TestCase):
    def test__field(self):
        f = field()
        self.assertTrue(isinstance(f, pydantic.fields.FieldInfo))
        self.assertTrue(len(f.json_schema_extra["json_data_field"]) == 0)

        f = field(...)
        self.assertTrue(f.json_schema_extra["json_data_field"]["required"])

        f = field(0, gt=0, lt=1)
        self.assertFalse("required" in f.json_schema_extra["json_data_field"])
        str_repr = format(f)
        self.assertTrue("gt=0" in str_repr)
        self.assertTrue("lt=1" in str_repr)

        f = field(0, ge=0.1, le=1.2)
        str_repr = format(f)
        self.assertTrue("ge=0.1" in str_repr)
        self.assertTrue("le=1.2" in str_repr)

        f = field(0, min_length=1, max_length=10)
        str_repr = format(f)
        self.assertTrue("min_length=1" in str_repr)
        self.assertTrue("max_length=10" in str_repr)

        f = field("abc", pattern=r"\w+")
        self.assertEqual(f.default, "abc")
        str_repr = format(f)
        self.assertTrue("pattern='\\\\w+'" in str_repr)

    def test__one_of(self):
        f = one_of(fields=["f1", "f2"], required=True)
        self.assertTrue(f.required)
        self.assertListEqual(f.fields, ["f1", "f2"])
        f = one_of(fields=["f3", "f4", "f5"], required=False)
        self.assertFalse(f.required)
        self.assertListEqual(f.fields, ["f3", "f4", "f5"]) 