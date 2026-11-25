"""Tests for michelangelo.lib.evaluator.projection."""

from __future__ import annotations

import importlib.util
import sys
import types

import numpy as np
import pandas as pd
import pytest

pytest.importorskip("sklearn")
pytest.importorskip("torch")
pytest.importorskip("matplotlib")

from michelangelo.gen.api.v2.chart_pb2 import ChartType
from michelangelo.lib.evaluator.projection import (
    MAX_SCATTER_POINTS,
    OTHER,
    embedding_chart_columns,
    project,
    projection_charts,
    projection_columns,
    sample_rows,
)
from michelangelo.workflow.schema.evaluator import (
    EmbeddingProjection,
    EvaluatorConfig,
    ProjectionMethod,
)
from michelangelo.workflow.schema.exceptions import ConfigurationError

_HAS_UMAP = importlib.util.find_spec("umap") is not None


class _FakeRayDataset:
    """The slice of ray.data.Dataset that sample_rows uses, one batch per part."""

    def __init__(self, parts: list[pd.DataFrame]):
        """Hold the pandas batches this dataset yields."""
        self.parts = parts

    def select_columns(self, columns):
        """Project every batch to ``columns``."""
        return _FakeRayDataset([part[columns] for part in self.parts])

    def count(self):
        """Total rows across batches."""
        return sum(len(part) for part in self.parts)

    def map_batches(self, fn, batch_format):
        """Apply ``fn`` to a copy of each batch."""
        assert batch_format == "pandas"
        return _FakeRayDataset([fn(part.copy()) for part in self.parts])

    def to_pandas(self):
        """Concatenate the batches."""
        return pd.concat(self.parts, ignore_index=True)


def _blobs(n_per_blob: int = 150, dim: int = 16) -> pd.DataFrame:
    """Two well-separated clusters, labelled by ``blob``."""
    rng = np.random.default_rng(0)
    rows = [
        {"item": f"{blob}{i}", "blob": blob, "emb": center + rng.normal(size=dim)}
        for blob, center in [("a", 5.0), ("b", -5.0)]
        for i in range(n_per_blob)
    ]
    return pd.DataFrame(rows)


def _projection(**overrides) -> EmbeddingProjection:
    """A PCA projection of ``emb`` colored by ``blob``, with ``overrides``."""
    conf = {
        "name": "items",
        "embedding_col": "emb",
        "color_cols": ["blob"],
        "method": "pca",
        "id_col": "item",
        "hover_cols": ["item"],
    }
    return EmbeddingProjection(**{**conf, **overrides})


def test_config_requires_a_color_column():
    """An empty color_cols is rejected."""
    with pytest.raises(ConfigurationError):
        EmbeddingProjection(name="items", embedding_col="emb", color_cols=[])


@pytest.mark.parametrize(
    "override", [{"color_cols": ["emb"]}, {"hover_cols": ["emb"]}, {"id_col": "emb"}]
)
def test_config_rejects_the_embedding_column_in_another_role(override):
    """The embedding column cannot also color, label or identify points."""
    with pytest.raises(ConfigurationError, match="embedding_col 'emb' cannot also be"):
        _projection(**override)


def test_config_defaults_and_method_coercion():
    """Defaults apply, and a method string becomes the enum."""
    projection = EmbeddingProjection(
        name="items", embedding_col="emb", color_cols=["blob"], method="umap"
    )
    assert projection.method is ProjectionMethod.UMAP
    assert projection.max_samples == 20_000


def test_config_rejects_unknown_method_and_bad_sizes():
    """An unknown method and out-of-range sizes are configuration errors."""
    with pytest.raises(ConfigurationError, match="method"):
        _projection(method="tsne")
    with pytest.raises(ConfigurationError, match="max_samples"):
        _projection(max_samples=99)
    with pytest.raises(ConfigurationError, match="max_categories"):
        _projection(max_categories=0)


def test_evaluator_config_holds_projections():
    """EvaluatorConfig carries the projections."""
    config = EvaluatorConfig(embedding_projections=[_projection()])
    assert config.embedding_projections[0].name == "items"


def test_projection_columns():
    """A projection reads its embedding, color, hover and ID columns."""
    columns = projection_columns(_projection(hover_cols=["title"]))
    assert columns == {"emb", "blob", "item", "title"}


def test_embedding_chart_columns():
    """The columns of every projection are unioned; none gives an empty set."""
    assert embedding_chart_columns([]) == set()
    both = [_projection(), _projection(embedding_col="emb2", hover_cols=["title"])]
    assert embedding_chart_columns(both) == {"emb", "emb2", "blob", "item", "title"}


