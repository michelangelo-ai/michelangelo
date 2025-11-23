"""Staging query for Amazon Books dataset
Prepares base table by joining books and reviews for feature engineering
"""

from ai.chronon.api.ttypes import MetaData, StagingQuery

# REAL Chronon staging query for Amazon Books dataset
# Uses SQL string format that Chronon can serialize properly
base_table = StagingQuery(
    metaData=MetaData(
        name="amazon_books.books_reviews",
        team="amazon_books",
        description="Base table for Amazon Books feature computation",
    ),
    query="""
        SELECT
            reviews.Id AS book_id,
            reviews.Title AS book_title,
            books.description AS book_description,
            CAST(reviews.`review/score` AS DOUBLE) AS review_score,
            UNIX_TIMESTAMP(to_date(reviews.`review/time`, 'yyyy-MM-dd')) * 1000 AS ts
        FROM amazon_books_reviews reviews
        LEFT JOIN amazon_books_books books ON reviews.Title = books.Title
        WHERE reviews.`review/time` IS NOT NULL
        AND reviews.`review/score` IS NOT NULL
        AND books.Title IS NOT NULL
    """,
)
