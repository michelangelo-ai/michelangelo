import unittest
import tempfile
import torch

from michelangelo.sdk.core.models.tte.tte_model import AbstractTwoTowerModel, create_tte_model, SharedInBatchTwoTower


import os

dir_path = os.path.dirname(os.path.realpath(__file__))


class TestSharedInBatchTwoTower(unittest.TestCase):
    def test_compute_hit_rate(self):
        logits = torch.randn(16, 16)
        labels = torch.arange(0, 16, device=logits.device)
        hit_rate_5 = SharedInBatchTwoTower.compute_hit_rate(logits, labels, 5)
        self.assertGreater(hit_rate_5, 0.0)
        hit_rate_10 = SharedInBatchTwoTower.compute_hit_rate(logits, labels, 10)
        self.assertGreater(hit_rate_10, hit_rate_5)
        hit_rate_15 = SharedInBatchTwoTower.compute_hit_rate(logits, labels, 15)
        self.assertGreater(hit_rate_15, hit_rate_10)

    def test_loss_shared_tte(self):
        tte = SharedInBatchTwoTower(
            text_model_class_name="TextEncoder",
            query_text_column="query",
            item_text_column="item",
            query_text_max_len=16,
            item_text_max_len=16,
            pretrained_text_model_path=os.path.join(dir_path, "unit_test_model_files"),
        )
        batch = {"query": ["hello me", "yes I am"], "item": ["hello me", "yes I am"]}
        loss, scores, labels = tte.get_loss(batch)
        assert scores.shape == (2, 2)
        assert labels.shape == (2,)
        assert loss >= 0.0
        temp_dir = tempfile.TemporaryDirectory().name
        tte.save_model(temp_dir)

    def test_loss_created_shared_tte(self):
        tte = create_tte_model(
            "SharedInBatchTwoTower",
            text_model_class_name="TextEncoder",
            query_text_column="query",
            item_text_column="item",
            query_text_max_len=16,
            item_text_max_len=16,
            pretrained_text_model_path=os.path.join(dir_path, "unit_test_model_files"),
        )
        batch = {"query": ["hello me", "yes I am"], "item": ["hello me", "yes I am"]}
        loss, scores, labels = tte.get_loss(batch)
        assert scores.shape == (2, 2)
        assert labels.shape == (2,)
        assert loss >= 0.0

    def test_loss_tte(self):
        tte = create_tte_model(
            "InBatchTwoTower",
            text_model_class_name="TextEncoder",
            query_text_column="query",
            item_text_column="item",
            query_text_max_len=16,
            item_text_max_len=16,
            pretrained_text_model_path=os.path.join(dir_path, "unit_test_model_files"),
            item_selection_prob_column="item_prob",
            metrics_k_all=[1],
        )
        batch = {"query": ["hello me", "yes I am"], "item": ["hello me", "yes I am"], "item_prob": torch.tensor([0.1, 0.1])}
        loss, scores, labels = tte.get_loss(batch)
        assert scores.shape == (2, 2)
        assert labels.shape == (2,)
        assert loss >= 0.0

        train_log = tte.get_log_metrics(loss, scores, labels, True)
        assert "train_loss" in train_log
        assert "train_hit_rate_at_1" in train_log
        val_log = tte.get_log_metrics(loss, scores, labels, False)
        assert "val_loss" in val_log
        assert "val_hit_rate_at_1" in val_log
        temp_dir = tempfile.TemporaryDirectory().name
        tte.save_model(temp_dir)

    def test_abstract_tte(self):
        with self.assertRaises(TypeError):
            AbstractTwoTowerModel(
                query_text_column="query",
                item_text_column="item",
                query_text_max_len=16,
                item_text_max_len=16,
            )
