"""
Create a sample Amazon Books dataset for testing the pipeline
"""

import pandas as pd
import zipfile
import tempfile
import os

def create_sample_amazon_dataset():
    """Create a sample dataset that matches the Amazon Books format"""

    print("Creating sample Amazon Books dataset...")

    # Sample reviews data (matching expected format)
    sample_reviews = pd.DataFrame({
        'Id': ['B001', 'B002', 'B003', 'B004', 'B005'] * 20,  # 100 total records
        'Title': [
            'The Great Gatsby',
            'To Kill a Mockingbird',
            'Pride and Prejudice',
            'The Catcher in the Rye',
            'Lord of the Flies'
        ] * 20,
        'review/summary': [
            'Classic American literature masterpiece',
            'Powerful story about justice and morality',
            'Romance and social commentary at its finest',
            'Coming-of-age story with deep insights',
            'Dark tale of human nature'
        ] * 20,
        'review/text': [
            'This is a masterpiece of American literature that explores themes of wealth, love, and the American Dream in the Jazz Age.',
            'Harper Lee created an unforgettable story about racial injustice and moral growth in the American South.',
            'Jane Austen crafted a brilliant story of love, class, and society in Regency England with wit and insight.',
            'J.D. Salinger captures the alienation and confusion of adolescence with remarkable authenticity and humor.',
            'William Golding presents a chilling examination of civilization versus savagery when society breaks down.'
        ] * 20,
        'review/score': [5.0, 4.5, 4.8, 4.2, 4.6] * 20,
        'review/helpfulness': ['8/10', '12/15', '25/30', '5/8', '18/22'] * 20,
        'review/time': ['2014-07-15', '2013-11-20', '2014-03-10', '2013-08-05', '2014-01-25'] * 20
    })

    # Sample books metadata
    sample_books = pd.DataFrame({
        'Id': ['B001', 'B002', 'B003', 'B004', 'B005'],
        'Title': [
            'The Great Gatsby',
            'To Kill a Mockingbird',
            'Pride and Prejudice',
            'The Catcher in the Rye',
            'Lord of the Flies'
        ],
        'Description': [
            'F. Scott Fitzgerald\'s masterpiece about the Jazz Age and the decline of the American Dream. Set in the summer of 1922, it tells the story of Jay Gatsby and his obsession with Daisy Buchanan.',
            'A gripping tale of racial injustice in 1930s Alabama, told through the eyes of young Scout Finch. This Pulitzer Prize-winning novel explores themes of morality and justice.',
            'Jane Austen\'s beloved novel about Elizabeth Bennet and her complicated relationship with the proud Mr. Darcy. A witty exploration of love, marriage, and social class.',
            'J.D. Salinger\'s iconic coming-of-age novel following Holden Caulfield through his weekend in New York City. A profound examination of adolescent alienation.',
            'William Golding\'s Nobel Prize-winning novel about a group of British boys stranded on an uninhabited island. A dark allegory of human nature and civilization.'
        ],
        'authors': [
            'F. Scott Fitzgerald',
            'Harper Lee',
            'Jane Austen',
            'J.D. Salinger',
            'William Golding'
        ],
        'categories': [
            'Fiction, Classics, American Literature',
            'Fiction, Classics, Southern Literature',
            'Romance, Classics, Historical Fiction',
            'Fiction, Coming of Age, American Literature',
            'Fiction, Classics, Dystopian'
        ],
        'publisher': [
            'Scribner',
            'J.B. Lippincott & Co.',
            'T. Egerton',
            'Little, Brown and Company',
            'Faber & Faber'
        ],
        'publishedDate': ['1925', '1960', '1813', '1951', '1954']
    })

    # Create temporary directory and ZIP file
    temp_dir = tempfile.mkdtemp()
    zip_path = '/tmp/amazon-books-reviews.zip'

    print(f"Creating dataset ZIP file at: {zip_path}")

    with zipfile.ZipFile(zip_path, 'w') as zipf:
        # Save CSVs to temp files and add to ZIP
        reviews_csv = os.path.join(temp_dir, 'reviews.csv')
        books_csv = os.path.join(temp_dir, 'books.csv')

        sample_reviews.to_csv(reviews_csv, index=False)
        sample_books.to_csv(books_csv, index=False)

        zipf.write(reviews_csv, 'reviews.csv')
        zipf.write(books_csv, 'books.csv')

    print(f"✅ Sample dataset created successfully!")
    print(f"   Reviews: {len(sample_reviews)} records")
    print(f"   Books: {len(sample_books)} unique books")
    print(f"   File size: {os.path.getsize(zip_path)} bytes")
    print(f"   Location: {zip_path}")

    return zip_path

if __name__ == "__main__":
    create_sample_amazon_dataset()
    print("\n🎯 Now you can run the pipeline:")
    print("poetry run python amazon_books_qwen.py")