import { isURL } from 'validator';

export const capitalizeFirstLetter = (str: string): string =>
  str.charAt(0).toUpperCase() + str.slice(1);

/**
 * Checks for absolute URLs, including localhost values without a public TLD.
 */
export const isAbsoluteUrl = (value: string) =>
  isURL(encodeURI(value), { require_protocol: true, require_tld: false });

/**
 * Checks whether a value is a navigable URL — either an absolute URL or a
 * root-relative path. Use this to decide whether to render a value as a link.
 *
 * Distinct from {@link isAbsoluteUrl}, which rejects relative paths and is
 * appropriate for validation (e.g., requiring a fully-qualified URL in a form
 * field). This function is appropriate for display logic where internal
 * navigation paths like `/{namespace}/models/foo` are valid link targets.
 */
export const isNavigableUrl = (value: string) => isAbsoluteUrl(value) || value.startsWith('/');

/**
 * @description
 * Transforms a string value into a sentence case format. Special handling for
 * enum values is provided to strip enum value prefixes.
 *
 * @remarks
 * Enum values are the string values associated with a particular enum fields.
 * For instance, PipelineStateValues is a Unified API enum with values like
 * PIPELINE_STATE_BUILDING, PIPELINE_STATE_ERROR, etc. This function can be used
 * to translate these values to Building and Error respectively.
 *
 * @param enumValue - The value to translate
 * @param enumValuePrefix - The prefix to remove from the value
 * @returns The translated value
 *
 * @example
 * ```ts
 * sentenceCaseEnumValue('PIPELINE_STATE_BUILDING', 'PIPELINE_STATE_'); // 'Building'
 * sentenceCaseEnumValue('PIPELINE_STATE_MULTIPLE_ERRORS', 'PIPELINE_STATE_'); // 'Multiple errors'
 * sentenceCaseEnumValue('SOME_OTHER_ENUM_TYPE_VALUE', 'SOME_OTHER_ENUM_TYPE_'); // 'Value'
 * ```
 */
export const sentenceCaseEnumValue = (
  enumValue: string,
  enumValuePrefix: string | RegExp = ''
): string => {
  if (!(typeof enumValue === 'string')) {
    return enumValue;
  }

  if (!(typeof enumValuePrefix === 'string') && !(enumValuePrefix instanceof RegExp)) {
    return enumValue;
  }

  let enumPrefixRegExp: RegExp;
  if (typeof enumValuePrefix === 'string') {
    enumPrefixRegExp = new RegExp(`^${enumValuePrefix}`);
  } else {
    // Support the caller explicitly providing match start character and caller
    // omitting the match start character.
    enumPrefixRegExp = enumValuePrefix.source.startsWith('^')
      ? enumValuePrefix
      : new RegExp(`^${enumValuePrefix.source}`);
  }

  return capitalizeFirstLetter(
    enumValue.replace(enumPrefixRegExp, '').replace(/_/g, ' ').toLowerCase()
  );
};

/**
 * @description
 * Safely convert any value to a string, with JSON.stringify for objects.
 * Handles edge cases like circular references, BigInt, and undefined values.
 *
 * @param value - The value to convert to a string
 * @returns A string representation of the value
 *
 * @example
 * ```ts
 * safeStringify('already a string'); // 'already a string'
 * safeStringify({ code: 500, message: 'Error' }); // '{"code":500,"message":"Error"}'
 * safeStringify([1, 2, 3]); // '[1,2,3]'
 * safeStringify(undefined); // 'undefined'
 * safeStringify(circularRef); // '[object Object]' (fallback)
 * ```
 */
export function safeStringify(value: unknown): string {
  if (typeof value === 'string') {
    return value;
  }

  try {
    return JSON.stringify(value) ?? String(value);
  } catch {
    return String(value);
  }
}

/**
 * Applies nav casing (Title Case) to an entity name, or returns it unchanged.
 * Nav regions are wayfinding chrome that's scanned, not read as a sentence —
 * breadcrumbs, left-nav, tabs, page H1s — per Uber's style guide. Everywhere
 * else, the lowercase canonical config value IS the correct display form, so
 * there's nothing to pass for `casing`.
 *
 * @example
 * ```tsx
 * formatEntityName('trained models', 'nav'); // 'Trained Models'
 * formatEntityName('trained models'); // 'trained models'
 * ```
 */
export function formatEntityName(name: string, casing?: 'nav'): string {
  if (casing !== 'nav') return name;

  // Capitalize the first letter of each space-separated word. Splitting on
  // spaces (not hyphens) keeps hyphenated names like "one-off" intact, and
  // already-uppercase letters are untouched, so acronyms survive.
  return name
    .split(' ')
    .map((word) => (word ? word[0].toUpperCase() + word.slice(1) : word))
    .join(' ');
}
