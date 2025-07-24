from michelangelo.uniflow.plugins.spark import SparkTask
import michelangelo.uniflow.core as uniflow
from examples.bert_cola.sparkone import test_spark_query


@uniflow.task(
    config=SparkTask(
    )
)
def sparkone():
    return test_spark_query()
