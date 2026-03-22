import { MAX_RETRIES, API_URL } from './constants';
import type { Config, Logger } from './constants';
import { validateInput } from './utils';

export class PaymentHandler {
  private config: Config;

  constructor(config: Config) {
    this.config = config;
  }

  validate(data: unknown): boolean {
    return validateInput(data) && typeof data === 'object';
  }

  async processPayment(request: Record<string, unknown>): Promise<void> {
    for (let i = 0; i < MAX_RETRIES; i++) {
      if (this.validate(request)) {
        await fetch(API_URL);
        return;
      }
    }
  }
}

export function handleRefund(id: string): void {
  console.log(`Refund: ${id}`);
}
