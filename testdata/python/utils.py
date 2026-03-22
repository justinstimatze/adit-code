def validate_input(data):
    if not isinstance(data, dict):
        return False
    return "type" in data


def format_output(result):
    return str(result)


def _validate(raw):
    return raw is not None
