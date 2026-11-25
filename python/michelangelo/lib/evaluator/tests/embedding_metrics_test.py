"""Tests for the intrinsic embedding-quality metrics."""

import math
import unittest

import numpy as np
import torch

from michelangelo.lib.evaluator.embedding_metrics import (
    EmbeddingCosineSimilarity,
    EmbeddingEffectiveRank,
    EmbeddingNorm,
    EmbeddingSilhouette,
)


def _score(metric, *batches: torch.Tensor) -> float:
    for batch in batches:
        metric.update(batch)
    return metric.compute().item()


class EmbeddingCosineSimilarityTest(unittest.TestCase):
    """EmbeddingCosineSimilarity."""

    def test_collapsed_space_has_mean_one(self) -> None:
        """Collapsed space has mean one."""
        x = torch.tensor([[1.0, 2.0], [2.0, 4.0], [0.5, 1.0]])
        self.assertAlmostEqual(_score(EmbeddingCosineSimilarity(), x), 1.0, places=5)

    def test_orthogonal_rows_have_mean_zero(self) -> None:
        """Orthogonal rows have mean zero."""
        self.assertAlmostEqual(
            _score(EmbeddingCosineSimilarity(), torch.eye(4)), 0.0, places=5
        )

    def test_std_over_distinct_pairs(self) -> None:
        """Std over distinct pairs."""
        # Pairs: (a,b)=1, (a,c)=0, (b,c)=0 -> sample std of [1, 0, 0].
        x = torch.tensor([[1.0, 0.0], [1.0, 0.0], [0.0, 1.0]])
        expected = torch.tensor([1.0, 0.0, 0.0]).std().item()
        self.assertAlmostEqual(
            _score(EmbeddingCosineSimilarity(stat="std"), x), expected, places=5
        )

    def test_percentiles_over_distinct_pairs(self) -> None:
        """Percentiles read the sorted distinct-pair similarities."""
        # Pairs: (a,b)=1, (a,c)=0, (b,c)=0 -> sorted [0, 0, 1].
        x = torch.tensor([[1.0, 0.0], [1.0, 0.0], [0.0, 1.0]])
        for stat, expected in (("p0", 0.0), ("p50", 0.0), ("p75", 0.5), ("p100", 1.0)):
            self.assertAlmostEqual(
                _score(EmbeddingCosineSimilarity(stat=stat), x), expected, places=5
            )

    def test_percentile_matches_numpy_linear_interpolation(self) -> None:
        """Percentiles interpolate linearly, like numpy."""
        x = torch.randn(40, 8, generator=torch.Generator().manual_seed(0))
        normed = torch.nn.functional.normalize(x, dim=1)
        upper = torch.triu(torch.ones(40, 40, dtype=torch.bool), diagonal=1)
        pairs = (normed @ normed.T)[upper]
        for stat, q in (("p5", 5.0), ("p25", 25.0), ("p99.5", 99.5)):
            expected = float(np.percentile(pairs.numpy(), q))
            self.assertAlmostEqual(
                _score(EmbeddingCosineSimilarity(stat=stat), x), expected, places=5
            )

    def test_invalid_stat_raises(self) -> None:
        """Invalid stat raises."""
        for stat in ("median", "p101", "q5", "p", "5", 5):
            with self.subTest(stat=stat), self.assertRaises(ValueError):
                EmbeddingCosineSimilarity(stat=stat)


class EmbeddingNormTest(unittest.TestCase):
    """EmbeddingNorm."""

    def test_mean_and_std_of_norms(self) -> None:
        """Mean and std of norms."""
        x = torch.tensor([[3.0, 4.0], [0.0, 1.0]])  # norms 5 and 1
        self.assertAlmostEqual(_score(EmbeddingNorm(), x), 3.0, places=5)
        self.assertAlmostEqual(
            _score(EmbeddingNorm(stat="std"), x),
            torch.tensor([5.0, 1.0]).std().item(),
            places=5,
        )

    def test_percentile_of_norms(self) -> None:
        """Percentiles of the row norms."""
        x = torch.tensor([[3.0, 4.0], [1.0, 0.0]])  # norms 5 and 1
        self.assertAlmostEqual(_score(EmbeddingNorm(stat="p50"), x), 3.0, places=5)
        self.assertAlmostEqual(_score(EmbeddingNorm(stat="p100"), x), 5.0, places=5)

    def test_percentile_propagates_nan(self) -> None:
        """A nan norm makes every percentile nan, like mean and std."""
        x = torch.tensor([[3.0, 4.0], [1.0, 0.0], [float("nan"), 0.0]])
        self.assertTrue(math.isnan(_score(EmbeddingNorm(stat="p5"), x)))

    def test_percentile_preserves_inf(self) -> None:
        """Infinite norms stay infinite instead of turning into nan."""
        x = torch.tensor([[float("inf"), 0.0], [float("inf"), 0.0]])
        self.assertEqual(_score(EmbeddingNorm(stat="p100"), x), float("inf"))
        self.assertEqual(_score(EmbeddingNorm(stat="p50"), x), float("inf"))


