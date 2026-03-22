from .shared_constants import MAX_RETRIES, TIMEOUT, LONELY_FLAG


def other_work():
    for _ in range(MAX_RETRIES):
        pass
    return TIMEOUT, LONELY_FLAG
