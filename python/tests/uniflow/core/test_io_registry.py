import unittest
from io import BytesIO

from michelangelo.uniflow.core.io_registry import (
    IO,
    BytesIOIO,
    IORegistry,
    default_io,
    io_registry,
)


class CustomType:
    """Custom type for testing."""

    pass


class CustomIO(IO[CustomType]):
    """Custom IO handler for testing."""

    def write(self, url: str, value: CustomType):
        return None

    def read(self, url: str, metadata):
        return CustomType()


class TestIORegistry(unittest.TestCase):
    def test_set_new_type(self):
        """Test registering a new type."""
        registry = IORegistry({})
        result = registry.set(CustomType, CustomIO())

        # Should return self for chaining
        self.assertIs(result, registry)
        self.assertIn(CustomType, registry)

    def test_set_duplicate_type_raises_error(self):
        """Test that registering a duplicate type raises KeyError."""
        registry = IORegistry({CustomType: CustomIO()})

        with self.assertRaises(KeyError):
            registry.set(CustomType, CustomIO())

    def test_set_duplicate_type_with_force(self):
        """Test that force=True allows overwriting existing type."""
        io1 = CustomIO()
        io2 = CustomIO()
        registry = IORegistry({CustomType: io1})

        registry.set(CustomType, io2, force=True)

        # Should have replaced the IO handler
        self.assertIs(registry[CustomType], io2)

    def test_update_multiple_types(self):
        """Test bulk registration with update()."""
        registry = IORegistry({})
        io1 = CustomIO()
        io2 = BytesIOIO()

        result = registry.update({
            CustomType: io1,
            BytesIO: io2,
        })

        # Should return self for chaining
        self.assertIs(result, registry)
        self.assertIn(CustomType, registry)
        self.assertIn(BytesIO, registry)

    def test_update_with_force(self):
        """Test update() with force=True."""
        io1 = CustomIO()
        io2 = CustomIO()
        registry = IORegistry({CustomType: io1})

        registry.update({CustomType: io2}, force=True)

        self.assertIs(registry[CustomType], io2)

    def test_copy(self):
        """Test copy() creates independent registry."""
        registry = IORegistry({CustomType: CustomIO()})
        copied = registry.copy()

        # Should be different instances
        self.assertIsNot(copied, registry)

        # But should have same contents
        self.assertIn(CustomType, copied)

    def test_getitem_type_not_found(self):
        """Test __getitem__ raises KeyError for unknown type."""
        registry = IORegistry({})

        with self.assertRaises(KeyError) as ctx:
            _ = registry[CustomType]

        self.assertIn("io not found", str(ctx.exception))

    def test_getitem_with_inheritance(self):
        """Test __getitem__ finds IO through inheritance."""
        class BaseType:
            pass

        class DerivedType(BaseType):
            pass

        class BaseIO(IO[BaseType]):
            def write(self, url, value):
                return None

            def read(self, url, metadata):
                return BaseType()

        base_io = BaseIO()
        registry = IORegistry({BaseType: base_io})

        # Should find BaseIO for DerivedType through MRO
        found_io = registry[DerivedType]
        self.assertIs(found_io, base_io)

    def test_setitem(self):
        """Test __setitem__ dictionary syntax."""
        registry = IORegistry({})
        io_handler = CustomIO()

        registry[CustomType] = io_handler

        self.assertIn(CustomType, registry)
        self.assertIs(registry[CustomType], io_handler)

    def test_contains_with_inheritance(self):
        """Test __contains__ checks through MRO."""
        class BaseType:
            pass

        class DerivedType(BaseType):
            pass

        registry = IORegistry({BaseType: CustomIO()})

        self.assertIn(BaseType, registry)
        self.assertIn(DerivedType, registry)  # Through inheritance

    def test_io_registry_deprecated_function(self):
        """Test deprecated io_registry() function returns default_io."""
        result = io_registry()
        self.assertIs(result, default_io)

    def test_bytes_io_write_read(self):
        """Test BytesIOIO write and read operations."""
        io_handler = BytesIOIO()
        test_data = b"Hello, World!"
        buffer = BytesIO(test_data)
        test_url = "memory://test.bin"

        # Write
        metadata = io_handler.write(test_url, buffer)
        self.assertIsNone(metadata)

        # Read
        loaded = io_handler.read(test_url, None)
        self.assertIsInstance(loaded, BytesIO)
        self.assertEqual(loaded.read(), test_data)


if __name__ == "__main__":
    unittest.main()
