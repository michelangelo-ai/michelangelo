"""California Housing XGBoost regression example.

Intentionally contains no imports. Importing this package in a Ray task
container would eagerly load SparkTask, which starts a SparkContext at
module level — failing in containers where Java is absent. Keep this file
import-free so Ray and Spark task containers only load what they need.
"""
