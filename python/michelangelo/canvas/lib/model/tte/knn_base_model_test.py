"""Tests for KNN base model module."""
# ruff: noqa: D101, D102
import os
import unittest

import numpy as np
import pandas as pd
import pyarrow as pa
import torch
from torch import nn

from michelangelo.canvas.lib.model.tte.knn_base_model import (
    KNNModel,
    MultiGPUKNNFFNModel,
    MultiGPUKNNModel,
)

assert pa


class KNNModelTest(unittest.TestCase):
    def test_load_category_filters(self):
        knn_model = KNNModel(
            item_id_col="", category_filter_columns=["cat_col1", "cat_col2"]
        )
        data = [
            {"cat_col1": "a1", "cat_col2": "b1", "embedding": np.array([1, 2, 3])},
            {"cat_col1": "a2", "cat_col2": "b2", "embedding": np.array([1, 2, 3])},
            {"cat_col1": "a3", "cat_col2": "b2", "embedding": np.array([1, 2, 3])},
            {"cat_col1": "a1", "cat_col2": "b1", "embedding": np.array([1, 2, 3])},
        ]
        df = pd.DataFrame(data)
        cat2id_map = knn_model._load_category_filters(df)
        assert len(cat2id_map["cat_col1"]) == 3
        assert len(cat2id_map["cat_col2"]) == 2

    def test_load_set_filters(self):
        knn_model = KNNModel(
            item_id_col="", set_filter_columns=["set_col1", "set_col2"]
        )
        data = [
            {
                "set_col1": ["a1", "a2"],
                "set_col2": [1, 2],
                "embedding": np.array([1, 2, 3]),
            },
            {
                "set_col1": ["a2"],
                "set_col2": [2, 3, 4],
                "embedding": np.array([1, 2, 3]),
            },
            {"set_col1": ["a3"], "set_col2": [3], "embedding": np.array([1, 2, 3])},
            {"set_col1": ["a1"], "set_col2": [1], "embedding": np.array([1, 2, 3])},
        ]
        df = pd.DataFrame(data)
        set_filter_cat2id_map = knn_model._load_set_filters(df)
        assert len(set_filter_cat2id_map["set_col1"]) == 3
        assert len(set_filter_cat2id_map["set_col2"]) == 4

    def test_category2tensor(self):
        knn_model = KNNModel(
            item_id_col="", category_filter_columns=["cat_col1", "cat_col2"]
        )
        data = [
            {"cat_col1": "a1", "cat_col2": "b1", "embedding": np.array([1, 2, 3])},
            {"cat_col1": "a2", "cat_col2": "b2", "embedding": np.array([1, 2, 3])},
            {"cat_col1": "a3", "cat_col2": "b2", "embedding": np.array([1, 2, 3])},
            {"cat_col1": "a1", "cat_col2": "b1", "embedding": np.array([1, 2, 3])},
        ]
        df = pd.DataFrame(data)
        cat2id_map = knn_model._load_category_filters(df)
        cat2tensor_map = knn_model._category2tensor(df, cat2id_map, device="cpu")
        assert cat2tensor_map["cat_col1"].shape == (4,)

    def test_set_filter2tensor(self):
        knn_model = KNNModel(
            item_id_col="",
            set_filter_columns=["set_col1", "set_col2"],
            set_filter_max_sizes={"set_col1": 3, "set_col2": 4},
        )
        data = [
            {
                "set_col1": ["a1", "a2"],
                "set_col2": [1, 2],
                "embedding": np.array([1, 2, 3]),
            },
            {
                "set_col1": ["a2"],
                "set_col2": [2, 3, 4],
                "embedding": np.array([1, 2, 3]),
            },
            {"set_col1": ["a3"], "set_col2": [3], "embedding": np.array([1, 2, 3])},
            {"set_col1": ["a1"], "set_col2": [1], "embedding": np.array([1, 2, 3])},
        ]
        df = pd.DataFrame(data)
        set_filter_cat2id_map = knn_model._load_set_filters(df)
        set_filter2tensor_map = knn_model._set_filter2tensor(
            df, set_filter_cat2id_map, device="cpu"
        )
        assert set_filter2tensor_map["set_col1"].shape == (4, 3)
        assert set_filter2tensor_map["set_col2"].shape == (4, 4)

    def test_get_query_category_filter_score(self):
        knn_model = KNNModel(
            item_id_col="", category_filter_columns=["cat_col1", "cat_col2"]
        )
        data = [
            {"cat_col1": "a1", "cat_col2": "b1", "embedding": np.array([1, 2, 3])},
            {"cat_col1": "a2", "cat_col2": "b2", "embedding": np.array([1, 2, 3])},
            {"cat_col1": "a3", "cat_col2": "b2", "embedding": np.array([1, 2, 3])},
            {"cat_col1": "a1", "cat_col2": "b1", "embedding": np.array([1, 2, 3])},
        ]
        df = pd.DataFrame(data)
        cat2id_map = knn_model._load_category_filters(df)
        cat2tensor_map = knn_model._category2tensor(df, cat2id_map, device="cpu")
        batch_query_data = {
            "embedding": np.array([[1, 2, 3], [4, 5, 6]]),
            "cat_col1": ["a1", "a4"],
            "cat_col2": ["b1", "b2"],
            "top_k": 10,
        }
        scores, num_filters = knn_model._get_query_category_filter_score(
            batch_query_data, cat2id_map, cat2tensor_map
        )
        assert num_filters == 2
        assert scores.shape == (2, 4)

    def test_get_query_set_filter_score(self):
        knn_model = KNNModel(
            item_id_col="",
            set_filter_columns=["set_col1", "set_col2"],
            set_filter_max_sizes={"set_col1": 3, "set_col2": 4},
        )
        data = [
            {
                "set_col1": ["a1", "a2"],
                "set_col2": [1, 2],
                "embedding": np.array([1, 2, 3]),
            },
            {
                "set_col1": ["a2"],
                "set_col2": [2, 3, 4],
                "embedding": np.array([1, 2, 3]),
            },
            {"set_col1": ["a3"], "set_col2": [3], "embedding": np.array([1, 2, 3])},
            {"set_col1": ["a1"], "set_col2": [1], "embedding": np.array([1, 2, 3])},
        ]
        df = pd.DataFrame(data)
        set_filter_cat2id_map = knn_model._load_set_filters(df)
        set_filter2tensor_map = knn_model._set_filter2tensor(
            df, set_filter_cat2id_map, device="cpu"
        )
        batch_query_data = {
            "embedding": np.array([[1, 2, 3], [4, 5, 6]]),
            "set_col1": ["a1", "a4"],
            "set_col2": [3, 3],
            "top_k": 10,
        }
        scores, num_filters = knn_model._get_query_set_filter_score(
            batch_query_data, set_filter_cat2id_map, set_filter2tensor_map
        )
        assert num_filters == 2
        assert scores.shape == (2, 4)

    def test_get_numeric_filter_score(self):
        knn_model = KNNModel(item_id_col="", numeric_filter_columns=["num_col1"])
        data = [
            {"num_col1": 3.5, "embedding": np.array([1, 2, 3])},
            {"num_col1": 6.8, "embedding": np.array([1, 2, 3])},
            {"num_col1": 9.6, "embedding": np.array([1, 2, 3])},
            {"num_col1": 1.5, "embedding": np.array([1, 2, 3])},
        ]
        df = pd.DataFrame(data)
        numeric2tensor_map = knn_model._numeric2tensor(df, device="cpu")
        batch_query_data = {
            "embedding": np.array([[1, 2, 3], [4, 5, 6]]),
            "numeric_lower_bound_filters": {"num_col1": [5.6, 4.0]},
            "numeric_upper_bound_filters": {"num_col1": [9.0, 7.0]},
            "top_k": 10,
        }
        scores, num_filters = knn_model._get_numeric_filter_score(
            batch_query_data, numeric2tensor_map
        )
        assert num_filters == 2
        assert scores.shape == (2, 4)

        batch_query_data = {
            "embedding": np.array([[1, 2, 3], [4, 5, 6]]),
            "numeric_upper_bound_filters": {"num_col1": [9.0, 7.0]},
            "top_k": 10,
        }
        scores, num_filters = knn_model._get_numeric_filter_score(
            batch_query_data, numeric2tensor_map
        )
        assert num_filters == 1
        assert scores.shape == (2, 4)

    def test_load_model_by_partitions(self):
        knn_model = KNNModel(
            item_id_col="item_id",
            numeric_filter_columns=["num_col1"],
            category_filter_columns=["cat_col1"],
            set_filter_columns=["set_col1"],
            set_filter_max_sizes={"set_col1": 3, "set_col2": 4},
        )
        data = [
            {
                "num_col1": 3.5,
                "embedding": np.array([1, 2, 3]),
                "item_id": 1,
                "cat_col1": "a1",
                "set_col1": ["a1"],
            },
            {
                "num_col1": 6.8,
                "embedding": np.array([1, 2, 3]),
                "item_id": 2,
                "cat_col1": "a2",
                "set_col1": ["a2"],
            },
            {
                "num_col1": 9.6,
                "embedding": np.array([1, 2, 3]),
                "item_id": 3,
                "cat_col1": "a3",
                "set_col1": ["a1", "a2"],
            },
            {
                "num_col1": 1.5,
                "embedding": np.array([1, 2, 3]),
                "item_id": 4,
                "cat_col1": "a4",
                "set_col1": ["a2"],
            },
        ]
        df = pd.DataFrame(data)
        os.system("mkdir -p /tmp/item_embeddings")
        os.system("mkdir -p /tmp/item_embeddings_bad")
        df.to_parquet(
            "/tmp/item_embeddings/0.parquet",
            engine="auto",
            compression="snappy",
            index=None,
        )
        knn_model.load_model_by_partitions(
            local_item_data_path="/tmp/item_embeddings", n_partitions=2, device="cpu"
        )

        df.to_parquet(
            "/tmp/item_embeddings_bad/0.c000",
            engine="auto",
            compression="snappy",
            index=None,
        )
        knn_model.load_model_by_partitions(
            local_item_data_path="/tmp/item_embeddings_bad",
            n_partitions=2,
            device="cpu",
        )
        assert knn_model.numeric2tensor_map["num_col1"].shape == (4,)

    def test_load_model(self):
        knn_model = KNNModel(
            item_id_col="item_id",
            numeric_filter_columns=["num_col1"],
            category_filter_columns=["cat_col1"],
            set_filter_columns=["set_col1"],
            set_filter_max_sizes={"set_col1": 3, "set_col2": 4},
        )
        data = [
            {
                "num_col1": 3.5,
                "embedding": np.array([1, 2, 3]),
                "item_id": 1,
                "cat_col1": "a1",
                "set_col1": ["a1"],
            },
            {
                "num_col1": 6.8,
                "embedding": np.array([1, 2, 3]),
                "item_id": 2,
                "cat_col1": "a2",
                "set_col1": ["a2"],
            },
            {
                "num_col1": 9.6,
                "embedding": np.array([1, 2, 3]),
                "item_id": 3,
                "cat_col1": "a3",
                "set_col1": ["a1", "a2"],
            },
            {
                "num_col1": 1.5,
                "embedding": np.array([1, 2, 3]),
                "item_id": 4,
                "cat_col1": "a4",
                "set_col1": ["a2"],
            },
        ]
        df = pd.DataFrame(data)
        os.system("mkdir -p /tmp/item_embeddings")
        df.to_parquet(
            "/tmp/item_embeddings/0.parquet",
            engine="auto",
            compression="snappy",
            index=None,
        )
        knn_model.load_model(
            local_item_data_path="/tmp/item_embeddings",
            load_by_partitions=True,
            n_partitions=2,
            device="cpu",
        )

    def test_predict_batch_from_single_gpu_with_filter(self):
        knn_model = KNNModel(
            item_id_col="item_id",
            numeric_filter_columns=["num_col1", "num_col2"],
            default_ann_max_filter_size=3,
        )
        emb1 = np.array([1, 2, 3], dtype=np.float32) / np.linalg.norm(
            np.array([1, 2, 3], dtype=np.float32)
        )
        emb2 = np.array([4, 5, 6], dtype=np.float32) / np.linalg.norm(
            np.array([4, 5, 6], dtype=np.float32)
        )
        data = [
            {"num_col1": 3.5, "num_col2": 3.5, "embedding": emb1, "item_id": 1},
            {"num_col1": 6.8, "num_col2": 8.5, "embedding": emb1, "item_id": 2},
            {"num_col1": 9.6, "num_col3": 4.5, "embedding": emb2, "item_id": 3},
            {"num_col1": 1.5, "num_col2": 9.5, "embedding": emb2, "item_id": 4},
        ]
        df = pd.DataFrame(data)
        query_data = {
            "embedding": [emb1, emb2],
            "numeric_lower_bound_filters": {
                "num_col1": [5.6, 4.0],
                "num_col2": [1.6, 4.0],
            },
            "numeric_upper_bound_filters": {
                "num_col1": [9.0, 7.0],
                "num_col2": [4.0, 7.0],
            },
            "top_k": 2,
        }
        os.system("mkdir -p /tmp/item_embeddings")
        df.to_parquet(
            "/tmp/item_embeddings/0.parquet",
            engine="auto",
            compression="snappy",
            index=None,
        )
        knn_model.load_model(
            local_item_data_path="/tmp/item_embeddings", n_partitions=1, device="cpu"
        )
        scores, _, labels_tensor = knn_model._predict_batch_from_single_gpu(
            query_data,
            knn_model.item_emb,
            knn_model.labels_tensor,
            knn_model.cat2id_map,
            knn_model.cat2tensor_map,
            knn_model.numeric2tensor_map,
            knn_model.set_filter_cat2id_map,
            knn_model.set_filter2tensor_map,
        )
        assert scores.shape == (2, 2)
        assert labels_tensor.shape == (2, knn_model.default_ann_max_filter_size)

    def test_predict_batch_from_single_gpu_without_filter(self):
        knn_model = KNNModel(item_id_col="item_id", default_ann_max_filter_size=3)
        emb1 = np.array([1, 2, 3], dtype=np.float32) / np.linalg.norm(
            np.array([1, 2, 3], dtype=np.float32)
        )
        emb2 = np.array([4, 5, 6], dtype=np.float32) / np.linalg.norm(
            np.array([4, 5, 6], dtype=np.float32)
        )
        data = [
            {"num_col1": 3.5, "num_col2": 3.5, "embedding": emb1, "item_id": 1},
            {"num_col1": 6.8, "num_col2": 8.5, "embedding": emb1, "item_id": 2},
            {"num_col1": 9.6, "num_col3": 4.5, "embedding": emb2, "item_id": 3},
            {"num_col1": 1.5, "num_col2": 9.5, "embedding": emb2, "item_id": 4},
        ]
        df = pd.DataFrame(data)
        query_data = {
            "embedding": [emb1, emb2],
            "top_k": 2,
        }
        os.system("mkdir -p /tmp/item_embeddings")
        df.to_parquet(
            "/tmp/item_embeddings/0.parquet",
            engine="auto",
            compression="snappy",
            index=None,
        )
        knn_model.load_model(
            local_item_data_path="/tmp/item_embeddings", n_partitions=1, device="cpu"
        )
        scores, _, labels_tensor = knn_model._predict_batch_from_single_gpu(
            query_data,
            knn_model.item_emb,
            knn_model.labels_tensor,
            knn_model.cat2id_map,
            knn_model.cat2tensor_map,
            knn_model.numeric2tensor_map,
            knn_model.set_filter_cat2id_map,
            knn_model.set_filter2tensor_map,
        )
        assert scores.shape == (2, 2)
        assert labels_tensor.shape == (2, 4)

        res = knn_model.predict_batch(query_data)
        assert len(res) == 2


