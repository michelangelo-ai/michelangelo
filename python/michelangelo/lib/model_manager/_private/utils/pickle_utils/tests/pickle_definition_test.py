import os
import tempfile
import pickle
import pickletools
from unittest import TestCase
from unittest.mock import patch, MagicMock, create_autospec
from uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils import find_pickle_definitions
from uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package import A, func
from uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.dep import B


class PickleDefinitionTest(TestCase):
    def test_find_pickle_definitions(self):
        with tempfile.TemporaryDirectory() as temp_dir:
            fn = os.path.join(temp_dir, "test.pkl")

            with open(fn, "wb") as f:
                pickle.dump(A(), f)

            defs = find_pickle_definitions(fn)
            self.assertEqual(defs, ["uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod.A"])

            with open(fn, "wb") as f:
                pickle.dump(func, f)

            defs = find_pickle_definitions(fn)
            self.assertEqual(defs, ["uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod.func"])

            with open(fn, "wb") as f:
                pickle.dump({"b": A(), "func": func}, f)

            defs = find_pickle_definitions(fn)
            self.assertEqual(
                set(defs),
                {
                    "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod.A",
                    "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod.func",
                },
            )

            with open(fn, "wb") as f:
                pickle.dump({"B": B(1), "func": func}, f)

            defs = find_pickle_definitions(fn)
            self.assertEqual(
                set(defs),
                {
                    "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.dep.B",
                    "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod.func",
                },
            )

            with open(fn, "wb") as f:
                pickle.dump([A(), B(1), func], f)

            defs = find_pickle_definitions(fn)
            self.assertEqual(
                set(defs),
                {
                    "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod.A",
                    "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.dep.B",
                    "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod.func",
                },
            )

            with open(fn, "wb") as f:
                pickle.dump([A() for _ in range(300)], f)

            defs = find_pickle_definitions(fn)
            self.assertEqual(
                defs,
                [
                    "uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.tests.fixtures.package.mod.A",
                ],
            )

    @patch("uber.ai.michelangelo.sdk.model_manager._private.utils.pickle_utils.pickle_definition.pickletools.genops")
    def test_find_pickle_definitions_proto_3(self, mock_genops):
        op1 = MagicMock()
        op1.name = "GLOBAL"
        op1.stack_before = []
        op1.stack_after = []

        op2 = MagicMock()
        op2.name = "SHORT_BINUNICODE"
        op2.arg = 0
        op2.stack_before = []
        op2.stack_after = [1, pickletools.markobject]

        op3 = MagicMock()
        op3.name = "BINPUT"
        op3.stack_before = []
        op3.stack_after = []

        op4 = MagicMock
        op4.name = "SHORT_BINUNICODE"
        op4.arg = 0

        mock_stack_before = create_autospec(list, instance=True)
        mock_stack_before.__contains__.side_effect = lambda x: x == pickletools.markobject
        mock_stack_before.index.side_effect = ValueError
        op4.stack_before = mock_stack_before
        op4.stack_after = [1]

        mock_genops.return_value = [
            (op1, "foo.bar A", 0),
            (op2, "A", 0),
            (op2, "A", 0),
            (op3, 0, 0),
            (op4, "A", 0),
        ]

        with tempfile.TemporaryDirectory() as temp_dir:
            fn = os.path.join(temp_dir, "test.pkl")

            with open(fn, "wb") as f:
                pickle.dump(1, f)

            defs = find_pickle_definitions(fn)

            self.assertEqual(defs, ["foo.bar.A"])
