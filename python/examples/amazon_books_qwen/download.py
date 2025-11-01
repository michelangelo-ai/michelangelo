"""
Amazon Books Dataset Download Task
Production-ready Kaggle dataset download with SparkTask
Downloads data and returns Spark DataFrames directly
"""

import os
from typing import Dict, Any, Optional, Tuple

# Uniflow
import michelangelo.uniflow.core as uniflow
from michelangelo.uniflow.plugins.spark import SparkTask
from michelangelo.sdk.workflow.variables import DatasetVariable

# PySpark
from pyspark.sql import SparkSession
from pyspark.sql.functions import col


@uniflow.task(
    config=SparkTask(
        driver_memory="2g",
        driver_cpu=1,
        executor_memory="2g",
        executor_cpu=1,
        executor_instances=1,
    )
)
def download_kaggle_dataset(
    dataset_config: Dict[str, Any]
) -> Tuple[Optional[DatasetVariable], Optional[DatasetVariable]]:
    """
    Download Amazon Books dataset from Kaggle using SparkTask
    Returns Spark DataFrames (books_df, reviews_df) for downstream tasks
    Fails cleanly if download fails - no fallback logic
    """
    print("📊 Starting Kaggle dataset download with SparkTask...")

    dataset_name = "mohamedbakhet/amazon-books-reviews"
    download_path = "/tmp/amazon_books_dataset"

    # Get Spark session from SparkTask framework
    spark = SparkSession.getActiveSession()
    if spark == None:
        spark = SparkSession.builder.getOrCreate()

    try:
        os.makedirs(download_path, exist_ok=True)

        # Define file paths
        books_file = f"{download_path}/books_data.csv"
        reviews_file = f"{download_path}/Books_rating.csv"

        # Download if files don't exist
        if not (os.path.exists(books_file) and os.path.exists(reviews_file)):
            print("🔑 Authenticating with Kaggle API...")
            from kaggle.api.kaggle_api_extended import KaggleApi
            api = KaggleApi()
            api.authenticate()
            print("✅ Kaggle authentication successful")

            print(f"📥 Downloading {dataset_name} to {download_path}")
            api.dataset_download_files(dataset_name, path=download_path, unzip=True)

            # Verify files were downloaded
            if not (os.path.exists(books_file) and os.path.exists(reviews_file)):
                print(f"❌ Download failed: Expected files not found in {download_path}")
                return None, None
        else:
            print("✅ Dataset already exists, skipping download")

        # Load data into Spark DataFrames
        print("📚 Loading books dataset into Spark...")
        books_df_full = spark.read.csv(books_file, header=True, inferSchema=True)

        print("📝 Loading reviews dataset into Spark...")
        reviews_df_full = spark.read.csv(reviews_file, header=True, inferSchema=True)

        # Sample a subset for testing
        sample_size = dataset_config.get("sample_size", 100)
        books_df = books_df_full.sample(False, sample_size / 10000.0, seed=42).limit(50)

        # Get reviews for sampled books (join on Title since books doesn't have Id)
        book_titles = books_df.select("Title").collect()
        book_title_list = [row.Title for row in book_titles]
        reviews_df = reviews_df_full.filter(col("Title").isin(book_title_list)).limit(500)

        print(f"📊 Successfully loaded {books_df.count()} books and {reviews_df.count()} reviews")

        # Convert to DatasetVariable following boston_housing pattern
        books_dv = DatasetVariable.create(books_df)
        books_dv.save_spark_dataframe()

        reviews_dv = DatasetVariable.create(reviews_df)
        reviews_dv.save_spark_dataframe()

        print("✅ DatasetVariables created and saved as Spark DataFrames")

        return books_dv, reviews_dv

    except Exception as e:
        print(f"❌ Kaggle download/loading failed: {str(e)}")
        print("💡 Check Kaggle API credentials and network connectivity")
        return None, None