class MultiGPUKNNModelTest(unittest.TestCase):
    def test_load_model(self):
        emb1 = np.array([1, 2, 3], dtype=np.float32) / np.linalg.norm(
            np.array([1, 2, 3], dtype=np.float32)
        )
        emb2 = np.array([4, 5, 6], dtype=np.float32) / np.linalg.norm(
            np.array([4, 5, 6], dtype=np.float32)
        )
        data = [
            {"num_col1": 3.5, "num_col2": 3.5, "embedding": emb1, "item_id": 1},
            {"num_col1": 6.8, "num_col2": 8.5, "embedding": emb1, "item_id": 2},
            {"num_col1": 9.6, "num_col3": 4.5, "embedding": emb2, "item_id": 3},
            {"num_col1": 1.5, "num_col2": 9.5, "embedding": emb2, "item_id": 4},
            {"num_col1": 3.5, "num_col2": 3.5, "embedding": emb1, "item_id": 5},
            {"num_col1": 6.8, "num_col2": 8.5, "embedding": emb1, "item_id": 6},
            {"num_col1": 9.6, "num_col3": 4.5, "embedding": emb2, "item_id": 7},
            {"num_col1": 1.5, "num_col2": 9.5, "embedding": emb2, "item_id": 8},
        ]
        df = pd.DataFrame(data)
        os.system("mkdir -p /tmp/item_embeddings")
        df.to_parquet(
            "/tmp/item_embeddings/0.parquet",
            engine="auto",
            compression="snappy",
            index=None,
        )
        knn_model = MultiGPUKNNModel(
            item_id_col="item_id", default_ann_max_filter_size=3
        )
        knn_model.gpus_per_worker = 1
        knn_model.load_model("/tmp/item_embeddings")

        query_data = {
            "embedding": [emb1, emb2],
            "top_k": 2,
        }
        res1 = knn_model.predict_batch(query_data)
        res2 = knn_model.predict_batch_mt(query_data)
        assert len(res1) == 2
        assert len(res2) == 2


