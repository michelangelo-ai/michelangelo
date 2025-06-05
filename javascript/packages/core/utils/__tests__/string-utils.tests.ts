import { capitalizeFirstLetter, isAbsoluteURL } from '../string-utils';

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
