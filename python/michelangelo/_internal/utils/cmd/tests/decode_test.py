from unittest import TestCase
from michelangelo._internal.utils.cmd import decode_output


class DecodeTest(TestCase):
    def test_decode_output(self):
        self.assertEqual(decode_output(b"output"), "output")
        self.assertEqual(decode_output("output"), "output")
