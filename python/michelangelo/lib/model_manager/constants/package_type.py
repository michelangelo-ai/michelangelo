class PackageType:
    """
    Define the model package type constants
    """

    SPARK = "spark"  # legacy MA model with spark pipeline model
    TRITON = "triton"  # triton package
    RAW = "raw"  # raw package type means the model package is unaltered from the raw model
    MOBILE = "mobile"  # mobile package type means the model package is optimized for mobile
