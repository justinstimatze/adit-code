# Constants used by multiple files — should NOT be flagged as relocatable
MAX_RETRIES = 3
TIMEOUT = 30
API_VERSION = "v2"

# Constant used by only one file — SHOULD be flagged as relocatable
SECRET_SAUCE = "only_consumer_a_uses_this"

# Constant used by only one file — SHOULD be flagged as relocatable
LONELY_FLAG = True
