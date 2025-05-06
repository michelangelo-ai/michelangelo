load("@plugin", "cachedoutput")

def test_query():
    match_criterion = {}
    match_criterion["cached_output.label.uniflow_step"] = "boston_housing.train"
    match_criterion["cached_output.label.uniflow_hash"] = "xxx"

    order_by = [
        {
            "field": "metadata.update_timestamp",
            "dir": 2,
        },
    ]
    response = cachedoutput.query(
        namespace = "default",
        match_criterion = match_criterion,
        order_by = order_by,
        lookback_days = 2,
        limit = 1,
    )
    return response
