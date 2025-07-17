import { FunctionInterpolation } from '../function-interpolation';
import { interpolate } from '../interpolate';
import { StringInterpolation } from '../string-interpolation';

describe('interpolate factory', () => {
  test('creates StringInterpolation for string input', () => {
    const result = interpolate('Hello ${user.name}');
    expect(result).toBeInstanceOf(StringInterpolation);
  });

  test('creates FunctionInterpolation for function input', () => {
    const fn = ({ data }: { data: { user: { name: string } } }) => `Hello ${data.user.name}`;
    const result = interpolate(fn);
    expect(result).toBeInstanceOf(FunctionInterpolation);
  });

  test('preserves string value in StringInterpolation', () => {
    const template = 'Hello ${user.name}';
    const result = interpolate(template);
    expect((result as StringInterpolation).interpolator).toBe(template);
  });

  test('preserves function reference in FunctionInterpolation', () => {
    const fn = ({ data }: { data: { user: { name: string } } }) => `Hello ${data.user.name}`;
    const result = interpolate(fn) as unknown as FunctionInterpolation<string>;
    expect(result.interpolator).toBe(fn);
  });

  test('works with empty string', () => {
    const result = interpolate('');
    expect(result).toBeInstanceOf(StringInterpolation);
    expect((result as StringInterpolation).interpolator).toBe('');
  });
});
