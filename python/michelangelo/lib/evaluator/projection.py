"""2-D projections of embedding columns for the evaluation report.

See :class:`~michelangelo.workflow.schema.evaluator.EmbeddingProjection` for the
config. A projection samples rows of the dataset, reduces their embeddings to 2-D
with UMAP or PCA, clusters them with HDBSCAN (in the layout, or in a reduction of
``cluster_dims`` dimensions), and returns report charts: per color column a static
image of every sampled row and an interactive scatter of up to
``MAX_SCATTER_POINTS`` of them, plus one table of the clusters.

``umap`` and ``matplotlib`` are imported only when a projection runs, so the rest of
the evaluator does not need them. PCA needs neither ``umap`` nor anything beyond
scikit-learn.
"""

from __future__ import annotations

from typing import TYPE_CHECKING, Any

import numpy as np

from michelangelo.gen.api.v2.chart_pb2 import (
    Chart,
    ChartDataPoint,
    ChartDataset,
    ChartType,
    PointChart,
)
from michelangelo.lib.evaluator.embedding_metrics import (
    _hash_ids,
    _id_keys,
    check_embedding_column,
)
from michelangelo.lib.evaluator.report import (
    create_chart_with_image,
    create_metrics_table,
    matplotlib_fig_to_html,
)
from michelangelo.workflow.schema.evaluator import (
    EmbeddingProjection,
    ProjectionMethod,
)

if TYPE_CHECKING:
    import pandas as pd
    import ray

__all__ = [
    "MAX_SCATTER_POINTS",
    "embedding_chart_columns",
    "project",
    "projection_charts",
    "projection_columns",
    "sample_rows",
]

# The scatter carries every point (and its hover text) inside the report proto,
# which the API server stores whole, so it shows a sample of the sample. The
# static image draws all of them.
MAX_SCATTER_POINTS = 2_000
EXAMPLES_PER_CLUSTER = 5
OTHER = "other"
NULL = "null"
UNCLUSTERED = -1
# Draw this many times max_samples by hash before trimming, so the sample still
# fills after rows without an embedding are dropped.
_OVERSAMPLE = 2


def projection_columns(projection: EmbeddingProjection) -> set[str]:
    """Columns a projection reads from the dataset.

    Args:
        projection: The projection config.

    Returns:
        The embedding, color, hover, ``where`` and ID columns.
    """
    columns = {
        projection.embedding_col,
        *projection.color_cols,
        *projection.hover_cols,
        *projection.where,
    }
    if projection.id_col:
        columns.add(projection.id_col)
    return columns


def embedding_chart_columns(projections: list[EmbeddingProjection]) -> set[str]:
    """Columns the embedding projections read.

    Args:
        projections: The configured projections.

    Returns:
        The union of every projection's columns; empty when none is configured.
    """
    return set().union(*(projection_columns(projection) for projection in projections))


def sample_rows(
    ray_dataset: ray.data.Dataset, projection: EmbeddingProjection
) -> pd.DataFrame:
    """Draw up to ``max_samples`` rows with an embedding, among those ``where`` selects.

    The rows come back in random order.

    Rows are kept by a key in [0, 1): hashed from ``id_col`` and the seed when set,
    so the same IDs are drawn however the rows are ordered or partitioned, else
    random.

    Args:
        ray_dataset: The dataset to sample.
        projection: The projection config.

    Returns:
        The sampled rows, restricted to :func:`projection_columns`.
    """
    dataset = ray_dataset.select_columns(sorted(projection_columns(projection)))
    if projection.where:
        # Filter before counting, so the sampling threshold is a share of the
        # selected rows.
        dataset = dataset.map_batches(
            lambda batch: batch[_matches(batch, projection.where)],
            batch_format="pandas",
        )
    threshold = min(1.0, _OVERSAMPLE * projection.max_samples / max(dataset.count(), 1))

    def keep(batch: pd.DataFrame) -> pd.DataFrame:
        hashes = _hash_ids(batch[projection.id_col]) if projection.id_col else None
        if hashes is not None:
            keys = _id_keys(hashes, projection.seed).numpy()
        else:
            keys = np.random.default_rng().random(len(batch))
        batch = batch.assign(_key=keys)
        return batch[batch["_key"] < threshold]

    df = dataset.map_batches(keep, batch_format="pandas").to_pandas()
    df = df[df[projection.embedding_col].notna()]
    return (
        df.nsmallest(projection.max_samples, "_key")
        .drop(columns="_key")
        .reset_index(drop=True)
    )


