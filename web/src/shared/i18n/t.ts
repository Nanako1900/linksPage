import { type MessageKey, messages } from "./messages";

type PartialCatalog = Readonly<Partial<Record<MessageKey, string>>>;
const catalogs: Readonly<Record<string, PartialCatalog>> = messages;

export type Translator = (key: MessageKey) => string;

/**
 * Create a translator with the fallback chain locale → defaultLocale → en.
 * `overrides` (admin-provided UI copy, M2b) win over built-in strings.
 */
export function createT(
  locale: string,
  defaultLocale: string,
  overrides: Readonly<Record<string, PartialCatalog>> = {},
): Translator {
  const chain = [locale, defaultLocale, "en"];
  return (key: MessageKey): string => {
    for (const l of chain) {
      const v = overrides[l]?.[key] || catalogs[l]?.[key];
      if (v) return v;
    }
    return key;
  };
}