class MultiGPUKNNFFNModelTest(unittest.TestCase):
    # TODO: currently we skip all unit test because we download data from TB
    # Ideally, we should develop mock TB download with fake model using example:
    def test_similarity_score_v1(self):
        class FFNClass(nn.Module):
            def __init__(self):
                super().__init__()
                input_dim = 6
                self.ffn_model = nn.Sequential(
                    nn.Linear(input_dim, 4 * input_dim),
                    nn.ReLU(),
                    nn.Linear(4 * input_dim, input_dim),
                    nn.Dropout(0.1),
                )
                self.projection = nn.Linear(input_dim, 1)

            def forward(self, inputs):
                """Implements forward pass for task specific head."""
                # Expecting inputs: [batch_size, input_dim]
                out = inputs + self.ffn_model(inputs)
                out = self.projection(out)
                return out

        pretrained_script_model = torch.jit.script(FFNClass())
        pretrained_script_model.eval()

        model = MultiGPUKNNFFNModel("")
        query_embedding = torch.tensor([[1.1, 1.2, 1.3], [2.1, 2.2, 2.3]])
        item_embedding = torch.tensor(
            [[31.1, 32.2, 33.3], [41.1, 42.2, 43.3], [51.1, 52.2, 53.3]]
        )
        # test no ANN filter
        relevance_score = model._get_relevance_scores_v1(
            pretrained_script_model, query_embedding, item_embedding
        )
        res = []
        for i in range(query_embedding.shape[0]):
            row = []
            for j in range(item_embedding.shape[0]):
                inputs = torch.cat([query_embedding[i], item_embedding[j]], dim=0)
                score = pretrained_script_model(inputs).item()
                row.append(score)
            res.append(row)
        res = torch.tensor(res)
        assert torch.allclose(relevance_score, res)

        # test with category ANN filter
        _, batch_indices = torch.topk(
            torch.tensor([[1.0, 0.78, 0.6], [1.0, 0.78, 0.6]]), 1
        )
        filtered_item_emb = item_embedding[batch_indices, :]
        query_embed_extended = query_embedding.unsqueeze(dim=1)
        relevance_score = model._get_relevance_scores_v1(
            pretrained_script_model, query_embed_extended, filtered_item_emb, True
        )
        res = []
        for i in range(query_embedding.shape[0]):
            row = []
            for j in batch_indices[i]:
                inputs = torch.cat(
                    [query_embed_extended[i][0], filtered_item_emb[i][j]], dim=0
                )
                score = pretrained_script_model(inputs).item()
                row.append(score)
            res.append(row)
        res = torch.tensor(res)
        assert torch.allclose(relevance_score, res)

    def test_similarity_score_v2(self):
        """Test similarity score calculation v2 with deterministic neural network."""
        # Fix non-deterministic behavior by setting random seed
        torch.manual_seed(42)

        class FFNClass(nn.Module):
            def __init__(self):
                super().__init__()
                input_dim = 6
                self.ffn_model = nn.Sequential(
                    nn.Linear(input_dim, 4 * input_dim),
                    nn.ReLU(),
                    nn.Linear(4 * input_dim, input_dim),
                    nn.Dropout(0.1),
                )
                self.projection = nn.Linear(input_dim, 1)

            def forward(self, query_embedding, item_embedding):
                """Implements forward pass for task specific head."""
                # Expecting inputs: [batch_size, input_dim]
                inputs = torch.cat([query_embedding, item_embedding], dim=-1)
                out = inputs + self.ffn_model(inputs)
                out = self.projection(out)
                return out

        pretrained_script_model = torch.jit.script(FFNClass())
        pretrained_script_model.eval()
        model = MultiGPUKNNFFNModel("")
        query_embedding = torch.tensor([[1.1, 1.2, 1.3], [2.1, 2.2, 2.3]])
        item_embedding = torch.tensor(
            [[31.1, 32.2, 33.3], [41.1, 42.2, 43.3], [51.1, 52.2, 53.3]]
        )
        relevance_score = model._get_relevance_scores_v2(
            pretrained_script_model, query_embedding, item_embedding
        )

        res = []
        for i in range(query_embedding.shape[0]):
            row = []
            for j in range(item_embedding.shape[0]):
                score = pretrained_script_model(
                    query_embedding[i], item_embedding[j]
                ).item()
                row.append(score)
            res.append(row)
        res = torch.tensor(res)
        assert torch.allclose(relevance_score, res)

        # test with category ANN filter
        _, batch_indices = torch.topk(
            torch.tensor([[1.0, 0.78, 0.6], [1.0, 0.78, 0.6]]), 1
        )
        filtered_item_emb = item_embedding[batch_indices, :]
        query_embed_extended = query_embedding.unsqueeze(dim=1)
        relevance_score = model._get_relevance_scores_v2(
            pretrained_script_model, query_embed_extended, filtered_item_emb, True
        )
        res = []
        for i in range(query_embedding.shape[0]):
            row = []
            for j in batch_indices[i]:
                score = pretrained_script_model(
                    query_embed_extended[i][0], filtered_item_emb[i][j]
                ).item()
                row.append(score)
            res.append(row)
        res = torch.tensor(res)
        assert torch.allclose(relevance_score, res)
