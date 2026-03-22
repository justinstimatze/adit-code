export type Config = {
  retries: number;
  timeout: number;
};

export interface Logger {
  log(msg: string): void;
}
