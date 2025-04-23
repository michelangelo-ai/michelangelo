import json
from typing import Optional
import pandas as pd
from pathlib import Path
import torch

import logging

logger = logging.getLogger(__name__)


class KNNModel:

    def __init__(
        self,
        item_id_col,
        top_k=100,
        item_embedding_col: str = "embedding",
        query_embedding_col: str = "embedding",
        set_filter_columns: Optional[list[str]] = None,
        set_filter_max_sizes: Optional[dict[str, int]] = None,
        category_filter_columns: Optional[list[str]] = None,
        numeric_filter_columns: Optional[list[str]] = None,
        # `default_ann_max_filter_size` -1 means disable ANN attribute filter
        # suggest value to be less than 100000 otherwise do not set
        default_ann_max_filter_size: int = -1,
    ):
        """
        :param item_id_col: unique identifier of item embedding table such as: store_uuid, item_uuid, hash_item_name
        :param top_k:  how many unique items we want to retrieve
        :param item_embedding_col: the columns name in the item embedding table
        :param query_embedding_col: the columns name in the query embedding table
        :param set_filter_columns: this could be a single store_uuid.
               This is designed for an item appears multiple stores, but we do not want to duplicate this item for many rows.
        :param category_filter_columns: the filter columns for category features. Eg: city_id
        :param numeric_filter_columns: the filter columns for numeric features. Eg: price
        :param default_ann_max_filter_size: how many items after filters we want to retrieve from
        """
        self.item_embedding_col = item_embedding_col
        self.item_id_col = item_id_col
        self.query_embedding_col = query_embedding_col
        self.set_filter_columns = set_filter_columns or []
        self.set_filter_max_sizes = set_filter_max_sizes or {}
        self.category_filter_columns = category_filter_columns or []
        self.numeric_filter_columns = numeric_filter_columns or []
        self.numeric_lower_bound_filter_key = "numeric_lower_bound_filters"
        self.numeric_upper_bound_filter_key = "numeric_upper_bound_filters"
        self.top_k = top_k
        self.default_ann_max_filter_size = default_ann_max_filter_size

    def _load_model_data(self, local_item_data_path: str = "/tmp/item_embeddings"):
        # assume we download all the data to local_item_data_path
        parquet_files = list(Path(local_item_data_path).glob("*.parquet"))
        if len(parquet_files) == 0:
            # sometimes data does not end with `*.parquet` but `.c000` or `-c000`
            parquet_files = list(
                set(
                    list(Path(local_item_data_path).glob("*.c000"))
                    + list(Path(local_item_data_path).glob("*-c000"))
                )
            )
        full_df = pd.concat(
            pd.read_parquet(parquet_file) for parquet_file in parquet_files
        )
        return full_df

    def _load_category_filters(self, df):
        cat2id_map = {}
        for cat_col in self.category_filter_columns:
            values = list(df[cat_col])
            cat2id = {k: i for i, k in enumerate(set(values))}
            cat2id_map[cat_col] = cat2id
        return cat2id_map

    def _load_set_filters(self, df):
        set_filter_cat2id_map = {}
        for set_filter_column in self.set_filter_columns:
            values = [j for sub in list(df[set_filter_column]) for j in sub]
            set_filter_cat2id = {k: i for i, k in enumerate(set(values))}
            set_filter_cat2id_map[set_filter_column] = set_filter_cat2id
        return set_filter_cat2id_map

    def _category2tensor(self, df, cat2id_map, device="cuda:0"):
        cat2tensor_map = {}
        for cat_col in self.category_filter_columns:
            cat2id = cat2id_map[cat_col]
            val_tensor = torch.tensor(
                [cat2id[v] for v in df[cat_col]], dtype=torch.int32
            ).to(device)
            cat2tensor_map[cat_col] = val_tensor
        return cat2tensor_map

    def _set_filter2tensor(self, df, set_filter_cat2id_map, device="cuda:0"):
        set_filter2tensor_map = {}
        for set_filter_column in self.set_filter_columns:
            set_filter_cat2id = set_filter_cat2id_map[set_filter_column]
            filling_value = len(set_filter_cat2id) + 1
            max_size = self.set_filter_max_sizes[set_filter_column]

            def fill_list(ll: list, size, value):
                return ll[:size] + [value] * (size - len(ll[:size]))

            set_filter_tensor = torch.tensor(
                [
                    fill_list(
                        [set_filter_cat2id[vv] for vv in v], max_size, filling_value
                    )
                    for v in df[set_filter_column]
                ],
                dtype=torch.int32,
            ).to(device)
            set_filter2tensor_map[set_filter_column] = set_filter_tensor
        return set_filter2tensor_map

    def _numeric2tensor(self, df, device="cuda:0"):
        numeric2tensor_map = {}
        for num_col in self.numeric_filter_columns:
            val_tensor = torch.tensor(list(df[num_col])).to(device)
            numeric2tensor_map[num_col] = val_tensor
        return numeric2tensor_map

    def _get_query_category_filter_score(
        self, batch_query_data, cat2id_map, cat2tensor_map
    ):
        scores = None
        num_filters = 0
        for cat_col in self.category_filter_columns:
            if cat_col in batch_query_data:
                cat2id = cat2id_map[cat_col]
                cat_doc_tensor = cat2tensor_map[cat_col]
                batch_query_values = batch_query_data[cat_col]
                cat_score = torch.cat(
                    [
                        sum(cat_doc_tensor == cat2id.get(qv, -1) for qv in query_values)
                        .unsqueeze(0)
                        .float()
                        for query_values in batch_query_values
                    ],
                    dim=0,
                )
                if scores is None:
                    scores = cat_score
                else:
                    scores += cat_score
                scores -= 1.0
                num_filters += 1
                del cat_score
        # torch.cuda.empty_cache()
        return scores, num_filters

    def _get_query_set_filter_score(
        self, batch_query_data, set_filter_cat2id_map, set_filter2tensor_map
    ):
        scores = None
        num_filters = 0
        for set_filter_column in self.set_filter_columns:
            if set_filter_column in batch_query_data:
                set_filter_cat2id = set_filter_cat2id_map[set_filter_column]
                set_filter_tensor = set_filter2tensor_map[set_filter_column]
                batch_query_primary_filter = batch_query_data[set_filter_column]
                set_filter_score = torch.cat(
                    [
                        torch.sum(
                            set_filter_tensor == set_filter_cat2id.get(qv, -1), dim=1
                        )
                        .unsqueeze(0)
                        .float()
                        for qv in batch_query_primary_filter
                    ],
                    dim=0,
                )
                if scores is None:
                    scores = set_filter_score
                else:
                    scores += set_filter_score
                scores -= 1.0
                num_filters += 1
                del set_filter_score
        # torch.cuda.empty_cache()
        return scores, num_filters

    def _get_numeric_filter_score(self, batch_query_data, numeric2tensor_map):
        scores = None
        num_filters = 0
        numeric_lower_bound_filter = (
            batch_query_data.get(self.numeric_lower_bound_filter_key) or []
        )
        numeric_upper_bound_filter = (
            batch_query_data.get(self.numeric_upper_bound_filter_key) or []
        )
        for filter_col in self.numeric_filter_columns:
            if filter_col in numeric_lower_bound_filter:
                batch_filter_values = numeric_lower_bound_filter[filter_col]
                numeric2tensor = numeric2tensor_map[filter_col]
                batch_filter_scores = torch.cat(
                    [
                        (numeric2tensor >= qv).unsqueeze(0).float()
                        for qv in batch_filter_values
                    ],
                    dim=0,
                )
                if scores is None:
                    scores = batch_filter_scores
                else:
                    scores += batch_filter_scores
                scores -= 1.0
                num_filters += 1
                del batch_filter_scores, numeric2tensor

            if filter_col in numeric_upper_bound_filter:
                batch_filter_values = numeric_upper_bound_filter[filter_col]
                numeric2tensor = numeric2tensor_map[filter_col]
                batch_filter_scores = torch.cat(
                    [
                        (numeric2tensor <= qv).unsqueeze(0).float()
                        for qv in batch_filter_values
                    ],
                    dim=0,
                )
                if scores is None:
                    scores = batch_filter_scores
                else:
                    scores += batch_filter_scores
                scores -= 1.0
                num_filters += 1
                del batch_filter_scores, numeric2tensor
        # torch.cuda.empty_cache()
        return scores, num_filters

    def load_model_by_partitions(
        self,
        local_item_data_path: str = "/tmp/item_embeddings",
        n_partitions=2,
        device="cuda:0",
    ):
        full_df = self._load_model_data(local_item_data_path)
        self.cat2id_map = self._load_category_filters(full_df)
        self.set_filter_cat2id_map = self._load_set_filters(full_df)

        size_per_partition = len(full_df) // n_partitions
        partitions = [
            [i * size_per_partition, (i + 1) * size_per_partition]
            for i in range(n_partitions - 1)
        ] + [[(n_partitions - 1) * size_per_partition, len(full_df)]]
        labels_tensor_list = []
        item_emb_list = []
        self.cat2tensor_map = {}
        self.numeric2tensor_map = {}
        self.set_filter2tensor_map = {}
        self.labels = []

        for i, indices in enumerate(partitions):
            logger.info(f"Loading partition {i} for data range {indices}")
            s, e = indices
            df = full_df.iloc[s:e]
            embedding_array = df[self.item_embedding_col].apply(pd.Series).to_numpy()
            labels = list(df[self.item_id_col])
            # we keep the indices as label for row filtering on embedding table
            labels_tensor = torch.tensor(list(range(len(labels)))).to(device)
            item_emb = torch.tensor(embedding_array, dtype=torch.float32).to(device)
            labels_tensor_list.append(labels_tensor)
            item_emb_list.append(item_emb)

            cat2tensor_map = self._category2tensor(df, self.cat2id_map, device=device)
            numeric2tensor_map = self._numeric2tensor(df, device)
            set_filter2tensor_map = self._set_filter2tensor(
                df, self.set_filter_cat2id_map, device=device
            )
            for k, v in cat2tensor_map.items():
                if k not in self.cat2tensor_map:
                    self.cat2tensor_map[k] = [v]
                else:
                    self.cat2tensor_map[k].append(v)
            for k, v in numeric2tensor_map.items():
                if k not in self.numeric2tensor_map:
                    self.numeric2tensor_map[k] = [v]
                else:
                    self.numeric2tensor_map[k].append(v)
            for k, v in set_filter2tensor_map.items():
                if k not in self.set_filter2tensor_map:
                    self.set_filter2tensor_map[k] = [v]
                else:
                    self.set_filter2tensor_map[k].append(v)

            self.labels += labels
            del embedding_array, labels, df

        self.labels_tensor = torch.cat(labels_tensor_list, dim=0)
        self.item_emb = torch.cat(item_emb_list, dim=0)

        for v in labels_tensor_list:
            del v
        for v in item_emb_list:
            del v
        torch.cuda.empty_cache()
        del labels_tensor_list, item_emb_list

        for k, v in self.cat2tensor_map.items():
            self.cat2tensor_map[k] = torch.cat(v, dim=0)
            del v
        for k, v in self.numeric2tensor_map.items():
            self.numeric2tensor_map[k] = torch.cat(v, dim=0)
            del v
        for k, v in self.set_filter2tensor_map.items():
            self.set_filter2tensor_map[k] = torch.cat(v, dim=0)
            del v
        del full_df

    def load_model(
        self,
        local_item_data_path: str = "/tmp/item_embeddings",
        load_by_partitions: bool = False,
        device="cuda:0",
        **kwargs,
    ):
        if load_by_partitions:
            return self.load_model_by_partitions(
                local_item_data_path=local_item_data_path, device=device, **kwargs
            )

        full_df = self._load_model_data(local_item_data_path)
        embedding_array = full_df[self.item_embedding_col].apply(pd.Series).to_numpy()

        self.labels = list(full_df[self.item_id_col])
        self.labels_tensor = torch.tensor(list(range(len(self.labels)))).to(device)
        self.item_emb = torch.tensor(embedding_array, dtype=torch.float32).to(device)

        self.cat2id_map = self._load_category_filters(full_df)
        self.cat2tensor_map = self._category2tensor(
            full_df, self.cat2id_map, device=device
        )
        self.numeric2tensor_map = self._numeric2tensor(full_df, device=device)
        self.set_filter_cat2id_map = self._load_set_filters(full_df)
        self.set_filter2tensor_map = self._set_filter2tensor(
            full_df, self.set_filter_cat2id_map, device=device
        )

        del full_df, embedding_array

    def _predict_batch_from_single_gpu(
        self,
        query_data,
        item_emb_per_gpu,
        labels_tensor_per_gpu,
        cat2id_map_per_gpu,
        cat2tensor_map_per_gpu,
        numeric2tensor_map_per_gpu,
        set_filter_cat2id_map_per_gpu,
        set_filter2tensor_map_per_gpu,
    ):
        """
        :param query_data: dict from search request, contains query_embedding, query filter values,
                           other search args such as top k
        :param item_emb_per_gpu: the sharded item embedding on the GPU
        :param labels_tensor_per_gpu: the sharded indices on the GPU for original item embedding table
        :param cat2id_map_per_gpu: category value to index on each GPU
        :param cat2tensor_map_per_gpu: category value as a tensor on each GPU
        :param numeric2tensor_map_per_gpu:
        :param set_filter_cat2id_map_per_gpu:
        :param set_filter2tensor_map_per_gpu:
        :return:
        """
        query_emb = torch.tensor(query_data[self.query_embedding_col]).to(
            item_emb_per_gpu.device
        )

        cat_scores, num_cat_filters = self._get_query_category_filter_score(
            query_data, cat2id_map_per_gpu, cat2tensor_map_per_gpu
        )
        numeric_scores, num_numeric_filters = self._get_numeric_filter_score(
            query_data, numeric2tensor_map_per_gpu
        )

        set_scores, num_set_filter = self._get_query_set_filter_score(
            query_data, set_filter_cat2id_map_per_gpu, set_filter2tensor_map_per_gpu
        )
        zero_scores = torch.zeros(
            query_emb.shape[0],
            item_emb_per_gpu.shape[0],
            device=item_emb_per_gpu.device,
        )
        if cat_scores is None:
            cat_scores = zero_scores
        if numeric_scores is None:
            numeric_scores = zero_scores
        if set_scores is None:
            set_scores = zero_scores

        if (
            num_cat_filters + num_set_filter + num_numeric_filters > 0
            and self.default_ann_max_filter_size > 0
        ):
            filter_scores = cat_scores + numeric_scores + set_scores
            ann_max_filter_size = query_data.get(
                "ann_max_filter_size", self.default_ann_max_filter_size
            )
            _, batch_indices = torch.topk(filter_scores, ann_max_filter_size)
            labels_tensor = labels_tensor_per_gpu[batch_indices]
            filtered_item_emb = item_emb_per_gpu[batch_indices, :]
            query_emb = query_emb.unsqueeze(dim=1)
            # Old code to keep
            # relevance_scores = torch.bmm(query_emb, filtered_item_emb.transpose(1, 2)).squeeze(dim=1)
            relevance_scores = self.get_relevance_scores(
                query_emb, filtered_item_emb, True
            )
            relevance_scores += torch.cat(
                [
                    filter_scores[i, batch_indices[i]].unsqueeze(dim=0)
                    for i in range(len(filter_scores))
                ],
                dim=0,
            )

        else:
            labels_tensor = labels_tensor_per_gpu.unsqueeze(0).repeat(
                query_emb.shape[0], 1
            )
            # Old code to keep
            # relevance_scores = query_emb.matmul(item_emb_per_gpu.transpose(0, 1))
            relevance_scores = self.get_relevance_scores(
                query_emb, item_emb_per_gpu, False
            )
            relevance_scores = (
                relevance_scores + numeric_scores + cat_scores + set_scores
            )

        top_k = min(query_data.get("top_k", self.top_k), len(item_emb_per_gpu))
        scores, indices = torch.topk(relevance_scores, top_k)
        del query_emb, relevance_scores, cat_scores
        # torch.cuda.empty_cache()
        return scores, indices, labels_tensor

    @staticmethod
    def get_relevance_scores(query_emb, item_emb, filtered: bool = False):
        if filtered:
            relevance_scores = torch.bmm(query_emb, item_emb.transpose(1, 2)).squeeze(
                dim=1
            )
        else:
            relevance_scores = query_emb.matmul(item_emb.transpose(0, 1))
        return relevance_scores

    def predict_batch(self, query_data):
        batch_scores, batch_indices, batch_labels_tensor = (
            self._predict_batch_from_single_gpu(
                query_data,
                self.item_emb,
                self.labels_tensor,
                self.cat2id_map,
                self.cat2tensor_map,
                self.numeric2tensor_map,
                self.set_filter_cat2id_map,
                self.set_filter2tensor_map,
            )
        )
        res = []
        for i, indices in enumerate(batch_indices):
            labels_tensor = batch_labels_tensor[i, :]
            items_ids = [labels_tensor[idx].cpu().item() for idx in indices]
            scores = batch_scores[i].cpu().numpy().tolist()
            predictions = [
                {
                    "item_id": self.labels[items_ids[j]],
                    "score": scores[j],
                }
                for j in range(len(scores))
            ]
            queries = {
                k: v[i] if isinstance(v, list) else v
                for k, v in query_data.items()
                if k != self.query_embedding_col
            }
            res.append(
                {**queries, "predictions": json.dumps(predictions)},
            )
        del batch_scores, batch_indices
        # torch.cuda.empty_cache()
        return res


