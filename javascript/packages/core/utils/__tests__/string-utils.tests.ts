import { capitalizeFirstLetter } from '../string-utils';

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