def _matches(batch: pd.DataFrame, where: dict[str, str]) -> np.ndarray:
    """Mask of the rows whose columns all equal their ``where`` value as strings."""
    mask = np.ones(len(batch), dtype=bool)
    for col, value in where.items():
        mask &= (batch[col].astype(str) == value).to_numpy()
    return mask


def _reducer(projection: EmbeddingProjection, cluster_dims: int | None = None) -> Any:
    """Build the 2-D layout's reducer, or with ``cluster_dims`` the clustering one.

    Args:
        projection: The projection config.
        cluster_dims: When set, build the reducer HDBSCAN clusters in, with this
            many output dimensions, instead of the 2-D layout's.

    Returns:
        An object with ``fit_transform``.
    """
    if projection.method == ProjectionMethod.UMAP:
        import umap  # slow to import; only projections need it

        kwargs = {
            "n_neighbors": 15,
            "min_dist": 0.1,
            "metric": "cosine",
            "random_state": projection.seed,
            **projection.params,
        }
        if cluster_dims:
            # min_dist only spreads points out for the eye; packed tight they
            # cluster better.
            kwargs.update(n_components=cluster_dims, min_dist=0.0)
        return umap.UMAP(**kwargs)

    from sklearn.decomposition import PCA

    kwargs = {"n_components": 2, "random_state": projection.seed, **projection.params}
    if cluster_dims:
        kwargs["n_components"] = cluster_dims
    return PCA(**kwargs)


