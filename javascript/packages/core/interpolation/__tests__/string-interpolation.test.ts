import { StringInterpolation } from '../string-interpolation';

describe('StringInterpolation', () => {
  describe('isInterpolation', () => {
    test.each([
      [true, 'Hello ${user.name}', 'string with interpolation in middle'],
      [true, '${page.title}', 'string starting with interpolation'],
      [true, 'Before ${value} after', 'string with interpolation surrounded by text'],
      [false, 'Hello world', 'plain string without interpolation'],
      [false, '$value', 'string with $ but no braces'],
      [false, '{value}', 'string with braces but no $'],
      [false, '', 'empty string'],
    ])('should return %s for %s (%s)', (expected, input) => {
      expect(StringInterpolation.isInterpolation(input)).toBe(expected);
    });
  });

  describe('execute', () => {
    test('resolves simple string interpolations', () => {
      const interpolation = new StringInterpolation('Hello ${data.user.name}');
      const result = interpolation.execute({
        data: { user: { name: 'World' } },
      });
      expect(result).toBe('Hello World');
    });

    test('resolves multiple interpolations in one string', () => {
      const interpolation = new StringInterpolation(
        '${data.user.name} works at ${data.user.company}'
      );
      const result = interpolation.execute({
        data: { user: { name: 'John', company: 'Uber' } },
      });
      expect(result).toBe('John works at Uber');
    });

    test('throws on missing data', () => {
      const interpolation = new StringInterpolation('Hello ${user.name}');
      expect(() => interpolation.execute({})).toThrow(
        'Insufficient data to resolve the string interpolation'
      );
    });

    test('handles empty interpolation values', () => {
      const interpolation = new StringInterpolation('Value: ${data.empty}');
      const result = interpolation.execute({
        data: { empty: '' },
      });
      expect(result).toBe('Value: ');
    });

    test('handles falsy but defined values', () => {
      const interpolation = new StringInterpolation('Count: ${data.count}');
      const result = interpolation.execute({
        data: { count: 0 },
      });
      expect(result).toBe('Count: 0');
    });
  });
});
