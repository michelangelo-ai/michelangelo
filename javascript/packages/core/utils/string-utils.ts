import startCase from 'lodash/startCase';
import { isURL } from 'validator';

export const capitalizeFirstLetter = (str: string): string =>
  str.charAt(0).toUpperCase() + str.slice(1);

/**
 * Acronyms that should stay fully uppercase when title-cased, rather than
 * being rendered as e.g. "Ai" or "Llm".
 */
const ACRONYMS = new Set(['AI', 'LLM', 'MES']);

/**
 * @description
 * Converts a lower case, space/hyphen-delimited entity or category name into
 * Title Case for display, preserving known acronyms (e.g. AI, LLM, MES).
 *
 * @example
 * ```ts
 * toTitleCase('trained models'); // 'Trained Models'
 * toTitleCase('llm evaluations'); // 'LLM Evaluations'
 * ```
 */
export const toTitleCase = (value: string): string =>
  startCase(value)
    .split(' ')
    .map((word) => (ACRONYMS.has(word.toUpperCase()) ? word.toUpperCase() : word))
    .join(' ');

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