class MultiGPUKNNModel(KNNModel):
    """
    The details of algorithm could be found in
    https://docs.google.com/document/d/1Q-EiIE1i5R4BvotzbwWyeWKT4crc-ts1ndKTgHvI0QM/edit?tab=t.0#bookmark=id.hik6308q1our
    The basic idea is to shard the giant embedding table to multiple GPUs to avoid GPU OOM.
    """

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self.gpus_per_worker = torch.cuda.device_count()

    def load_model(self, local_item_data_path: str = "/tmp/item_embeddings"):
        full_df = self._load_model_data(local_item_data_path)
        size_per_gpu = len(full_df) // self.gpus_per_worker

        partitions = [
            [i * size_per_gpu, (i + 1) * size_per_gpu]
            for i in range(self.gpus_per_worker - 1)
        ] + [[(self.gpus_per_worker - 1) * size_per_gpu, len(full_df)]]

        assert len(partitions) == self.gpus_per_worker

        self.gpu2item_map = []

        for i, indices in enumerate(partitions):
            s, e = indices
            df = full_df.iloc[s:e]
            device = f"cuda:{i}" if torch.cuda.is_available() else "cpu"
            logger.info(
                f"Loading partition {i} for data range {indices} on device {device}"
            )
            embedding_array = df[self.item_embedding_col].apply(pd.Series).to_numpy()
            labels = list(df[self.item_id_col])
            labels_tensor = torch.tensor(list(range(len(labels)))).to(device=device)
            item_emb = torch.tensor(embedding_array, dtype=torch.float32).to(
                device=device
            )

            cat2id_map = self._load_category_filters(df)
            cat2tensor_map = self._category2tensor(df, cat2id_map, device=device)
            numeric2tensor_map = self._numeric2tensor(df, device=device)
            set_filter_cat2id_map = self._load_set_filters(df)
            set_filter2tensor_map = self._set_filter2tensor(
                df, set_filter_cat2id_map, device=device
            )

            self.gpu2item_map.append(
                {
                    "labels": labels,
                    "labels_tensor": labels_tensor,
                    "item_emb": item_emb,
                    "cat2id_map": cat2id_map,
                    "cat2tensor_map": cat2tensor_map,
                    "numeric2tensor_map": numeric2tensor_map,
                    "set_filter_cat2id_map": set_filter_cat2id_map,
                    "set_filter2tensor_map": set_filter2tensor_map,
                }
            )
            del df, embedding_array
        del full_df

    def predict_batch(self, query_data):
        reduced_res = [[] for _ in range(len(query_data[self.query_embedding_col]))]
        for shard in range(self.gpus_per_worker):
            item_emb_per_gpu = self.gpu2item_map[shard]["item_emb"]
            labels_per_gpu = self.gpu2item_map[shard]["labels"]
            labels_tensor_per_gpu = self.gpu2item_map[shard]["labels_tensor"]

            cat2id_map_per_gpu = self.gpu2item_map[shard]["cat2id_map"]
            cat2tensor_map_per_gpu = self.gpu2item_map[shard]["cat2tensor_map"]
            numeric2tensor_map_per_gpu = self.gpu2item_map[shard]["numeric2tensor_map"]
            set_filter_cat2id_map_per_gpu = self.gpu2item_map[shard][
                "set_filter_cat2id_map"
            ]
            set_filter2tensor_map_per_gpu = self.gpu2item_map[shard][
                "set_filter2tensor_map"
            ]

            batch_scores_per_gpu, batch_indices_per_gpu, batch_labels_tensor_per_gpu = (
                self._predict_batch_from_single_gpu(
                    query_data,
                    item_emb_per_gpu,
                    labels_tensor_per_gpu,
                    cat2id_map_per_gpu,
                    cat2tensor_map_per_gpu,
                    numeric2tensor_map_per_gpu,
                    set_filter_cat2id_map_per_gpu,
                    set_filter2tensor_map_per_gpu,
                )
            )

            for i, indices in enumerate(batch_indices_per_gpu):
                labels_tensor = batch_labels_tensor_per_gpu[i, :]
                items = [labels_tensor[idx].cpu().item() for idx in indices]
                scores = batch_scores_per_gpu[i].cpu().numpy().tolist()
                predictions = [
                    {
                        "item_id": labels_per_gpu[items[j]],
                        "score": scores[j],
                        "shard": shard,
                    }
                    for j in range(len(scores))
                ]
                reduced_res[i].extend(predictions)
            del batch_scores_per_gpu, batch_indices_per_gpu, batch_labels_tensor_per_gpu
            # torch.cuda.empty_cache()
        res = []
        for i, preds in enumerate(reduced_res):
            top_k = min(query_data.get("top_k", self.top_k), len(preds))
            topk_pred = sorted(preds, key=lambda x: -1.0 * x["score"])[:top_k]
            queries = {
                k: v[i] if isinstance(v, list) else v
                for k, v in query_data.items()
                if k != self.query_embedding_col
            }
            res.append(
                {**queries, "predictions": json.dumps(topk_pred)},
            )
        return res

    def _process_shard(self, query_data, shard):
        item_emb_per_gpu = self.gpu2item_map[shard]["item_emb"]
        labels_tensor_per_gpu = self.gpu2item_map[shard]["labels_tensor"]

        cat2id_map_per_gpu = self.gpu2item_map[shard]["cat2id_map"]
        cat2tensor_map_per_gpu = self.gpu2item_map[shard]["cat2tensor_map"]
        numeric2tensor_map_per_gpu = self.gpu2item_map[shard]["numeric2tensor_map"]
        set_filter_cat2id_map_per_gpu = self.gpu2item_map[shard][
            "set_filter_cat2id_map"
        ]
        set_filter2tensor_map_per_gpu = self.gpu2item_map[shard][
            "set_filter2tensor_map"
        ]

        batch_scores_per_gpu, batch_indices_per_gpu, batch_labels_tensor_per_gpu = (
            self._predict_batch_from_single_gpu(
                query_data,
                item_emb_per_gpu,
                labels_tensor_per_gpu,
                cat2id_map_per_gpu,
                cat2tensor_map_per_gpu,
                numeric2tensor_map_per_gpu,
                set_filter_cat2id_map_per_gpu,
                set_filter2tensor_map_per_gpu,
            )
        )

        return (
            batch_scores_per_gpu,
            batch_indices_per_gpu,
            batch_labels_tensor_per_gpu,
            shard,
        )

    def predict_batch_mt(self, query_data):
        # multi-threads implementation of predict_batch
        reduced_res = [[] for _ in range(len(query_data[self.query_embedding_col]))]
        from concurrent.futures import ThreadPoolExecutor, as_completed

        with ThreadPoolExecutor(self.gpus_per_worker) as executor:
            futures = [
                executor.submit(self._process_shard, query_data, shard)
                for shard in range(self.gpus_per_worker)
            ]
            for future in as_completed(futures):
                (
                    batch_scores_per_gpu,
                    batch_indices_per_gpu,
                    batch_labels_tensor_per_gpu,
                    shard,
                ) = future.result()
                labels_per_gpu = self.gpu2item_map[shard]["labels"]
                for i, indices in enumerate(batch_indices_per_gpu):
                    labels_tensor = batch_labels_tensor_per_gpu[i, :]
                    items = [labels_tensor[idx].cpu().item() for idx in indices]
                    scores = batch_scores_per_gpu[i].cpu().numpy().tolist()
                    predictions = [
                        {
                            "item_id": labels_per_gpu[items[j]],
                            "score": scores[j],
                            "shard": shard,
                        }
                        for j in range(len(scores))
                    ]
                    reduced_res[i].extend(predictions)
                del (
                    batch_scores_per_gpu,
                    batch_indices_per_gpu,
                    batch_labels_tensor_per_gpu,
                )
                # torch.cuda.empty_cache()
        res = []
        for i, preds in enumerate(reduced_res):
            top_k = min(query_data.get("top_k", self.top_k), len(preds))
            topk_pred = sorted(preds, key=lambda x: -1.0 * x["score"])[:top_k]
            queries = {
                k: v[i] if isinstance(v, list) else v
                for k, v in query_data.items()
                if k != self.query_embedding_col
            }
            res.append(
                {**queries, "predictions": json.dumps(topk_pred)},
            )

        return res


