import logging

import datasets
import ray
from typing import Optional
from ray.data import Dataset

from michelangelo.uniflow.plugins.ray import RayTask
import michelangelo.uniflow.core as uniflow

log = logging.getLogger(__name__)


@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="2Gi",
        worker_cpu=1,
        worker_memory="2Gi",
        worker_instances=1,
        # breakpoint=True,
    )
)
def load_data(
    path: str,
    name: str,
    data_slice: str,
    predict_column: str,
    limit: Optional[int] = None,
) -> tuple[Dataset, Dataset, Dataset]:
    hf_dataset = datasets.load_dataset(path=path, name=name)
    hf_dataset = hf_dataset.rename_column(predict_column, "text")
    if limit:
        hf_dataset = hf_dataset[data_slice].select(range(limit))
    return ray.data.from_huggingface(hf_dataset)

@uniflow.task(
    config=RayTask(
        head_cpu=1,
        head_memory="2Gi",
        worker_cpu=1,
        worker_memory="2Gi",
        worker_instances=1,
        # breakpoint=True,
    )
)
def write_data(
    dataset: Dataset,
    out_path: str,
    partitions: Optional[int] = None,
):
    if partitions:
        dataset = dataset.repartition(partitions)
    dataset.write_csv(out_path)
    log.info(f"Wrote {dataset.count()} items to {out_path}")
