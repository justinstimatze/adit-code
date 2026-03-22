from .shared_constants import MAX_RETRIES, TIMEOUT, SECRET_SAUCE


def do_work():
    for _ in range(MAX_RETRIES):
        pass
    return TIMEOUT, SECRET_SAUCE
