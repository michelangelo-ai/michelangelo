"""Intrinsic embedding-quality metrics as TorchMetrics.

Label-free checks on the geometry of an embedding column -- is the space
collapsed, how many dimensions carry variance, does it cluster. Each metric
reads an ``[N, D]`` prediction tensor (the evaluator stacks a list/array
column into one) and returns a single scalar.

They are tagged ``_metric_type = "aggregation"``, so the evaluator calls
``update(preds)`` without a target. ``columns.target_col`` is still required
by the config schema. The metric never reads it, but the evaluator still loads
it from the dataset (without a ``data_preprocessor`` it is part of the parquet
column projection), so point it at a cheap, narrow column such as a small ID
or label column, not a wide one like another embedding:

    - name: user_emb_cosine_mean
      metric: michelangelo.lib.evaluator.embedding_metrics.EmbeddingCosineSimilarity
      params: {stat: mean}
      columns:
        prediction_col: user_embedding
        target_col: user_id   # required by schema; loaded but not read

``EmbeddingCosineSimilarity`` and ``EmbeddingNorm`` take a ``stat``: ``mean``,
``std``, or a percentile such as ``p5`` / ``p50`` / ``p95``. Configure one metric
per percentile to read the shape of the distribution, not just its center.

Every metric works on a seeded random sample of at most ``max_samples`` rows,
which bounds memory and keeps the pairwise metrics at ``max_samples**2`` work
regardless of dataset size, while staying reproducible across runs.
"""

import math
import re
from typing import Any, Optional, Union

import torch
from torchmetrics import Metric

__all__ = [
    "EmbeddingCosineSimilarity",
    "EmbeddingDeadDimensions",
    "EmbeddingEffectiveRank",
    "EmbeddingHubness",
    "EmbeddingNorm",
    "EmbeddingSilhouette",
    "EmbeddingUniformity",
]

_STATS = {"mean": torch.mean, "std": torch.std}
# "p5", "p50", "p99.9": a percentile in [0, 100].
_PERCENTILE_STAT = re.compile(r"p(\d+(?:\.\d+)?)")


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
        return _STATS[stat](values)
    # sort() moves NaNs to the end, which would hide them from low percentiles;
    # propagate them like mean/std and numpy do.
    if values.numel() == 0 or values.isnan().any():
        return torch.tensor(float("nan"))
    # Sort-based instead of torch.quantile, which rejects inputs over 2**24
    # elements -- fewer than the pairs of a sample larger than ~5.8k rows.
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


def _cat(state: Union[list[torch.Tensor], torch.Tensor]) -> torch.Tensor:
    """A list state as one tensor; a distributed sync has already concatenated it."""
    if isinstance(state, torch.Tensor):
        return state
    return torch.cat(state) if state else torch.empty(0)


