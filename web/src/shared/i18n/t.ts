import { type MessageKey, messages } from "./messages";

type PartialCatalog = Readonly<Partial<Record<MessageKey, string>>>;
const catalogs: Readonly<Record<string, PartialCatalog>> = messages;

export type Translator = (key: MessageKey) => string;

/**
 * Create a translator. Admin overrides (site.copy) win over built-in
 * strings, mirroring site.CopyOverrides.Get on the server:
 * overrides[locale] → overrides[defaultLocale] → overrides.en →
 * built-in[locale] → built-in[defaultLocale] → built-in.en → the key.
 */
export function createT(
  locale: string,
  defaultLocale: string,
  overrides: Readonly<Record<string, PartialCatalog>> = {},
): Translator {
  const chain = [locale, defaultLocale, "en"];
  return (key: MessageKey): string => {
    for (const source of [overrides, catalogs]) {
      for (const l of chain) {
        const v = source[l]?.[key];
        if (v) return v;
      }
    }
    return key;
  };
}

/** Replace `{name}` placeholders; unknown placeholders are left as-is. */
export function fmt(template: string, vars: Readonly<Record<string, string | number>>): string {
  return template.replace(/\{(\w+)\}/g, (whole, name: string) => {
    const v = vars[name];
    return v === undefined ? whole : String(v);
  });
}
