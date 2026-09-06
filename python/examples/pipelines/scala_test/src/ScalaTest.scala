import org.apache.spark.sql.SparkSession

/** Minimal Spark job used to smoke-test the ScalaSparkTask uniflow plugin.
  *
  * Sums the integers 1..5 with Spark and asserts the result, so a passing
  * run proves the plugin's fsspec download + spark-submit (local-run) or
  * SparkJob CRD (remote-run) path actually executed real Spark code, not
  * just that the JVM started.
  */
object ScalaTest {
  def main(args: Array[String]): Unit = {
    val spark = SparkSession.builder()
      .appName("scala-plugin-test")
      .getOrCreate()

    val df = spark.range(1, 6).toDF("n")
    val total = df.agg("n" -> "sum").first().getLong(0)

    println(s"ScalaTest: sum = $total")
    if (total != 15L) {
      throw new RuntimeException(s"unexpected sum: $total")
    }

    spark.stop()
    println("ScalaTest: SUCCESS")
  }
}
