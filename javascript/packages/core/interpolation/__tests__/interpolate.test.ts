import { interpolate } from '../interpolate';
import { StringInterpolation } from '../string-interpolation';

describe('interpolate factory', () => {
  test('creates StringInterpolation for string input', () => {
    const result = interpolate('Hello ${user.name}');
    expect(result).toBeInstanceOf(StringInterpolation);
  });

  test('preserves string value in StringInterpolation', () => {
    const template = 'Hello ${user.name}';
    const result = interpolate(template);
    expect(result.interpolator).toBe(template);
  });

  test('works with empty string', () => {
    const result = interpolate('');
    expect(result).toBeInstanceOf(StringInterpolation);
    expect(result.interpolator).toBe('');
  });
});
