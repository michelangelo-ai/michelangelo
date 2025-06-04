import { capitalizeFirstLetter, isAbsoluteURL, sentenceCaseEnumValue } from '../string-utils';

describe('capitalizeFirstLetter', () => {
  it('should capitalize the first letter of the string', () => {
    expect(capitalizeFirstLetter('hello')).toBe('Hello');
  });

  it('should return the same string if it is already capitalized', () => {
    expect(capitalizeFirstLetter('Hello')).toBe('Hello');
  });

  it('should return an empty string if the string is empty', () => {
    expect(capitalizeFirstLetter('')).toBe('');
  });
});

describe('isAbsoluteURL', () => {
  it('should return true if the string is a valid absolute URL', () => {
    expect(isAbsoluteURL('https://www.google.com')).toBe(true);
  });

  it('should return false if the string is not a valid absolute URL without a protocol', () => {
    expect(isAbsoluteURL('www.google.com')).toBe(false);
  });

  it('should return true if the string is a valid absolute URL with a protocol', () => {
    expect(isAbsoluteURL('http://www.google.com')).toBe(true);
  });

  it('should return false if the string is not a valid absolute URL', () => {
    expect(isAbsoluteURL('something')).toBe(false);
  });
});

describe('sentenceCaseEnumValue', () => {
  it('should convert enum values to sentence case', () => {
    expect(sentenceCaseEnumValue('PIPELINE_STATE_BUILDING', 'PIPELINE_STATE_')).toBe('Building');
    expect(sentenceCaseEnumValue('PIPELINE_STATE_MULTIPLE_ERRORS', 'PIPELINE_STATE_')).toBe(
      'Multiple errors'
    );
    expect(sentenceCaseEnumValue('SOME_OTHER_ENUM_TYPE_VALUE', 'SOME_OTHER_ENUM_TYPE_')).toBe(
      'Value'
    );
  });

  it('should handle empty prefix', () => {
    expect(sentenceCaseEnumValue('HELLO_WORLD')).toBe('Hello world');
  });

  it('should handle RegExp prefix', () => {
    expect(sentenceCaseEnumValue('PIPELINE_STATE_BUILDING', /^PIPELINE_STATE_/)).toBe('Building');
    expect(sentenceCaseEnumValue('PIPELINE_STATE_BUILDING', new RegExp('^PIPELINE_STATE_'))).toBe(
      'Building'
    );
  });

  it('should handle non-string input', () => {
    // @ts-expect-error - we want to test the function with a non-string input
    expect(sentenceCaseEnumValue(123)).toBe(123);
  });

  it('should handle invalid prefix type', () => {
    // @ts-expect-error - we want to test the function with an invalid prefix type
    expect(sentenceCaseEnumValue('HELLO_WORLD', 123)).toBe('HELLO_WORLD');
  });

  it('should handle empty string input', () => {
    expect(sentenceCaseEnumValue('')).toBe('');
  });

  it('should handle string with no underscores', () => {
    expect(sentenceCaseEnumValue('HELLO', '')).toBe('Hello');
  });
});
