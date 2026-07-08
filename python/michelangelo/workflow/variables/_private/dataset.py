"""DatasetVariable — tabular dataset variable for ML workflow tasks.

Mirrors the internal ``DatasetVariable`` design exactly: subclasses ``Variable``,
wraps a storage path and a transient in-memory value, and delegates persistence
to the uniflow IO registry (``PandasIO``, ``SparkIO``, ``RayDatasetIO``).

``_private/`` convention:
    This file lives in ``_private/`` — do not import directly from this path.
    Import ``DatasetVariable`` from ``michelangelo.workflow.variables`` instead.
"""

from __future__ import annotations

from typing import Any

from michelangelo.workflow.variables._private.base import Variable


class DatasetVariable(Variable):
    """A dataset variable flowing between workflow tasks.

    Subclasses ``Variable`` — the same base used by the internal
    ``DatasetVariable``. Wraps a storage path and a transient in-memory value;
    the value is loaded lazily from storage on first access.

    Three backends are supported as first-class citizens:

    - ``pandas.DataFrame`` — single-machine or small datasets.
    - ``pyspark.sql.DataFrame`` — large-scale distributed datasets (Spark).
    - ``ray.data.Dataset`` — Ray-based ML pipelines.

    Persistence is delegated to the IO registry via ``Variable`` helpers:

    - ``PandasIO`` reads/writes Parquet via PyArrow (part-*.parquet directory).
    - ``SparkIO`` reads/writes Parquet via Spark.
    - ``RayDatasetIO`` reads/writes Parquet via Ray.

    Each ``DataSink`` operates on ``variable.value`` in its native format:

    - ``LocalFileSink`` — accepts pandas only; raises ``TypeError`` for Spark/Ray.
    - ``HiveSink`` — accepts Spark only; accesses ``variable.value`` natively.

    Example:
        >>> import pandas as pd
        >>> var = DatasetVariable.create(pd.DataFrame([{"x": 1}]))
        >>> var.backend
        'pandas'
        >>> var.save()
        >>> var2 = DatasetVariable(path=var.path)
        >>> var2.load_pandas_dataframe()
        >>> len(var2.value)
        1
    """

    def __init__(
        self,
        value: Any = None,
        path: str | None = None,
        metadata: Any = None,
        _io_metadata: Any = None,
    ) -> None:
        """Initialise with an optional in-memory value and/or storage path.

        Args:
            value: The in-memory dataset. When provided, ``save()`` persists it
                to ``path``. When ``None``, accessing ``value`` triggers a load.
            path: Storage path (local or fsspec URL). Auto-generated from
                ``UF_STORAGE_URL`` env var (default ``memory://storage``) when
                not provided.
            metadata: Optional metadata forwarded to the IO layer.
            _io_metadata: Internal metadata written by the IO layer after a
                save. Passed through by the UniFlow codec when reconstructing
                a ``DatasetVariable`` from a task result.
        """
        import os
        import uuid

        if path is None:
            path = f"{os.environ.get('UF_STORAGE_URL', 'memory://storage')}/{uuid.uuid4().hex}"
        super().__init__(path=path, metadata=metadata, _io_metadata=_io_metadata)
        self._value = value  # override Variable.__post_init__'s None sentinel

    @classmethod
    def create(cls, value: Any, path: str | None = None) -> DatasetVariable:
        """Create a ``DatasetVariable`` holding an in-memory value.

        Args:
            value: The dataset value — a pandas, Spark, or Ray object.
            path: Optional storage path. Auto-generated when ``None``.

        Returns:
            A new ``DatasetVariable`` with ``value`` ready in memory.

        Example:
            >>> import pandas as pd
            >>> var = DatasetVariable.create(pd.DataFrame([{"x": 1}]))
            >>> var.backend
            'pandas'
        """
        return cls(value=value, path=path)

    def __repr__(self) -> str:
        """Return a concise string showing path and backend."""
        return f"DatasetVariable(path={self.path!r}, backend={self.backend!r})"

    # ------------------------------------------------------------------
    # Backend detection
    # ------------------------------------------------------------------

    @property
    def backend(self) -> str:
        """Return the name of the underlying data backend.

        Returns:
            ``"pandas"``, ``"spark"``, ``"ray"``, or ``"unknown"``.
            Checked in order: pandas → spark → ray. The first match wins.
        """
        try:
            import pandas as pd_rt

            if isinstance(self._value, pd_rt.DataFrame):
                return "pandas"
        except ImportError:
            pass
        try:
            import pyspark.sql as _ps

            if isinstance(self._value, _ps.DataFrame):
                return "spark"
        except ImportError:
            pass
        try:
            import ray.data as _rd

            if isinstance(self._value, _rd.Dataset):
                return "ray"
        except ImportError:
            pass
        return "unknown"

    # ------------------------------------------------------------------
    # Save — delegates to Variable._save_value_using_io
    # ------------------------------------------------------------------

    def save(self) -> None:
        """Persist the in-memory value to ``self.path`` using the right IO.

        Dispatches on the runtime type of ``value``:

        - ``pandas.DataFrame`` → ``PandasIO``
        - ``pyspark.sql.DataFrame`` → ``SparkIO``
        - ``ray.data.Dataset`` → ``RayDatasetIO``

        Raises:
            ValueError: If no value has been set.
            TypeError: If the value type is not supported.
        """
        if self._value is None:
            raise ValueError("Cannot save: no value has been set on this variable.")
        try:
            import pandas as pd_rt

            if isinstance(self._value, pd_rt.DataFrame):
                self.save_pandas_dataframe()
                return
        except ImportError:
            pass
        try:
            import pyspark.sql as _ps

            if isinstance(self._value, _ps.DataFrame):
                self.save_spark_dataframe()
                return
        except ImportError:
            pass
        try:
            import ray.data as _rd

            if isinstance(self._value, _rd.Dataset):
                self.save_ray_dataset()
                return
        except ImportError:
            pass
        raise TypeError(
            f"Cannot save {type(self._value).__name__}. "
            "Supported: pandas.DataFrame, pyspark.sql.DataFrame, ray.data.Dataset."
        )

    def save_pandas_dataframe(self) -> None:
        """Persist the pandas DataFrame to ``self.path`` via ``PandasIO``."""
        from michelangelo.uniflow.plugins.pandas.io import PandasIO

        self._save_value_using_io(PandasIO)

    def save_spark_dataframe(self) -> None:
        """Persist the Spark DataFrame to ``self.path`` via ``SparkIO``."""
        from michelangelo.uniflow.plugins.spark.io import SparkIO

        self._save_value_using_io(SparkIO)

    def save_ray_dataset(self) -> None:
        """Persist the Ray Dataset to ``self.path`` via ``RayDatasetIO``."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        self._save_value_using_io(RayDatasetIO)

    # ------------------------------------------------------------------
    # Load — delegates to Variable._load_value_using_io
    # ------------------------------------------------------------------

    def _load(self) -> None:
        """Load ``_value`` from ``self.path``, detecting the active runtime.

        Runtime detection order (mirrors internal ``DatasetVariable._load()``):

        1. Active Spark session → ``load_spark_dataframe()``
        2. Ray initialized → ``load_ray_dataset()``
        3. Fallback → ``load_pandas_dataframe()``
        """
        try:
            import pyspark.sql as _ps

            spark = _ps.SparkSession.getActiveSession()
            if spark is not None:
                self.load_spark_dataframe()
                return
        except ImportError:
            pass
        try:
            import ray

            if ray.is_initialized():
                self.load_ray_dataset()
                return
        except ImportError:
            pass
        self.load_pandas_dataframe()

    def load_pandas_dataframe(self) -> None:
        """Load the dataset from ``self.path`` as a pandas DataFrame."""
        from michelangelo.uniflow.plugins.pandas.io import PandasIO

        self._load_value_using_io(PandasIO)

    def load_spark_dataframe(self) -> None:
        """Load the dataset from ``self.path`` as a Spark DataFrame."""
        from michelangelo.uniflow.plugins.spark.io import SparkIO

        self._load_value_using_io(SparkIO)

    def load_ray_dataset(self) -> None:
        """Load the dataset from ``self.path`` as a Ray Dataset."""
        from michelangelo.uniflow.plugins.ray.io import RayDatasetIO

        self._load_value_using_io(RayDatasetIO)
