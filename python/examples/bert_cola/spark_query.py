from pyspark.sql import SparkSession


def test_spark_query():
    spark = SparkSession.builder.getOrCreate()
    df = spark.sql(f"""
        SELECT *
        FROM aletheia.chimera_platform_cost
        WHERE 
            year=2025
            AND month=1
            AND day=1
        """)
    df.show(1, False)


if __name__ == "__main__":
    test_spark_query()
