import { isURL } from 'validator';

export const capitalizeFirstLetter = (str: string): string =>
  str.charAt(0).toUpperCase() + str.slice(1);

export const isAbsoluteURL = (value: string) => isURL(encodeURI(value), { require_protocol: true });

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