class MultiGPUKNNFFNModel(MultiGPUKNNModel):
    """
    MultiGPUKNNFFNModel is use a small FFN model to compute semantic relevance instead of simple dot product.
    Relevance = f(query_embedding, item_embedding) where f could be arbitrary torch script model.
    Detailed design and discussion is discussed here:
    https://docs.google.com/document/d/13WDFdY4orPX2FOZL4_0YgrL68R4cPpL0SqNt6EoPyq0/edit?tab=t.0
    """

    def __init__(self, *args, pretrained_script_models=None, **kwargs):
        super().__init__(*args, **kwargs)
        self.gpus_per_worker = torch.cuda.device_count()
        self.pretrained_script_models = pretrained_script_models
        logger.info("Use FFN model instead of dot product")

    def get_relevance_scores(self, query_emb, item_emb, filtered: bool = False):
        # for multi-gpu KNN model, we need a copy of torch script model on each GPU
        model_scripted_gpu = self.pretrained_script_models[query_emb.get_device()]
        return self._get_relevance_scores_v1(
            model_scripted_gpu, query_emb, item_emb, filtered
        )

    @staticmethod
    def _get_relevance_scores_v1(
        pretrained_script_model, query_emb, item_emb, filtered: bool = False
    ):
        """
        This function assumes that the input to the pretrained_script_model is concat (query_embedding, item_embedding)

        :param pretrained_script_model:  a pretrained torch script model `f(query_embedding, item_embedding) -> float`
        :param query_emb:
        :param item_emb:
        :param filtered: bool variable to indicate whether the item_emb is already filtered
        :return:
        """
        # logger.info("Use FFN model V1 instead of dot product")
        if filtered:
            num_queries = query_emb.shape[0]
            num_items = item_emb.shape[1]
            assert item_emb.shape[0] == num_queries
            query_emb_expanded = query_emb.expand(-1, num_items, -1)
            item_emb_expanded = item_emb
        else:
            num_queries = query_emb.shape[0]
            num_items = item_emb.shape[0]
            query_emb_expanded = query_emb.unsqueeze(1).expand(-1, num_items, -1)
            item_emb_expanded = item_emb.unsqueeze(0).expand(num_queries, -1, -1)

        embeddings = torch.cat([query_emb_expanded, item_emb_expanded], dim=2)
        embeddings_flat = torch.reshape(
            embeddings, (num_queries * num_items, embeddings.shape[2])
        )
        with torch.no_grad():
            scores_flat = pretrained_script_model(embeddings_flat)
        relevance_scores = torch.reshape(scores_flat, (num_queries, num_items))
        return relevance_scores

    @staticmethod
    def _get_relevance_scores_v2(
        pretrained_script_model, query_emb, item_emb, filtered: bool = False
    ):
        """
        This function assumes that the input to the pretrained_script_model is [query_embedding, item_embedding]
        :param pretrained_script_model:  a pretrained torch script model `f(query_embedding, item_embedding) -> float`
        :param query_emb:
        :param item_emb:
        :param filtered: bool variable to indicate whether the item_emb is already filtered
        :return:
        """
        # logger.info("Use FFN model V2 instead of dot product")
        if filtered:
            num_queries = query_emb.shape[0]
            num_items = item_emb.shape[1]
            assert item_emb.shape[0] == num_queries
            query_emb_expanded = query_emb.expand(-1, num_items, -1)
            item_emb_expanded = item_emb
        else:
            num_queries = query_emb.shape[0]
            num_items = item_emb.shape[0]
            query_emb_expanded = query_emb.unsqueeze(1).expand(-1, num_items, -1)
            item_emb_expanded = item_emb.unsqueeze(0).expand(num_queries, -1, -1)

        query_emb_expanded = torch.reshape(
            query_emb_expanded, (num_queries * num_items, query_emb_expanded.shape[2])
        )
        item_emb_expanded = torch.reshape(
            item_emb_expanded, (num_queries * num_items, item_emb_expanded.shape[2])
        )
        with torch.no_grad():
            scores_flat = pretrained_script_model(query_emb_expanded, item_emb_expanded)
        relevance_scores = torch.reshape(scores_flat, (num_queries, num_items))
        return relevance_scores
