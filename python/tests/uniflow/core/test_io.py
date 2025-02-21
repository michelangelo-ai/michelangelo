import uuid
from unittest import TestCase

import numpy as np
import pandas as pd
import pyarrow as pa

from michelangelo.uniflow.core.io_registry import IO
from michelangelo.uniflow.plugins.pandas.io import PandasIO

assert pa  # pyarrow needs to be included for Pandas IO


class Test(TestCase):
    def test_pandas_io_instance(self):
        io = PandasIO()
        assert isinstance(io, IO)

        path = f"memory://~/storage/{uuid.uuid4().hex}"
        data1 = pd.DataFrame(np.random.default_rng().random((5, 3)), columns=list("ABC"))
        metadata = io.write(path, data1)
        data2 = io.read(path, metadata)

        self.assertTrue(data1.equals(data2))
