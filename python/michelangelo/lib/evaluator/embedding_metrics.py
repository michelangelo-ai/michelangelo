"""Intrinsic embedding-quality metrics as TorchMetrics.

Label-free checks on the geometry of an embedding column -- is the space
collapsed, how many dimensions carry variance, does it cluster -- and on how
many rows have a usable embedding at all. Each metric returns a single scalar.

They are tagged ``_metric_type = "embedding"``, so the evaluator hands
``update(preds)`` the prediction column as-is, one vector per row, without a
target. Before any of them runs, the evaluator fails the scope if the non-null
rows differ in length or hold a NaN, Inf or missing element: no threshold makes
such a column usable, and every geometry metric would be invalid on it. Rows
with no embedding at all are skipped by the geometry metrics and counted by
``EmbeddingNullRate``.

``columns.target_col`` is still required by the config schema. The metric never
reads it, but the evaluator still loads it from the dataset (without a
``data_preprocessor`` it is part of the parquet column projection), so point it
at a cheap, narrow column such as a small ID or label column, not a wide one
like another embedding. The optional ``columns.index_col`` names the row's
entity ID, which the sampled metrics below draw their sample by:

    - name: user_emb_cosine_p95
      metric: michelangelo.lib.evaluator.embedding_metrics.EmbeddingCosineSimilarity
      params: {stat: p95}
      columns:
        prediction_col: user_embedding
        target_col: user_id   # required by schema; loaded but not read
        index_col: user_id    # optional; the sample is drawn by user_id

``EmbeddingCosineSimilarity`` and ``EmbeddingNorm`` take a ``stat``: ``mean``,
``std``, or a percentile such as ``p5`` / ``p50`` / ``p95``. Configure one metric
per percentile to read the shape of the distribution, not just its center.

Where the math allows, a metric is exact over every non-null row, streamed in
chunks so memory stays bounded: ``EmbeddingNorm``, ``EmbeddingDeadDimensions``,
``EmbeddingEffectiveRank`` (from the D x D covariance) and the mean and std of
``EmbeddingCosineSimilarity`` (from sums over the normalized rows, without
forming any pair). The others compare rows with each other, so they work on a
seeded uniform sample of ``max_samples`` rows: cosine percentiles and
``EmbeddingUniformity`` score ``num_pairs`` random pairs of it (every pair, when
there are fewer), ``EmbeddingHubness`` searches neighbors within it and
``EmbeddingSilhouette`` clusters it.

With an ``index_col``, a row's place in the sample follows from a hash of its ID
and the ``seed`` alone, so the same entities are sampled however the rows are
ordered, partitioned or spread over workers, and two runs or two models over the
same entities score the same ones. That needs one row per ID: the metric fails
when more than 1% of the sampled rows share an ID. Without IDs, or for rows whose
ID is missing, the sample is drawn at random, which is reproducible only for the
same rows in the same order.
"""

import logging
import math
import re
from collections.abc import Iterator
from typing import Any, Optional, Union

import numpy as np
import pandas as pd
import torch
from torchmetrics import Metric

__all__ = [
    "EmbeddingCosineSimilarity",
    "EmbeddingDeadDimensions",
    "EmbeddingEffectiveRank",
    "EmbeddingHubness",
    "EmbeddingNorm",
    "EmbeddingNullRate",
    "EmbeddingSilhouette",
    "EmbeddingUniformity",
    "EmbeddingZeroRate",
]

_logger = logging.getLogger(__name__)

_STATS = {"mean": torch.mean, "std": torch.std}
# "p5", "p50", "p99.9": a percentile in [0, 100].
_PERCENTILE_STAT = re.compile(r"p(\d+(?:\.\d+)?)")

_DEFAULT_MAX_SAMPLES = 50_000
_DEFAULT_NUM_PAIRS = 10_000_000
# Share of sampled rows that may share an ID with another before ID sampling fails.
_MAX_DUPLICATE_ID_SHARE = 0.01

# Elements stacked or multiplied at a time (~128 MB at float64), bounding each copy
# to a slice of the column however wide the embedding -- the GLOBAL scope evaluates
# every row at once, on a node already holding the whole dataset.
_STACK_CHUNK_ELEMENTS = 16_000_000

Rows = Union[torch.Tensor, np.ndarray]


def _percentile_of(stat: Any) -> Optional[float]:
    match = _PERCENTILE_STAT.fullmatch(stat) if isinstance(stat, str) else None
    if match is None or float(match.group(1)) > 100:
        return None
    return float(match.group(1))


def _check_stat(stat: str) -> str:
    if stat not in _STATS and _percentile_of(stat) is None:
        raise ValueError(
            "stat must be 'mean', 'std' or a percentile such as 'p5' or 'p95', "
            f"got {stat!r}"
        )
    return stat


