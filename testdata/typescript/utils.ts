export function validateInput(data: unknown): boolean {
  return data !== null && data !== undefined;
}

export function formatOutput(result: unknown): string {
  return JSON.stringify(result);
}

export const HELPER_CONST = 42;
