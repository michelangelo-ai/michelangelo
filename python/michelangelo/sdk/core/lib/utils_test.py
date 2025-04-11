import unittest

from michelangelo.sdk.core.lib.utils import get_class


class DummyClass:
    def __init__(self):
        self.a = 1


class TestUtils(unittest.TestCase):
    def test_get_class(self):
        klass = get_class("michelangelo.sdk.core.lib.utils_test.DummyClass")
        assert klass().__class__.__name__ == "DummyClass"

        klass = get_class(DummyClass)
        assert klass().__class__.__name__ == "DummyClass"