def _reduce(stat: str, values: torch.Tensor) -> torch.Tensor:
    if stat in _STATS:
        return _STATS[stat](values.double())
    # sort() moves NaNs to the end, which would hide them from low percentiles;
    # propagate them like mean/std and numpy do.
    if values.numel() == 0 or values.isnan().any():
        return torch.tensor(float("nan"))
    # Sort-based instead of torch.quantile, which rejects inputs over 2**24 elements.
    # Interpolates linearly, like numpy.
    ordered = values.flatten().sort().values
    pos = _percentile_of(stat) / 100 * (ordered.numel() - 1)
    lo = int(pos)
    hi = min(lo + 1, ordered.numel() - 1)
    if lo == hi or ordered[lo] == ordered[hi]:
        return ordered[lo]
    return ordered[lo] + (ordered[hi] - ordered[lo]) * (pos - lo)


def _rank() -> int:
    """This process's rank, so each one draws its own keys; 0 outside a group."""
    if torch.distributed.is_available() and torch.distributed.is_initialized():
        return torch.distributed.get_rank()
    return 0


def _hash_ids(ids: Any) -> Optional[np.ndarray]:
    """A 64-bit hash of each row's ID as a string, or None when one is missing.

    Hashing the string form makes 5 and "5" agree. hash_array hashes strings
    with SipHash under a fixed key: the same on every run and host.
    """
    column = np.asarray(_as_column(ids), dtype=object)
    if pd.isna(column).any():
        return None
    return pd.util.hash_array(column.astype(str).astype(object))


def _id_keys(id_hashes: np.ndarray, seed: int) -> torch.Tensor:
    """Sample keys in [0, 1) from each ID hash mixed with the seed.

    Uses splitmix64's finalizer.
    """
    z = id_hashes.astype(np.uint64) ^ np.uint64(
        (seed * 0x9E3779B97F4A7C15) & 0xFFFFFFFFFFFFFFFF
    )
    # uint64 array arithmetic wraps around, as the mix intends.
    z = (z ^ (z >> np.uint64(30))) * np.uint64(0xBF58476D1CE4E5B9)
    z = (z ^ (z >> np.uint64(27))) * np.uint64(0x94D049BB133111EB)
    z ^= z >> np.uint64(31)
    return torch.from_numpy((z >> np.uint64(11)).astype(np.float64) * 2.0**-53)


def _cat(state: Union[list[torch.Tensor], torch.Tensor]) -> torch.Tensor:
    """A list state as one tensor; a distributed sync has already concatenated it."""
    if isinstance(state, torch.Tensor):
        return state
    return torch.cat(state) if state else torch.empty(0)


def _as_column(preds: Any) -> Any:
    """One object per row; np.asarray would turn equal-length rows into a matrix."""
    if not isinstance(preds, list):
        return preds
    column = np.empty(len(preds), dtype=object)
    for i, row in enumerate(preds):
        column[i] = row
    return column


def _null_mask(preds: Any) -> np.ndarray:
    """Rows with no embedding: None, NaN of any float width, ``pd.NA`` or ``pd.NaT``.

    Each row is one value to ``pd.isna``, so a NaN inside a vector is not a
    missing embedding.
    """
    if isinstance(preds, torch.Tensor):  # a stacked tensor cannot hold a missing row
        return np.zeros(len(preds), dtype=bool)
    return np.asarray(pd.isna(_as_column(preds)), dtype=bool)


def _non_null_rows(preds: Any) -> Rows:
    if isinstance(preds, torch.Tensor):
        if preds.numel() and preds.dim() != 2:
            raise ValueError(
                f"expected [N, D] embeddings, got shape {tuple(preds.shape)}"
            )
        return preds
    column = _as_column(preds)
    return column[~_null_mask(column)]


def _width(rows: Rows) -> int:
    return rows.shape[1] if isinstance(rows, torch.Tensor) else int(np.size(rows[0]))


