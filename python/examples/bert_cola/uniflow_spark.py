import michelangelo.uniflow.core as uniflow
from examples.bert_cola.spark_query import test_spark_query
from michelangelo.uniflow.plugins.spark import SparkTask


@uniflow.task(
    config=SparkTask(
    )
)
def spark_query():
    return test_spark_query()
