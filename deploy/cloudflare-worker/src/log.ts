// Structured error logging for Workers Logs. Never pass secrets, cookies or
// full visitor headers in fields.

export type LogFields = Readonly<Record<string, string | number | boolean | null>>;

export function errorMessage(err: unknown): string {
  return err instanceof Error ? `${err.name}: ${err.message}` : String(err);
}

export function logError(event: string, fields: LogFields = {}): void {
  console.error(JSON.stringify({ level: "error", event, ...fields }));
}
