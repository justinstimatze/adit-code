from .constants import TRUST_HINTS, MAX_RETRIES
from .utils import validate_input


class PaymentHandler:
    def __init__(self, config):
        self.config = config

    def _validate(self, data):
        return validate_input(data) and data.get("amount", 0) > 0

    def process_payment(self, request):
        if not self._validate(request):
            return None
        hints = TRUST_HINTS.get(request["type"])
        return {"status": "ok", "hints": hints}


class RefundHandler:
    def _validate(self, data):
        return data.get("refund_id") is not None

    def process_refund(self, request):
        for _ in range(MAX_RETRIES):
            if self._validate(request):
                return {"status": "refunded"}
        return {"status": "failed"}