def test_sample_by_id_ignores_row_order_and_partitioning():
    """The same IDs are drawn however the rows are ordered or partitioned."""
    df = _blobs()
    projection = _projection(max_samples=100)
    one_part = sample_rows(_FakeRayDataset([df]), projection)
    shuffled = df.sample(frac=1, random_state=1)
    parts = [shuffled.iloc[:100], shuffled.iloc[100:250], shuffled.iloc[250:]]
    three_parts = sample_rows(_FakeRayDataset(parts), projection)
    assert len(one_part) == 100
    assert set(one_part["item"]) == set(three_parts["item"])
    assert list(one_part.columns) == sorted(projection_columns(projection))


def test_sample_drops_rows_without_embedding():
    """Rows with no embedding never reach the sample."""
    df = _blobs()
    df.loc[: len(df) // 2, "emb"] = None
    sample = sample_rows(_FakeRayDataset([df]), _projection(max_samples=1_000))
    assert len(sample) == len(df) - (len(df) // 2 + 1)
    assert sample["emb"].notna().all()


def test_sample_without_id_col_still_fills():
    """Without an ID column the sample is random but the right size."""
    sample = sample_rows(
        _FakeRayDataset([_blobs()]), _projection(id_col=None, max_samples=100)
    )
    assert len(sample) == 100


def test_pca_separates_blobs_into_clusters():
    """Each blob lands in a single cluster of its own."""
    out = project(_blobs(), _projection())
    assert {"_x", "_y", "_cluster"} <= set(out.columns)
    assert "emb" not in out.columns
    by_blob = out.groupby("blob")["_cluster"].agg(lambda c: c.mode().iloc[0])
    assert by_blob["a"] != by_blob["b"]
    assert (out.groupby("blob")["_cluster"].nunique() == 1).all()


def test_pca_needs_no_umap(monkeypatch):
    """PCA works when umap cannot be imported at all."""
    monkeypatch.setitem(sys.modules, "umap", None)
    out = project(_blobs(), _projection())
    assert len(out) == 300


def test_umap_gets_defaults_and_params(monkeypatch):
    """UMAP gets cosine defaults, the seed, and the user's params on top."""
    seen = {}

    class FakeUMAP:
        """Records its constructor arguments."""

        def __init__(self, **kwargs):
            """Remember the keyword arguments."""
            seen.update(kwargs)

        def fit_transform(self, x):
            """Return the first two coordinates."""
            return x[:, :2]

    monkeypatch.setitem(sys.modules, "umap", types.SimpleNamespace(UMAP=FakeUMAP))
    project(_blobs(), _projection(method="umap", seed=7, params={"n_neighbors": 30}))
    assert seen == {
        "n_neighbors": 30,
        "min_dist": 0.1,
        "metric": "cosine",
        "random_state": 7,
    }


@pytest.mark.skipif(not _HAS_UMAP, reason="umap-learn is not installed")
def test_real_umap_separates_blobs():
    """With umap-learn installed, the real reducer separates the blobs."""
    out = project(_blobs(), _projection(method="umap"))
    by_blob = out.groupby("blob")["_cluster"].agg(lambda c: c.mode().iloc[0])
    assert by_blob["a"] != by_blob["b"]


def test_color_column_keeps_top_categories():
    """Only the most common values survive; nulls and the rest become other."""
    df = _blobs()
    df["blob"] = ["a"] * 200 + ["b"] * 60 + ["c"] * 30 + [None] * 10
    out = project(df, _projection(max_categories=2))
    assert set(out["blob"]) == {"a", "b", OTHER}


def test_charts():
    """Per color column an image and a scatter, then one cluster table."""
    df = _blobs(n_per_blob=1_500, dim=4)
    projection = _projection(color_cols=["blob", "item"], max_categories=3)
    charts = projection_charts(project(df, projection), projection)

    assert [c.chart_type for c in charts] == [
        ChartType.CHART_TYPE_IMAGE,
        ChartType.CHART_TYPE_POINT_CHART,
        ChartType.CHART_TYPE_IMAGE,
        ChartType.CHART_TYPE_POINT_CHART,
        ChartType.CHART_TYPE_TABLE,
    ]
    assert charts[0].image.img_src.startswith("data:image/png;base64,")

    points = [p for s in charts[1].point_chart.point_sets for p in s.points]
    assert len(points) == MAX_SCATTER_POINTS
    assert points[0].point_label == points[0].point_metadata["item"]
    # Item names are unique, so all but max_categories fold into "other", drawn last.
    assert [s.line_name for s in charts[3].point_chart.point_sets][-1] == OTHER

    table = charts[4].table
    assert list(table.column_names) == [
        "cluster",
        "rows",
        "share",
        "top blob",
        "top item",
        "examples (item)",
    ]
    assert sum(row.values[1].number for row in table.rows) == len(df)
