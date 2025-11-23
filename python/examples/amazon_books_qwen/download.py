"""
Amazon Books Dataset Download Task
Production-ready Kaggle dataset download with SparkTask
Downloads data and returns Spark DataFrames directly
"""

import os
from typing import Any, Dict, Optional, Tuple

# PySpark
from pyspark.sql import SparkSession
from pyspark.sql.functions import col

# Uniflow
import michelangelo.uniflow.core as uniflow
from michelangelo.sdk.workflow.variables import DatasetVariable
from michelangelo.uniflow.plugins.spark import SparkTask


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
    dataset_config: Dict[str, Any],
) -> Tuple[Optional[DatasetVariable], Optional[DatasetVariable]]:
    """
    Download Amazon Books dataset from Kaggle using SparkTask
    Returns Spark DataFrames (books_df, reviews_df) for downstream tasks
    Fails cleanly if download fails - no fallback logic
    """
    print("📊 Starting Kaggle dataset download with SparkTask...1")

    dataset_name = "mohamedbakhet/amazon-books-reviews"

    # Set environment variables to disable checksum creation BEFORE Spark session starts

    os.environ[
        "SPARK_CONF_spark.hadoop.mapreduce.fileoutputcommitter.marksuccessfuljobs"
    ] = "false"
    os.environ["SPARK_CONF_spark.hadoop.dfs.client.write.checksum"] = "false"
    os.environ["SPARK_CONF_spark.hadoop.dfs.checksum"] = "false"
    os.environ["SPARK_CONF_spark.hadoop.dfs.client.read.checksum"] = "false"
    os.environ["SPARK_CONF_spark.hadoop.fs.file.impl"] = (
        "org.apache.hadoop.fs.RawLocalFileSystem"
    )
    os.environ["SPARK_CONF_spark.hadoop.fs.AbstractFileSystem.file.impl"] = (
        "org.apache.hadoop.fs.local.RawLocalFs"
    )

    # First, try to use local dataset files
    script_dir = os.path.dirname(os.path.abspath(__file__))
    local_dataset_path = os.path.join(script_dir, "datasets")
    download_path = "/tmp/amazon_books_dataset"

    # Get Spark session from SparkTask framework
    spark = SparkSession.getActiveSession()
    if spark is None:
        spark = (
            SparkSession.builder.appName("AmazonBooksDownload")
            .config(
                "spark.hadoop.mapreduce.fileoutputcommitter.marksuccessfuljobs", "false"
            )
            .config("spark.hadoop.dfs.client.write.checksum", "false")
            .config("spark.hadoop.dfs.checksum", "false")
            .config("spark.hadoop.dfs.client.read.checksum", "false")
            .config(
                "spark.hadoop.fs.file.impl", "org.apache.hadoop.fs.RawLocalFileSystem"
            )
            .config(
                "spark.hadoop.fs.AbstractFileSystem.file.impl",
                "org.apache.hadoop.fs.local.RawLocalFs",
            )
            .getOrCreate()
        )

    # Always set the configuration to ensure it's applied
    spark.conf.set(
        "spark.hadoop.mapreduce.fileoutputcommitter.marksuccessfuljobs", "false"
    )
    spark.conf.set("spark.hadoop.dfs.client.write.checksum", "false")
    spark.conf.set("spark.hadoop.dfs.checksum", "false")
    spark.conf.set("spark.hadoop.dfs.client.read.checksum", "false")
    spark.conf.set(
        "spark.hadoop.fs.file.impl", "org.apache.hadoop.fs.RawLocalFileSystem"
    )
    spark.conf.set(
        "spark.hadoop.fs.AbstractFileSystem.file.impl",
        "org.apache.hadoop.fs.local.RawLocalFs",
    )

    # Check for local datasets first
    local_books_file = os.path.join(local_dataset_path, "books_data.csv")
    local_reviews_file = os.path.join(local_dataset_path, "Books_rating.csv")

    # Define download file paths (fallback)
    os.makedirs(download_path, exist_ok=True)
    books_file = f"{download_path}/books_data.csv"
    reviews_file = f"{download_path}/Books_rating.csv"

    # Use local files if available
    if os.path.exists(local_books_file) and os.path.exists(local_reviews_file):
        print("📁 Found local dataset files, using them instead of downloading")
        books_file = local_books_file
        reviews_file = local_reviews_file
        print(
            f"📚 Using local books file: {books_file} ({os.path.getsize(books_file)} bytes)"
        )
        print(
            f"📝 Using local reviews file: {reviews_file} ({os.path.getsize(reviews_file)} bytes)"
        )
    else:
        print("📁 Local dataset files not found, will download from Kaggle")

    # Download if files don't exist
    if not (os.path.exists(books_file) and os.path.exists(reviews_file)):
        print("🔑 Authenticating with Kaggle API...")
        import shutil
        import zipfile

        from kaggle.api.kaggle_api_extended import KaggleApi

        api = KaggleApi()
        api.authenticate()
        print("✅ Kaggle authentication successful")

        # Clean download directory to avoid corrupted files
        if os.path.exists(download_path):
            shutil.rmtree(download_path)
        os.makedirs(download_path, exist_ok=True)

        print(f"📥 Downloading {dataset_name} to {download_path}")

        # Try download with retry logic
        max_retries = 3
        for attempt in range(max_retries):
            try:
                # Download without auto-unzip first
                api.dataset_download_files(
                    dataset_name, path=download_path, unzip=False
                )

                # Find the downloaded zip file
                zip_files = [f for f in os.listdir(download_path) if f.endswith(".zip")]
                if not zip_files:
                    raise Exception("No zip file found after download")

                zip_path = os.path.join(download_path, zip_files[0])
                print(
                    f"🔍 Downloaded zip file: {zip_path} ({os.path.getsize(zip_path)} bytes)"
                )

                # Manually extract the zip file with better error handling
                with zipfile.ZipFile(zip_path, "r") as zip_ref:
                    zip_ref.extractall(download_path)

                # Remove the zip file after extraction
                os.remove(zip_path)
                print("✅ Download and extraction successful")
                break

            except Exception as e:
                print(f"❌ Download attempt {attempt + 1} failed: {str(e)}")
                if attempt < max_retries - 1:
                    print("🔄 Retrying download...")
                    # Clean up any partial downloads
                    if os.path.exists(download_path):
                        shutil.rmtree(download_path)
                    os.makedirs(download_path, exist_ok=True)
                else:
                    print("❌ All download attempts failed")
                    return None, None

        # Verify files were downloaded
        if not (os.path.exists(books_file) and os.path.exists(reviews_file)):
            print(f"❌ Download failed: Expected files not found in {download_path}")
            print(
                f"📂 Available files: {os.listdir(download_path) if os.path.exists(download_path) else 'None'}"
            )
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

    print(
        f"📊 Successfully loaded {books_df.count()} books and {reviews_df.count()} reviews"
    )

    # Convert to DatasetVariable following boston_housing pattern
    books_dv = DatasetVariable.create(books_df)
    books_dv.save_spark_dataframe()

    reviews_dv = DatasetVariable.create(reviews_df)
    reviews_dv.save_spark_dataframe()

    print("✅ DatasetVariables created and saved as Spark DataFrames")

    return books_dv, reviews_dv