class _EmbeddingMetric(Metric):
    """Collects a bounded, seeded sample of embedding rows; subclasses score it.

    The sample is bottom-k: every row draws a random key and the
    ``max_samples`` rows with the smallest keys are kept. That is a uniform
    sample of every row seen, however the rows were split into updates, and
    two kept sets merge by concatenating them and keeping the smallest keys
    again -- which is what lets the states cat-reduce across processes. Rows
    are stored flattened so every state is 1-D, which a process that saw no
    rows can still sync.
    """

    higher_is_better = None
    # forward() then updates the global state directly instead of appending
    # each batch's kept set to it, so the state stays bounded there too.
    full_state_update = True
    _metric_type = "aggregation"

    def __init__(self, max_samples: int = 5_000, seed: int = 0, **kwargs: Any) -> None:
        """Create the metric.

        Args:
            max_samples: Most rows kept for scoring; larger inputs are sampled.
            seed: Seed for the row sample, so results are reproducible.
            **kwargs: Passed through to ``torchmetrics.Metric``.
        """
        super().__init__(**kwargs)
        if max_samples < 2:
            raise ValueError(f"max_samples must be >= 2, got {max_samples}")
        self.max_samples = max_samples
        self.seed = seed
        self.add_state("samples", default=[], dist_reduce_fx="cat")
        self.add_state("keys", default=[], dist_reduce_fx="cat")
        # Rows this process has seen; seeds each update's keys, so a run is
        # reproducible and no two updates draw the same keys.
        self.add_state("rows_seen", default=torch.tensor(0), dist_reduce_fx="sum")

    def update(self, preds: torch.Tensor) -> None:
        """Add a batch of ``[N, D]`` embedding rows to the sample."""
        # An empty scope (e.g. a filter that left no rows) arrives as a 1-D
        # empty tensor, since there were no rows to stack.
        if preds.numel() == 0:
            return
        if preds.dim() != 2:
            raise ValueError(
                f"expected [N, D] embeddings, got shape {tuple(preds.shape)}"
            )
        n, d = preds.shape
        generator = torch.Generator().manual_seed(
            hash((self.seed, _rank(), int(self.rows_seen)))
        )
        keys = torch.rand(n, generator=generator).to(preds.device)
        rows = torch.cat([*self.samples, preds.float().flatten()]).view(-1, d)
        rows, keys = self._smallest_keys(rows, torch.cat([*self.keys, keys]))
        self.samples = [rows.flatten()]
        self.keys = [keys]
        self.rows_seen += n

    def compute(self) -> torch.Tensor:
        """Score the collected sample; nan when no rows were seen."""
        # A distributed sync hands the list states over already concatenated.
        keys = _cat(self.keys)
        if keys.numel() == 0:
            return torch.tensor(float("nan"))
        rows = _cat(self.samples).view(keys.numel(), -1)
        return self._score(self._smallest_keys(rows, keys)[0])

    def _smallest_keys(
        self, rows: torch.Tensor, keys: torch.Tensor
    ) -> tuple[torch.Tensor, torch.Tensor]:
        if keys.numel() <= self.max_samples:
            return rows, keys
        idx = torch.topk(keys, self.max_samples, largest=False).indices
        return rows[idx], keys[idx]

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        raise NotImplementedError


class EmbeddingCosineSimilarity(_EmbeddingMetric):
    """Mean, std or a percentile of pairwise cosine similarity over row pairs.

    The collapse check: a mean near 1 with a small std means every embedding
    points the same way. A healthy space sits near 0 with a wide spread; the
    percentiles (e.g. ``p5`` and ``p95``) show how wide.
    """

    def __init__(self, stat: str = "mean", **kwargs: Any) -> None:
        """Create the metric.

        Args:
            stat: ``"mean"``, ``"std"`` or a percentile such as ``"p95"`` of the
                pairwise similarities.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        self.stat = _check_stat(stat)

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        x = torch.nn.functional.normalize(x, dim=1)
        n = x.shape[0]
        upper = torch.triu(
            torch.ones(n, n, dtype=torch.bool, device=x.device), diagonal=1
        )
        return _reduce(self.stat, (x @ x.T)[upper])


class EmbeddingUniformity(_EmbeddingMetric):
    """Uniformity of the L2-normalized rows on the hypersphere (Wang & Isola, 2020).

    Computed as ``log(mean(exp(-t * ||x - y||**2)))`` over distinct row pairs.
    Lower is better: 0 when every row points the same way, near ``-2 * t``
    when the rows spread evenly in high dimensions. Unlike a mean cosine,
    close pairs dominate, so it also catches rows bunched into a few tight
    clusters. Ignores norms (see ``EmbeddingNorm``), and the scale depends on
    D and ``t``, so compare it across embeddings of the same width.
    """

    def __init__(self, t: float = 2.0, **kwargs: Any) -> None:
        """Create the metric.

        Args:
            t: Temperature of the Gaussian potential; must be > 0.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        if t <= 0:
            raise ValueError(f"t must be > 0, got {t}")
        self.t = t

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        n = x.shape[0]
        if n < 2:
            return torch.tensor(float("nan"))
        x = torch.nn.functional.normalize(x, dim=1)
        upper = torch.triu(
            torch.ones(n, n, dtype=torch.bool, device=x.device), diagonal=1
        )
        # Unit rows: ||x - y||**2 = 2 - 2 * cos. Clamped against rounding below 0.
        sq_dist = (2 - 2 * (x @ x.T)[upper]).clamp(min=0)
        # logsumexp keeps the log of a mean of tiny exponentials finite.
        return torch.logsumexp(-self.t * sq_dist, dim=0) - math.log(sq_dist.numel())


