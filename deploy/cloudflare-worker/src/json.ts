// JSON serialization that is safe inside <script type="application/json">.

const UNSAFE_SCRIPT_CHARS = /[<>&\u2028\u2029]/g;

function escapeChar(ch: string): string {
  return `\\u${ch.charCodeAt(0).toString(16).padStart(4, "0")}`;
}

/**
 * JSON.stringify with "<", ">", "&", U+2028 and U+2029 escaped as \uXXXX,
 * so the text can never close the script element or open a comment
 * (contract 9). The output still parses to the same value.
 */
export function serializeScriptJSON(value: unknown): string {
  const json = JSON.stringify(value);
  if (json === undefined) {
    throw new TypeError("value is not JSON-serializable");
  }
  return json.replace(UNSAFE_SCRIPT_CHARS, escapeChar);
}