def _row_chunks(rows: Rows) -> Iterator[Rows]:
    """Consecutive slices of non-empty ``rows``.

    Each slice holds about ``_STACK_CHUNK_ELEMENTS`` elements.
    """
    step = max(1, _STACK_CHUNK_ELEMENTS // max(1, _width(rows)))
    for start in range(0, len(rows), step):
        yield rows[start : start + step]


def _to_matrix(rows: Rows) -> torch.Tensor:
    """Rows as one float32 ``[n, D]`` tensor."""
    if isinstance(rows, torch.Tensor):
        return rows.float()
    try:
        # vstack turns each row -- vector or scalar -- into one row of the matrix.
        matrix = np.vstack(rows)
    except ValueError as err:
        raise ValueError("embeddings must all have the same dimension") from err
    return torch.from_numpy(matrix.astype(np.float32, copy=False))


def _is_finite_row(row: Any) -> bool:
    try:
        values = np.asarray(row, dtype=float)
    except (TypeError, ValueError):  # pd.NA, strings
        return False
    return bool(np.isfinite(values).all())


def _dimension_message(rows: np.ndarray) -> str:
    lengths = np.fromiter(
        (np.size(row) for row in rows), dtype=np.int64, count=len(rows)
    )
    dims, counts = np.unique(lengths, return_counts=True)
    order = np.argsort(-counts, kind="stable")
    others = ", ".join(f"{counts[i]:,} rows have dim {dims[i]}" for i in order[1:4])
    if len(order) > 4:
        others += f" and {len(order) - 4} more dims"
    expected = order[0]
    share = counts[expected] / len(rows)
    return (
        "rows must all have the same dimension: expected "
        f"{dims[expected]} ({share:.1%} of rows), but {others}"
    )


def check_embedding_column(preds: Any) -> int:
    """Fail on a column no embedding metric can be valid on.

    Returns the column's count of null rows.

    Raises ValueError when the non-null rows differ in length, or any of them holds
    a NaN, Inf or missing element. Streams the rows in chunks.
    """
    column = preds if isinstance(preds, torch.Tensor) else _as_column(preds)
    null = _null_mask(column)
    rows = _non_null_rows(column)
    if len(rows) == 0:
        return int(null.sum())
    width, non_finite = None, 0
    for chunk in _row_chunks(rows):
        if isinstance(chunk, torch.Tensor):
            matrix = chunk.numpy()
        else:
            try:
                matrix = np.vstack(chunk)
            except ValueError:
                raise ValueError(_dimension_message(rows)) from None
        if width is not None and matrix.shape[1] != width:
            raise ValueError(_dimension_message(rows))
        width = matrix.shape[1]
        if width == 0:
            raise ValueError("embeddings have dimension 0")
        if matrix.dtype.kind in "biuf":
            non_finite += int((~np.isfinite(matrix).all(axis=1)).sum())
        else:  # missing elements (None, pd.NA) or non-numbers made it an object array
            non_finite += sum(not _is_finite_row(row) for row in chunk)
    if non_finite:
        raise ValueError(
            f"{non_finite:,} of {len(rows):,} rows contain NaN, Inf or missing elements"
        )
    return int(null.sum())


def _moments(
    x: torch.Tensor, covariance: bool
) -> tuple[torch.Tensor, torch.Tensor, torch.Tensor]:
    """Row count, column means and centered second moments of a float32 chunk.

    The second moments are the sums of squared deviations, or the full D x D
    matrix of their products; all three are float64.
    """
    mean = x.double().mean(dim=0)
    if covariance:
        # A float32 GEMM: centered first, so its rounding scales with the spread of
        # the rows rather than with their offset from the origin.
        centered = x - mean.float()
        m2 = (centered.T @ centered).double()
    else:
        m2 = (x.double() - mean).pow(2).sum(dim=0)
    return torch.tensor(float(len(x)), dtype=torch.float64), mean, m2


def _merge_moments(
    a: tuple[torch.Tensor, torch.Tensor, torch.Tensor],
    b: tuple[torch.Tensor, torch.Tensor, torch.Tensor],
) -> tuple[torch.Tensor, torch.Tensor, torch.Tensor]:
    """The moments of two row sets combined, stable in any order.

    Uses Chan et al.'s pairwise update.
    """
    n_a, mean_a, m2_a = a
    n_b, mean_b, m2_b = b
    n = n_a + n_b
    delta = mean_b - mean_a
    spread = torch.outer(delta, delta) if m2_a.dim() == 2 else delta**2
    return n, mean_a + delta * (n_b / n), m2_a + m2_b + spread * (n_a * n_b / n)


def _pair_cosines(x: torch.Tensor, num_pairs: int, seed: int) -> torch.Tensor:
    """Cosine similarity of every distinct row pair.

    Or of ``num_pairs`` random distinct pairs, when there are more.
    """
    x = torch.nn.functional.normalize(x, dim=1)
    n = x.shape[0]
    if n * (n - 1) // 2 <= num_pairs:
        cols = torch.arange(n, device=x.device)
        step = max(1, _STACK_CHUNK_ELEMENTS // max(1, n))
        blocks = []
        for start in range(0, n, step):
            rows = cols[start : start + step]
            blocks.append((x[rows] @ x.T)[cols > rows[:, None]])
        return torch.cat(blocks)
    generator = torch.Generator().manual_seed(seed)
    i = torch.randint(n, (num_pairs,), generator=generator)
    j = torch.randint(n - 1, (num_pairs,), generator=generator)
    j += j >= i  # skip row i itself, keeping j uniform over the other rows
    _logger.info(
        "Embedding pair metric: scoring %d random pairs of %d sampled rows",
        num_pairs,
        n,
    )
    out = torch.empty(num_pairs, device=x.device)
    step = max(1, _STACK_CHUNK_ELEMENTS // max(1, x.shape[1]))
    for start in range(0, num_pairs, step):
        end = start + step
        out[start:end] = (x[i[start:end]] * x[j[start:end]]).sum(dim=1)
    return out


def _check_num_pairs(num_pairs: int) -> int:
    if num_pairs < 1:
        raise ValueError(f"num_pairs must be >= 1, got {num_pairs}")
    return num_pairs


class _EmbeddingMetric(Metric):
    """Base for the geometry metrics: reads the column as-is and skips null rows.

    A subclass either streams every row through ``_consume`` and reports from
    its accumulated state in ``_finish`` (exact metrics), or calls
    ``_keep_sample`` and scores a seeded uniform sample in ``_score``.

    The sample is bottom-k: every row gets a key uniform in [0, 1) -- hashed from
    its ID and the seed, or drawn at random -- and the ``max_samples`` rows with
    the smallest keys are kept. That is a uniform sample of every row seen,
    however the rows were split into updates, and two kept sets merge by keeping
    the smallest keys again -- which is what lets the states cat-reduce across
    processes. Only the kept rows are ever stacked. All states are 1-D lists,
    which a process that saw no rows can still sync.
    """

    higher_is_better = None
    # forward() then updates the global state directly instead of appending each
    # batch's partial state to it, so the state stays bounded there too.
    full_state_update = True
    _metric_type = "embedding"

    def __init__(self, **kwargs: Any) -> None:
        super().__init__(**kwargs)
        self.max_samples: Optional[int] = None
        self._dim: Optional[int] = None

    def reset(self) -> None:
        super().reset()
        self._dim = None

    def update(self, preds: Any, ids: Any = None) -> None:
        """Add a batch of rows; ``ids``, one entity ID per row, place them."""
        rows = _non_null_rows(preds)
        if len(rows) == 0:
            return
        if self.max_samples is not None:
            id_hashes = None
            if ids is not None:
                if len(ids) != len(preds):
                    raise ValueError(f"got {len(ids)} ids for {len(preds)} rows")
                id_hashes = _hash_ids(ids)
                if id_hashes is None:
                    _logger.warning(
                        "%s: some rows have no ID; sampling at random instead",
                        type(self).__name__,
                    )
                else:
                    id_hashes = id_hashes[~_null_mask(preds)]
            self._sample(rows, id_hashes)
            return
        for chunk in _row_chunks(rows):
            x = _to_matrix(chunk)
            self._check_dim(x.shape[1])
            self._consume(x)

    def compute(self) -> torch.Tensor:
        if self.max_samples is None:
            return self._finish()
        # A distributed sync hands the list states over already concatenated.
        keys = _cat(self.keys)
        if keys.numel() == 0:
            return torch.tensor(float("nan"))
        kept, idx = torch.topk(
            keys, min(self.max_samples, keys.numel()), largest=False, sorted=True
        )
        # Equal keys come from equal IDs: random 53-bit keys practically never collide.
        shared = 1 - kept.unique().numel() / kept.numel()
        if shared > _MAX_DUPLICATE_ID_SHARE:
            raise ValueError(
                f"{shared:.1%} of the sampled rows share an ID with another; "
                "sampling by ID needs one row per ID -- point index_col at a "
                "unique ID column, or leave it unset to sample at random"
            )
        sample = _cat(self.samples).view(keys.numel(), -1)[idx]
        _logger.info(
            "%s: scoring a sample of %d of %d rows",
            type(self).__name__,
            len(sample),
            int(self.rows_seen),
        )
        return self._score(sample)

    def _check_dim(self, dim: int) -> None:
        if dim == 0:
            raise ValueError("embeddings have dimension 0")
        if self._dim is None:
            self._dim = dim
        elif dim != self._dim:
            raise ValueError(
                "embeddings must all have the same dimension, "
                f"got {self._dim} and {dim}"
            )

    # Exact metrics.

    def _consume(self, x: torch.Tensor) -> None:
        raise NotImplementedError

    def _finish(self) -> torch.Tensor:
        raise NotImplementedError

    def _keep_moments(self, covariance: bool) -> None:
        """Track the row count, column means and centered second moments."""
        self._covariance = covariance
        # One entry per process; updates merge into it, and compute() merges the
        # entries a distributed sync concatenated.
        self.add_state("counts", default=[], dist_reduce_fx="cat")
        self.add_state("means", default=[], dist_reduce_fx="cat")
        self.add_state("m2s", default=[], dist_reduce_fx="cat")

    def _add_moments(self, x: torch.Tensor) -> None:
        part = _moments(x, self._covariance)
        if self.counts:
            part = _merge_moments(
                (self.counts[0][0], self.means[0], self.m2s[0].view_as(part[2])), part
            )
        self.counts, self.means, self.m2s = (
            [part[0].reshape(1)],
            [part[1]],
            [part[2].flatten()],
        )

    def _merged_moments(
        self,
    ) -> Optional[tuple[torch.Tensor, torch.Tensor, torch.Tensor]]:
        counts = _cat(self.counts)
        if counts.numel() == 0:
            return None
        means = _cat(self.means).view(counts.numel(), -1)
        m2s = _cat(self.m2s).view(
            counts.numel(), *((means.shape[1],) * (2 if self._covariance else 1))
        )
        total = (counts[0], means[0], m2s[0])
        for i in range(1, counts.numel()):
            total = _merge_moments(total, (counts[i], means[i], m2s[i]))
        return total

    # Sampled metrics.

    def _keep_sample(self, max_samples: int, seed: int) -> None:
        if max_samples < 2:
            raise ValueError(f"max_samples must be >= 2, got {max_samples}")
        self.max_samples = max_samples
        self.seed = seed
        self.add_state("samples", default=[], dist_reduce_fx="cat")
        self.add_state("keys", default=[], dist_reduce_fx="cat")
        # Rows this process has seen; seeds each update's random keys, so a run
        # is reproducible and no two updates draw the same keys.
        self.add_state("rows_seen", default=torch.tensor(0), dist_reduce_fx="sum")

    def _sample(self, rows: Rows, id_hashes: Optional[np.ndarray]) -> None:
        n, dim = len(rows), _width(rows)
        self._check_dim(dim)
        if id_hashes is not None:
            new_keys = _id_keys(id_hashes, self.seed)
        else:
            generator = torch.Generator().manual_seed(
                hash((self.seed, _rank(), int(self.rows_seen)))
            )
            new_keys = torch.rand(n, generator=generator, dtype=torch.float64)
        old_keys = _cat(self.keys).cpu().double()
        all_keys = torch.cat([old_keys, new_keys])
        keys, idx = torch.topk(
            all_keys,
            min(self.max_samples, all_keys.numel()),
            largest=False,
            sorted=True,
        )
        from_old = idx < old_keys.numel()
        sample = torch.empty(len(idx), dim)
        # The sample lives on the CPU whatever the input's device; only the kept rows
        # move.
        sample[from_old] = _cat(self.samples).cpu().view(-1, dim)[idx[from_old]]
        picked = idx[~from_old] - old_keys.numel()
        if picked.numel():
            if isinstance(rows, torch.Tensor):
                sample[~from_old] = _to_matrix(rows[picked.to(rows.device)]).cpu()
            else:
                sample[~from_old] = _to_matrix(rows[picked.numpy()])
        self.samples = [sample.flatten()]
        self.keys = [keys]
        self.rows_seen += n

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        raise NotImplementedError


class EmbeddingCosineSimilarity(_EmbeddingMetric):
    """Mean, std or a percentile of pairwise cosine similarity over distinct row pairs.

    The collapse check: a mean near 1 with a small std means every embedding
    points the same way. A healthy space sits near 0 with a wide spread; the
    percentiles (e.g. ``p5`` and ``p95``) show how wide.

    The mean and std are exact over every pair of rows. Percentiles are read
    from ``num_pairs`` random pairs of a ``max_samples``-row sample, or from
    every pair of it when there are fewer; ``max_samples``, ``num_pairs`` and
    ``seed`` only apply to them.
    """

    def __init__(
        self,
        stat: str = "mean",
        max_samples: int = _DEFAULT_MAX_SAMPLES,
        num_pairs: int = _DEFAULT_NUM_PAIRS,
        seed: int = 0,
        **kwargs: Any,
    ) -> None:
        """Create the metric.

        Args:
            stat: ``"mean"``, ``"std"`` (exact over every row) or a percentile
                such as ``"p95"`` (over sampled pairs).
            max_samples: Most rows kept in the seeded sample.
            num_pairs: Most random distinct pairs scored; every pair when fewer.
            seed: Seed for the row and pair sample, so results are reproducible.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        self.stat = _check_stat(stat)
        if _percentile_of(stat) is not None:
            self.num_pairs = _check_num_pairs(num_pairs)
            self._keep_sample(max_samples, seed)
        else:
            self._keep_moments(covariance=stat == "std")
            # Rows that are not all zeros, i.e. whose normalized vector has norm 1.
            self.add_state(
                "unit_rows",
                default=torch.tensor(0.0, dtype=torch.float64),
                dist_reduce_fx="sum",
            )

    def _consume(self, x: torch.Tensor) -> None:
        self.unit_rows += (torch.linalg.vector_norm(x, dim=1) > 0).sum()
        self._add_moments(torch.nn.functional.normalize(x, dim=1))

    def _finish(self) -> torch.Tensor:
        moments = self._merged_moments()
        if moments is None:
            return torch.tensor(float("nan"))
        n, mean, m2 = moments
        pairs = n * (n - 1) / 2
        if pairs < 1 or (self.stat == "std" and pairs < 2):
            return torch.tensor(float("nan"))
        # Over normalized rows x_i, each with ||x_i||**2 (and ||x_i||**4) of 1, or 0 for
        # a zero row:   sum_{i<j} c_ij    = (||sum_i x_i||**2 - sum_i ||x_i||**2) / 2
        # sum_{i<j} c_ij**2 = (||X^T X||_F**2   - sum_i ||x_i||**4) / 2
        total = n * mean
        pair_mean = (total @ total - self.unit_rows) / 2 / pairs
        if self.stat == "mean":
            return pair_mean
        gram = m2 + n * torch.outer(mean, mean)
        sum_sq = (gram.pow(2).sum() - self.unit_rows) / 2
        return ((sum_sq - pairs * pair_mean**2) / (pairs - 1)).clamp(min=0).sqrt()

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        return _reduce(self.stat, _pair_cosines(x, self.num_pairs, self.seed))


class EmbeddingUniformity(_EmbeddingMetric):
    """Uniformity of the L2-normalized rows on the hypersphere (Wang & Isola, 2020).

    Computed as ``log(mean(exp(-t * ||x - y||**2)))`` over distinct row pairs.

    Lower is better: 0 when every row points the same way, near ``-2 * t``
    when the rows spread evenly in high dimensions. Unlike a mean cosine,
    close pairs dominate, so it also catches rows bunched into a few tight
    clusters. Ignores norms (see ``EmbeddingNorm``), and the scale depends on
    D and ``t``, so compare it across embeddings of the same width. Averaged
    over ``num_pairs`` random pairs of a ``max_samples``-row sample, or every
    pair of it when there are fewer.
    """

    def __init__(
        self,
        t: float = 2.0,
        max_samples: int = _DEFAULT_MAX_SAMPLES,
        num_pairs: int = _DEFAULT_NUM_PAIRS,
        seed: int = 0,
        **kwargs: Any,
    ) -> None:
        """Create the metric.

        Args:
            t: Temperature of the Gaussian potential; must be > 0.
            max_samples: Most rows kept in the seeded sample.
            num_pairs: Most random distinct pairs scored; every pair when fewer.
            seed: Seed for the row and pair sample, so results are reproducible.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        if t <= 0:
            raise ValueError(f"t must be > 0, got {t}")
        self.t = t
        self.num_pairs = _check_num_pairs(num_pairs)
        self._keep_sample(max_samples, seed)

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        if x.shape[0] < 2:
            return torch.tensor(float("nan"))
        # Unit rows: ||x - y||**2 = 2 - 2 * cos. Clamped against rounding below 0.
        sq_dist = (2 - 2 * _pair_cosines(x, self.num_pairs, self.seed)).clamp(min=0)
        # logsumexp keeps the log of a mean of tiny exponentials finite.
        return torch.logsumexp(-self.t * sq_dist, dim=0) - math.log(sq_dist.numel())


class EmbeddingNorm(_EmbeddingMetric):
    """Mean, std or a percentile of the rows' L2 norms.

    Flags exploding or vanishing vectors.

    Exact over every row: it keeps each row's norm, 4 bytes a row.
    """

    def __init__(self, stat: str = "mean", **kwargs: Any) -> None:
        """Create the metric.

        Args:
            stat: ``"mean"``, ``"std"`` or a percentile such as ``"p5"`` of the
                row norms.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        self.stat = _check_stat(stat)
        self.add_state("norms", default=[], dist_reduce_fx="cat")

    def _consume(self, x: torch.Tensor) -> None:
        self.norms.append(torch.linalg.vector_norm(x, dim=1))

    def _finish(self) -> torch.Tensor:
        norms = _cat(self.norms)
        if norms.numel() == 0:
            return torch.tensor(float("nan"))
        return _reduce(self.stat, norms)


class EmbeddingEffectiveRank(_EmbeddingMetric):
    """Effective rank: ``exp(entropy(p))`` over the rows' normalized spectrum.

    ``spectrum`` picks what ``p`` is:

    - ``variance`` (default): ``s**2 / sum(s**2)`` over the singular values of the
      centered rows, i.e. their PCA eigenvalues -- how many directions the rows
      vary along. Squaring makes it strict about weak directions.
    - ``singular``: ``s / sum(s)`` over the singular values of the raw rows. This
      is the effective rank of Roy & Vetterli (2007), which RankMe (Garrido et al.,
      2023) showed tracks downstream linear-probe accuracy across runs of one
      method. It is not centered, so an offset shared by every row lowers it too.

    Ranges from 1 (one direction) to D (spread evenly over every dimension), or to
    the row count if smaller. ``relative`` divides by D, so models with different
    widths compare directly. The two spectra read differently on the same
    embeddings (``singular`` is higher), so only compare values computed with the
    same one. Exact over every row, from the D x D covariance.
    """

    def __init__(
        self, spectrum: str = "variance", relative: bool = False, **kwargs: Any
    ) -> None:
        """Create the metric.

        Args:
            spectrum: ``"variance"`` or ``"singular"``; see the class docstring.
            relative: Divide the rank by the embedding width D.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        if spectrum not in ("variance", "singular"):
            raise ValueError(
                f"spectrum must be 'variance' or 'singular', got {spectrum!r}"
            )
        self.spectrum = spectrum
        self.relative = relative
        self._keep_moments(covariance=True)

    def _consume(self, x: torch.Tensor) -> None:
        self._add_moments(x)

    def _finish(self) -> torch.Tensor:
        moments = self._merged_moments()
        if moments is None:
            return torch.tensor(float("nan"))
        n, mean, m2 = moments
        if not torch.isfinite(m2).all():
            return torch.tensor(float("nan"))
        # The eigenvalues of the centered X^T X are the squared singular values of the
        # centered rows; adding back n * mean mean^T gives the raw rows' instead.
        if self.spectrum == "variance":
            s = torch.linalg.eigvalsh(m2).clamp(min=0)
        else:
            s = (
                torch.linalg.eigvalsh(m2 + n * torch.outer(mean, mean))
                .clamp(min=0)
                .sqrt()
            )
        p = s / s.sum()
        p = p[p > 0]
        rank = torch.exp(-(p * p.log()).sum())
        return rank / mean.numel() if self.relative else rank


class EmbeddingHubness(_EmbeddingMetric):
    """Skewness of the k-occurrence distribution under cosine similarity.

    Follows Radovanović et al. (2010). A row's k-occurrence is how many other
    rows list it among their ``k`` nearest neighbors; it averages exactly
    ``k``. A right skew means hubs -- a few rows that are a neighbor of nearly
    everyone -- and orphans that are nobody's neighbor, which retrieval would
    recommend everywhere and never, respectively. Near 0 is balanced; above ~1
    is notable hubness. Neighbors are searched within a ``max_samples``-row
    sample, and the value depends on ``k``, D and the sample size, so compare
    it at the same ``k`` and ``max_samples``.
    Returns nan when the sample has no more than ``k`` rows, or every row has the
    same k-occurrence (skewness is undefined).
    """

    def __init__(
        self,
        k: int = 10,
        max_samples: int = _DEFAULT_MAX_SAMPLES,
        seed: int = 0,
        **kwargs: Any,
    ) -> None:
        """Create the metric.

        Args:
            k: Number of nearest neighbors each row lists; must be >= 1.
            max_samples: Most rows kept in the seeded sample.
            seed: Seed for the row sample, so results are reproducible.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        if k < 1:
            raise ValueError(f"k must be >= 1, got {k}")
        self.k = k
        self._keep_sample(max_samples, seed)

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        n = x.shape[0]
        # NaN similarities would still yield (arbitrary) neighbors; propagate instead.
        if n <= self.k or x.isnan().any():
            return torch.tensor(float("nan"))
        x = torch.nn.functional.normalize(x, dim=1)
        occurrences = torch.zeros(n)
        step = max(1, _STACK_CHUNK_ELEMENTS // n)
        for start in range(0, n, step):
            sim = x[start : start + step] @ x.T
            rows = torch.arange(len(sim))
            sim[rows, rows + start] = float("-inf")  # a row is not its own neighbor
            neighbors = sim.topk(self.k, dim=1).indices
            occurrences += torch.bincount(neighbors.flatten(), minlength=n)
        centered = occurrences - occurrences.mean()
        std = centered.pow(2).mean().sqrt()
        if std == 0:
            return torch.tensor(float("nan"))
        return centered.pow(3).mean() / std**3


class EmbeddingSilhouette(_EmbeddingMetric):
    """Silhouette score (-1 to 1) of a K-means clustering of a row sample.

    The sample holds at most ``max_samples`` rows.

    Higher means the space has clearer group structure at ``n_clusters``.
    Returns nan when the sample cannot form at least two clusters.
    """

    def __init__(
        self,
        n_clusters: int = 8,
        max_samples: int = 20_000,
        seed: int = 0,
        **kwargs: Any,
    ) -> None:
        """Create the metric.

        Args:
            n_clusters: Number of K-means clusters to score.
            max_samples: Most rows kept in the seeded sample.
            seed: Seed for the row sample, so results are reproducible.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        if n_clusters < 2:
            raise ValueError(f"n_clusters must be >= 2, got {n_clusters}")
        self.n_clusters = n_clusters
        self._keep_sample(max_samples, seed)

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        # Imported lazily so loading this package (every evaluator run) does not pay for
        # sklearn.
        from sklearn.cluster import KMeans
        from sklearn.metrics import silhouette_score

        data = x.cpu().numpy()
        if len(data) <= self.n_clusters:
            return torch.tensor(float("nan"))
        labels = KMeans(
            n_clusters=self.n_clusters, random_state=self.seed, n_init=3
        ).fit_predict(data)
        if len(set(labels)) < 2:
            return torch.tensor(float("nan"))
        return torch.tensor(float(silhouette_score(data, labels)))


class EmbeddingDeadDimensions(_EmbeddingMetric):
    """Fraction (0 to 1) of dimensions that barely vary across rows.

    A dimension is dead when its std over every row is at most ``threshold``
    times the largest dimension's std. Relative, so one default fits embeddings
    of any scale -- which also means a space that shrinks as a whole keeps its
    score; ``EmbeddingNorm`` catches that instead. One repeated point scores 1.
    """

    def __init__(self, threshold: float = 1e-3, **kwargs: Any) -> None:
        """Create the metric.

        Args:
            threshold: Std ratio to the widest dimension at or below which a
                dimension counts as dead; in [0, 1).
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        if not 0 <= threshold < 1:
            raise ValueError(f"threshold must be in [0, 1), got {threshold}")
        self.threshold = threshold
        self._keep_moments(covariance=False)

    def _consume(self, x: torch.Tensor) -> None:
        self._add_moments(x)

    def _finish(self) -> torch.Tensor:
        moments = self._merged_moments()
        if moments is None:
            return torch.tensor(float("nan"))
        n, _, m2 = moments
        std = (m2 / (n - 1)).sqrt()
        # A NaN std would compare False and pass as alive; propagate it instead.
        if std.isnan().any():
            return torch.tensor(float("nan"))
        return (std <= self.threshold * std.max()).float().mean()


def _is_zero_row(row: Any) -> bool:
    """All elements zero; a missing element (NaN, None, pd.NA) makes it nonzero."""
    values = np.asarray(row, dtype=object)
    return not pd.isna(values).any() and not values.astype(float).any()


class _EmbeddingRowRate(Metric):
    """Share of all rows a subclass flags in ``_count``, over every row -- no sampling.

    Gets the column as-is -- one value per row, missing ones included -- so it can
    count the rows a stacked tensor could not hold.
    """

    higher_is_better = False
    full_state_update = False
    _metric_type = "embedding"

    def __init__(self, **kwargs: Any) -> None:
        super().__init__(**kwargs)
        self.add_state("matching", default=torch.tensor(0), dist_reduce_fx="sum")
        self.add_state("total", default=torch.tensor(0), dist_reduce_fx="sum")

    def update(self, preds: Any) -> None:
        self.matching += self._count(preds)
        self.total += len(preds)

    def compute(self) -> torch.Tensor:
        if self.total == 0:
            return torch.tensor(float("nan"))
        return self.matching.float() / self.total

    def _count(self, preds: Any) -> int:
        raise NotImplementedError


class EmbeddingNullRate(_EmbeddingRowRate):
    """Share of rows with no embedding at all.

    For example entities a left join found no vector for.
    """

    def _count(self, preds: Any) -> int:
        return int(_null_mask(preds).sum())


class EmbeddingZeroRate(_EmbeddingRowRate):
    """Share of rows whose embedding is all zeros, a common missing-vector filler.

    Rows with no embedding at all do not count here; ``EmbeddingNullRate``
    reports those. Both are shares of all rows, so they add up.
    """

    def _count(self, preds: Any) -> int:
        if isinstance(preds, torch.Tensor):
            return int((preds == 0).all(dim=1).sum())
        rows = _non_null_rows(preds)
        if len(rows) == 0:
            return 0
        count = 0
        for chunk in _row_chunks(rows):
            try:
                matrix = np.vstack(chunk)
            except ValueError:  # rows of mixed lengths cannot stack
                matrix = None
            if matrix is not None and matrix.dtype != object:
                # any() reduces without materializing an elementwise mask; a NaN is
                # nonzero.
                count += int((~matrix.any(axis=1)).sum())
            else:
                # Mixed lengths, or missing elements (None, pd.NA) that make the stack
                # an object array, whose reduction would call bool() on them and raise.
                count += sum(_is_zero_row(row) for row in chunk)
        return count