class EmbeddingEffectiveRankTest(unittest.TestCase):
    """EmbeddingEffectiveRank."""

    def test_rank_one_data_is_one(self) -> None:
        """Rank one data is one."""
        x = torch.arange(1.0, 11.0).unsqueeze(1) * torch.tensor([[1.0, 2.0, 3.0]])
        self.assertAlmostEqual(_score(EmbeddingEffectiveRank(), x), 1.0, places=3)

    def test_isotropic_data_is_close_to_dimension(self) -> None:
        """Isotropic data is close to dimension."""
        x = torch.randn(4000, 8, generator=torch.Generator().manual_seed(0))
        self.assertGreater(_score(EmbeddingEffectiveRank(), x), 7.5)

    def test_singular_spectrum_is_not_centered(self) -> None:
        """The singular spectrum keeps the dimension centering would remove."""
        # The identity's rows span all 4 dimensions, but centering removes one.
        singular = EmbeddingEffectiveRank(spectrum="singular")
        self.assertAlmostEqual(_score(singular, torch.eye(4)), 4.0, places=4)
        self.assertAlmostEqual(
            _score(EmbeddingEffectiveRank(), torch.eye(4)), 3.0, places=4
        )

    def test_singular_spectrum_weighs_singular_values_unsquared(self) -> None:
        """The singular spectrum normalizes s, not s**2."""
        # Singular values [10, 1, 1, 1]: p = [10, 1, 1, 1] / 13 (about 2.21), where
        # squaring them first would give [100, 1, 1, 1] / 103 (about 1.18).
        p = torch.tensor([10.0, 1.0, 1.0, 1.0]) / 13
        expected = torch.exp(-(p * p.log()).sum()).item()
        x = torch.diag(torch.tensor([10.0, 1.0, 1.0, 1.0]))
        self.assertAlmostEqual(
            _score(EmbeddingEffectiveRank(spectrum="singular"), x), expected, places=4
        )

    def test_relative_divides_by_dimension(self) -> None:
        """Relative rank divides by D."""
        metric = EmbeddingEffectiveRank(spectrum="singular", relative=True)
        self.assertAlmostEqual(_score(metric, torch.eye(4)), 1.0, places=4)

    def test_invalid_spectrum_raises(self) -> None:
        """Invalid spectrum raises."""
        with self.assertRaises(ValueError):
            EmbeddingEffectiveRank(spectrum="eigen")


class EmbeddingSilhouetteTest(unittest.TestCase):
    """EmbeddingSilhouette."""

    def test_well_separated_blobs_score_near_one(self) -> None:
        """Well separated blobs score near one."""
        g = torch.Generator().manual_seed(0)
        blobs = torch.cat(
            [
                torch.randn(50, 2, generator=g) * 0.01,
                torch.randn(50, 2, generator=g) * 0.01 + 10.0,
            ]
        )
        self.assertGreater(_score(EmbeddingSilhouette(n_clusters=2), blobs), 0.95)

    def test_too_few_rows_returns_nan(self) -> None:
        """Too few rows returns nan."""
        self.assertTrue(
            math.isnan(_score(EmbeddingSilhouette(n_clusters=8), torch.randn(3, 2)))
        )


class EmbeddingMetricSamplingTest(unittest.TestCase):
    """Sampling and accumulation shared by every embedding metric."""

    def test_sampled_result_is_reproducible(self) -> None:
        """Sampled result is reproducible."""
        x = torch.randn(500, 4, generator=torch.Generator().manual_seed(1))
        first = _score(EmbeddingCosineSimilarity(stat="std", max_samples=50), x)
        second = _score(EmbeddingCosineSimilarity(stat="std", max_samples=50), x)
        self.assertEqual(first, second)

    def test_multiple_updates_accumulate(self) -> None:
        """Multiple updates accumulate."""
        x = torch.tensor([[3.0, 4.0], [0.0, 1.0]])
        self.assertAlmostEqual(_score(EmbeddingNorm(), x[:1], x[1:]), 3.0, places=5)

    def test_empty_scope_returns_nan(self) -> None:
        """Empty scope returns nan."""
        self.assertTrue(math.isnan(_score(EmbeddingNorm(), torch.empty(0))))

    def test_non_matrix_input_raises(self) -> None:
        """Non matrix input raises."""
        with self.assertRaises(ValueError):
            EmbeddingNorm().update(torch.tensor([1.0, 2.0]))

    def test_sample_weighs_updates_by_their_row_count(self) -> None:
        """Sample weighs updates by their row count."""
        # 9,000 rows of norm 1 then 1,000 of norm 9: the true mean norm is 1.8.
        # Capping each update separately would weigh both equally (mean 5).
        metric = EmbeddingNorm(max_samples=500)
        metric.update(torch.tensor([[1.0, 0.0]]).repeat(9_000, 1))
        metric.update(torch.tensor([[9.0, 0.0]]).repeat(1_000, 1))
        self.assertAlmostEqual(metric.compute().item(), 1.8, delta=0.4)

    def test_state_stays_bounded_across_updates(self) -> None:
        """State stays bounded across updates."""
        for step in ("update", "forward"):
            with self.subTest(step=step):
                metric = EmbeddingNorm(max_samples=100)
                for _ in range(50):
                    getattr(metric, step)(torch.randn(80, 4))
                self.assertEqual(sum(t.numel() for t in metric.samples), 100 * 4)

    def test_compute_after_distributed_sync(self) -> None:
        """Compute after distributed sync."""
        # Stand-ins for all_gather across two ranks: one whose peer saw the same
        # rows, and one whose peer saw none. A sync leaves the list states as
        # concatenated tensors.
        peers = {
            "same rows": lambda t, group=None: [t, t],
            "no rows": lambda t, group=None: [
                t,
                t if t.dim() == 0 else t[:0],
            ],
        }
        x = torch.tensor([[3.0, 4.0], [0.0, 1.0]])
        for name, gather in peers.items():
            with self.subTest(peer=name):
                metric = EmbeddingNorm(
                    dist_sync_fn=gather, distributed_available_fn=lambda: True
                )
                self.assertAlmostEqual(_score(metric, x), 3.0, places=5)
