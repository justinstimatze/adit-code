export const MAX_RETRIES = 3;
export const API_URL = "https://api.example.com";
export const DEFAULT_TIMEOUT = 30000;

export type Config = {
  retries: number;
  timeout: number;
};

export interface Logger {
  log(message: string): void;
  error(message: string): void;
}
