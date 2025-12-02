"""Content-based features for Amazon Books recommendation system.

Computes features based on book content and metadata for enhanced recommendations
"""

from ai.chronon.api.ttypes import EventSource, Source
from ai.chronon.group_by import (
    Accuracy,
    Aggregation,
    GroupBy,
    Operation,
    TimeUnit,
    Window,
)
from ai.chronon.query import Query, select
from ai.chronon.utils import get_staging_query_output_table_name

from examples.amazon_books_qwen.data.staging_queries.amazon_books.books_reviews import (
    base_table,
)

# Source for author-level features
author_performance_source = Source(
    events=EventSource(
        table=get_staging_query_output_table_name(base_table),
        query=Query(
            selects=select("book_authors", "review_score", "book_id"), time_column="ts"
        ),
    )
)

# Author performance metrics
author_features = GroupBy(
    sources=[author_performance_source],
    keys=["book_authors"],
    aggregations=[
        # Number of books getting reviews
        Aggregation(
            input_column="book_id",
            operation=Operation.UNIQUE_COUNT,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
                Window(length=365, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Average rating for author's books
        Aggregation(
            input_column="review_score",
            operation=Operation.AVERAGE,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
                Window(length=365, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Total reviews for author's books (popularity)
        Aggregation(
            input_column="review_score",
            operation=Operation.COUNT,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
                Window(length=365, timeUnit=TimeUnit.DAYS),
            ],
        ),
    ],
    accuracy=Accuracy.TEMPORAL,
)

# Source for category/genre performance
category_performance_source = Source(
    events=EventSource(
        table=get_staging_query_output_table_name(base_table),
        query=Query(
            selects=select("book_categories", "review_score", "book_id"),
            time_column="ts",
        ),
    )
)

# Category/Genre trend features
category_features = GroupBy(
    sources=[category_performance_source],
    keys=["book_categories"],
    aggregations=[
        # Books per category getting reviews
        Aggregation(
            input_column="book_id",
            operation=Operation.UNIQUE_COUNT,
            windows=[
                Window(length=7, timeUnit=TimeUnit.DAYS),
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Average rating for category
        Aggregation(
            input_column="review_score",
            operation=Operation.AVERAGE,
            windows=[
                Window(length=7, timeUnit=TimeUnit.DAYS),
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Category engagement (total reviews)
        Aggregation(
            input_column="review_score",
            operation=Operation.COUNT,
            windows=[
                Window(length=7, timeUnit=TimeUnit.DAYS),
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Category rating variance (controversial vs unanimous)
        Aggregation(
            input_column="review_score",
            operation=Operation.VARIANCE,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
            ],
        ),
    ],
    accuracy=Accuracy.TEMPORAL,
)

# Source for publisher performance
publisher_performance_source = Source(
    events=EventSource(
        table=get_staging_query_output_table_name(base_table),
        query=Query(
            selects=select("book_publisher", "review_score", "book_id"),
            time_column="ts",
        ),
    )
)

# Publisher performance features
publisher_features = GroupBy(
    sources=[publisher_performance_source],
    keys=["book_publisher"],
    aggregations=[
        # Books published getting reviews
        Aggregation(
            input_column="book_id",
            operation=Operation.UNIQUE_COUNT,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
                Window(length=365, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Average rating for publisher's books
        Aggregation(
            input_column="review_score",
            operation=Operation.AVERAGE,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
                Window(length=365, timeUnit=TimeUnit.DAYS),
            ],
        ),
        # Publisher popularity (total reviews)
        Aggregation(
            input_column="review_score",
            operation=Operation.COUNT,
            windows=[
                Window(length=30, timeUnit=TimeUnit.DAYS),
                Window(length=90, timeUnit=TimeUnit.DAYS),
                Window(length=365, timeUnit=TimeUnit.DAYS),
            ],
        ),
    ],
    accuracy=Accuracy.TEMPORAL,
)