def project(df: pd.DataFrame, projection: EmbeddingProjection) -> pd.DataFrame:
    """Reduce a sample to 2-D, cluster it, and fold the color columns.

    Args:
        df: Rows from :func:`sample_rows`.
        projection: The projection config.

    Returns:
        ``df`` with the embedding column replaced by coordinates ``_x`` and ``_y``
        and a ``_cluster`` label, and each color column folded to its most common
        values.
    """
    from sklearn.cluster import HDBSCAN

    check_embedding_column(df[projection.embedding_col])
    x = np.vstack(df[projection.embedding_col].to_numpy())
    xy = _reducer(projection).fit_transform(x)
    cluster_dims = min(projection.cluster_dims, *x.shape)
    cluster_space = (
        xy if cluster_dims <= 2 else _reducer(projection, cluster_dims).fit_transform(x)
    )

    out = df.drop(columns=projection.embedding_col).assign(_x=xy[:, 0], _y=xy[:, 1])
    out["_cluster"] = HDBSCAN(min_cluster_size=max(20, len(out) // 100)).fit_predict(
        cluster_space
    )
    for col in projection.color_cols:
        out[col] = _top_categories(out[col], projection.max_categories)
    return out


def projection_charts(df: pd.DataFrame, projection: EmbeddingProjection) -> list[Chart]:
    """Build the report charts for a projected sample.

    Args:
        df: The output of :func:`project`.
        projection: The projection config.

    Returns:
        An image and a scatter per color column, then one cluster table.
    """
    charts = []
    for col in projection.color_cols:
        charts.append(_image_chart(df, projection, col))
        charts.append(_scatter_chart(df, projection, col))
    charts.append(_cluster_table(df, projection))
    return charts


def _top_categories(values: pd.Series, k: int) -> pd.Series:
    """Keep the ``k`` most common values as strings; nulls and the rest fold."""
    values = values.astype(object).where(values.notna(), NULL).astype(str)
    top = values.value_counts().index[:k]
    return values.where(values.isin(top), OTHER)


def _by_category(df: pd.DataFrame, col: str) -> list[tuple[str, pd.DataFrame]]:
    """Group by color value, most common first and "other" last."""
    order = df[col].value_counts().index.tolist()
    order = [c for c in order if c != OTHER] + ([OTHER] if OTHER in order else [])
    return [(category, df[df[col] == category]) for category in order]


def _title(projection: EmbeddingProjection, col: str) -> str:
    """Title shared by a color column's image and scatter."""
    return f"{projection.name} ({projection.method.value}) by {col}"


def _image_chart(df: pd.DataFrame, projection: EmbeddingProjection, col: str) -> Chart:
    """A static image of every sampled row, colored by ``col``."""
    from matplotlib.figure import Figure

    fig = Figure(figsize=(8, 7), dpi=110)
    ax = fig.subplots()
    for i, (category, group) in enumerate(_by_category(df, col)):
        color = "#c8c8c8" if category == OTHER else f"C{i % 10}"
        ax.scatter(
            group["_x"],
            group["_y"],
            s=2,
            alpha=0.6,
            linewidths=0,
            color=color,
            label=f"{category} ({len(group):,})",
        )
    ax.set_xticks([])
    ax.set_yticks([])
    ax.legend(markerscale=4, fontsize=8, frameon=False)
    return create_chart_with_image(
        matplotlib_fig_to_html(fig, fmt="png"),
        description=f"{len(df):,} sampled rows of {projection.embedding_col}",
        title=_title(projection, col),
    )


def _scatter_chart(
    df: pd.DataFrame, projection: EmbeddingProjection, col: str
) -> Chart:
    """An interactive scatter of the first ``MAX_SCATTER_POINTS`` rows."""
    # Rows arrive in random order, so the head is itself a random sample.
    shown = df.head(MAX_SCATTER_POINTS)
    scatter = PointChart(x_axis="dim 1", y_axis="dim 2")
    for category, group in _by_category(shown, col):
        points = ChartDataset(line_name=category)
        for row in group.to_dict(orient="records"):
            point = ChartDataPoint(x=float(row["_x"]), y=float(row["_y"]))
            if projection.id_col:
                point.point_label = str(row[projection.id_col])
            for hover_col in projection.hover_cols:
                point.point_metadata[hover_col] = str(row[hover_col])
            points.points.append(point)
        scatter.point_sets.append(points)

    chart = Chart()
    chart.chart_type = ChartType.CHART_TYPE_POINT_CHART
    chart.title = f"{_title(projection, col)}, {len(shown):,} points"
    chart.section_id = f"{_title(projection, col)} scatter"
    chart.point_chart.CopyFrom(scatter)
    return chart


def _cluster_table(df: pd.DataFrame, projection: EmbeddingProjection) -> Chart:
    """A table of the clusters: size, top color values and examples."""
    example_col = (
        projection.hover_cols[0] if projection.hover_cols else projection.id_col
    )
    columns = [
        "cluster",
        "rows",
        "share",
        *(f"top {col}" for col in projection.color_cols),
    ]
    if example_col:
        columns.append(f"examples ({example_col})")

    sizes = df["_cluster"].value_counts()
    order = [c for c in sizes.index if c != UNCLUSTERED] + (
        [UNCLUSTERED] if UNCLUSTERED in sizes.index else []
    )
    rows = []
    for cluster in order:
        group = df[df["_cluster"] == cluster]
        row = [
            ("unclustered" if cluster == UNCLUSTERED else str(cluster), "str_value"),
            (float(len(group)), "number"),
            (f"{len(group) / len(df):.1%}", "str_value"),
        ]
        for col in projection.color_cols:
            share = group[col].value_counts(normalize=True)
            row.append((f"{share.index[0]} ({share.iloc[0]:.0%})", "str_value"))
        if example_col:
            examples = group[example_col].astype(str).head(EXAMPLES_PER_CLUSTER)
            row.append((" · ".join(examples), "str_value"))
        rows.append(row)

    title = f"{projection.name} ({projection.method.value}) clusters"
    return create_metrics_table(
        column_names=columns, values=rows, title=title, section_id=title
    )