class EmbeddingNorm(_EmbeddingMetric):
    """Mean, std or a percentile of the rows' L2 norms.

    Flags exploding or vanishing vectors.
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

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        return _reduce(self.stat, torch.linalg.vector_norm(x, dim=1))


class EmbeddingEffectiveRank(_EmbeddingMetric):
    """Effective rank: ``exp(entropy(p))`` over the sample's normalized spectrum.

    ``spectrum`` picks what ``p`` is:

    - ``variance`` (default): ``s**2 / sum(s**2)`` over the singular values of the
      centered sample, i.e. its PCA eigenvalues -- how many directions the rows
      vary along. Squaring makes it strict about weak directions.
    - ``singular``: ``s / sum(s)`` over the singular values of the raw sample. This
      is the effective rank of Roy & Vetterli (2007), which RankMe (Garrido et al.,
      2023) showed tracks downstream linear-probe accuracy across runs of one
      method. It is not centered, so an offset shared by every row lowers it too.

    Ranges from 1 (one direction) to D (spread evenly over every dimension), or to
    the row count if smaller, so keep ``max_samples`` well above D. ``relative``
    divides by D, so models with different widths compare directly. The two
    spectra read differently on the same embeddings (``singular`` is higher), so
    only compare values computed with the same one.
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

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        if self.spectrum == "variance":
            s = torch.linalg.svdvals(x - x.mean(dim=0, keepdim=True)) ** 2
        else:
            s = torch.linalg.svdvals(x)
        p = s / s.sum()
        p = p[p > 0]
        rank = torch.exp(-(p * p.log()).sum())
        return rank / x.shape[1] if self.relative else rank


class EmbeddingHubness(_EmbeddingMetric):
    """Skewness of the k-occurrence distribution under cosine similarity.

    Follows Radovanović et al. (2010). A row's k-occurrence is how many other
    rows list it among their ``k`` nearest neighbors; it averages exactly
    ``k``. A right skew means hubs -- a few rows that are a neighbor of nearly
    everyone -- and orphans that are nobody's neighbor, which retrieval would
    recommend everywhere and never, respectively. Near 0 is balanced; above ~1
    is notable hubness. The value depends on ``k``, D and the sample size, so
    compare it at the same ``k`` and ``max_samples``. Returns nan when the
    sample has no more than ``k`` rows, or every row has the same k-occurrence
    (skewness is undefined).
    """

    def __init__(self, k: int = 10, **kwargs: Any) -> None:
        """Create the metric.

        Args:
            k: Number of nearest neighbors each row lists; must be >= 1.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        if k < 1:
            raise ValueError(f"k must be >= 1, got {k}")
        self.k = k

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        n = x.shape[0]
        # NaN similarities would still yield (arbitrary) neighbors; propagate.
        if n <= self.k or x.isnan().any():
            return torch.tensor(float("nan"))
        x = torch.nn.functional.normalize(x, dim=1)
        sim = x @ x.T
        sim.fill_diagonal_(float("-inf"))  # a row is not its own neighbor
        neighbors = sim.topk(self.k, dim=1).indices
        occurrences = torch.bincount(neighbors.flatten(), minlength=n).float()
        centered = occurrences - occurrences.mean()
        std = centered.pow(2).mean().sqrt()
        if std == 0:
            return torch.tensor(float("nan"))
        return centered.pow(3).mean() / std**3


class EmbeddingSilhouette(_EmbeddingMetric):
    """Silhouette score (-1 to 1) of a K-means clustering of the sample.

    Higher means the space has clearer group structure at ``n_clusters``.
    Returns nan when the sample cannot form at least two clusters.
    """

    def __init__(self, n_clusters: int = 8, **kwargs: Any) -> None:
        """Create the metric.

        Args:
            n_clusters: Number of K-means clusters to score.
            **kwargs: Passed through to ``_EmbeddingMetric``.
        """
        super().__init__(**kwargs)
        if n_clusters < 2:
            raise ValueError(f"n_clusters must be >= 2, got {n_clusters}")
        self.n_clusters = n_clusters

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        # Imported lazily so loading this module does not pay for sklearn.
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

    A dimension is dead when its std over the sample is at most ``threshold``
    times the largest dimension's std. Relative, so one default fits embeddings
    of any scale -- which also means a space that shrinks as a whole keeps its
    score; ``EmbeddingNorm`` catches that instead. A sample of one repeated
    point scores 1.
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

    def _score(self, x: torch.Tensor) -> torch.Tensor:
        std = x.std(dim=0)
        # A NaN std would compare False and pass as alive; propagate it instead.
        if std.isnan().any():
            return torch.tensor(float("nan"))
        return (std <= self.threshold * std.max()).float().mean()
