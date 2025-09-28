import {
  capitalizeFirstLetter,
  isAbsoluteURL,
  safeStringify,
  sentenceCaseEnumValue,
} from '../string-utils';

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
  it('should conversion enum values to sentence case', () => {
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

describe('safeStringify', () => {
  describe('strings', () => {
    it('should return strings unchanged', () => {
      expect(safeStringify('hello world')).toBe('hello world');
      expect(safeStringify('')).toBe('');
      expect(safeStringify('already a string')).toBe('already a string');
    });
  });

  describe('objects', () => {
    it('should JSON.stringify simple objects', () => {
      expect(safeStringify({ code: 500, message: 'Error' })).toBe('{"code":500,"message":"Error"}');
      expect(safeStringify({ name: 'John', age: 30 })).toBe('{"name":"John","age":30}');
    });

    it('should handle empty objects', () => {
      expect(safeStringify({})).toBe('{}');
    });

    it('should handle nested objects', () => {
      const nested = { user: { name: 'John', profile: { age: 30 } } };
      expect(safeStringify(nested)).toBe('{"user":{"name":"John","profile":{"age":30}}}');
    });

    it('should handle circular references gracefully', () => {
      const circular: Record<string, unknown> = { name: 'test' };
      circular.self = circular;

      // Should fall back to String() without throwing
      expect(safeStringify(circular)).toBe('[object Object]');
    });
  });

  describe('arrays', () => {
    it('should JSON.stringify arrays', () => {
      expect(safeStringify([1, 2, 3])).toBe('[1,2,3]');
      expect(safeStringify(['a', 'b', 'c'])).toBe('["a","b","c"]');
      expect(safeStringify([{ id: 1 }, { id: 2 }])).toBe('[{"id":1},{"id":2}]');
    });

    it('should handle empty arrays', () => {
      expect(safeStringify([])).toBe('[]');
    });

    it('should handle mixed type arrays', () => {
      expect(safeStringify([1, 'text', true, null])).toBe('[1,"text",true,null]');
    });
  });

  describe('primitives', () => {
    it('should handle numbers', () => {
      expect(safeStringify(42)).toBe('42');
      expect(safeStringify(0)).toBe('0');
      expect(safeStringify(-123)).toBe('-123');
      expect(safeStringify(3.14)).toBe('3.14');
    });

    it('should handle booleans', () => {
      expect(safeStringify(true)).toBe('true');
      expect(safeStringify(false)).toBe('false');
    });

    it('should handle null and undefined', () => {
      expect(safeStringify(null)).toBe('null');
      expect(safeStringify(undefined)).toBe('undefined');
    });

    it('should handle special numbers -- fallsback to JSON.stringify behavior', () => {
      expect(safeStringify(Infinity)).toBe('null');
      expect(safeStringify(-Infinity)).toBe('null');
      expect(safeStringify(NaN)).toBe('null');
    });
  });

  describe('edge cases that trigger fallback', () => {
    it('should handle BigInt by falling back to String()', () => {
      const result = safeStringify(BigInt(123));
      expect(result).toBe('123');
    });

    it('should handle Symbols by falling back to String()', () => {
      const sym = Symbol('test');
      const result = safeStringify(sym);
      expect(result).toBe('Symbol(test)');
    });

    it('should handle functions by falling back to String()', () => {
      const fn = function testFunction() {
        return 'test';
      };
      const result = safeStringify(fn);
      expect(result).toContain('function testFunction');
    });

    it('should handle Date objects', () => {
      const date = new Date('2023-01-01T00:00:00.000Z');
      expect(safeStringify(date)).toBe('"2023-01-01T00:00:00.000Z"');
    });

    it('should handle RegExp objects', () => {
      const regex = /test/gi;
      expect(safeStringify(regex)).toBe('{}'); // RegExp JSON.stringify to {}
    });

    it('should handle Error objects', () => {
      const error = new Error('Test error');
      expect(safeStringify(error)).toBe('{}'); // Error objects JSON.stringify to {}
    });
  });

  describe('complex scenarios', () => {
    it('should handle objects with JSON.stringify edge cases', () => {
      const complex = {
        string: 'hello',
        number: 42,
        boolean: true,
        nullValue: null,
        undefinedValue: undefined, // This will be omitted in JSON.stringify
        array: [1, 2, 3],
        nested: { inner: 'value' },
      };

      // undefined properties are omitted in JSON.stringify
      expect(safeStringify(complex)).toBe(
        '{"string":"hello","number":42,"boolean":true,"nullValue":null,"array":[1,2,3],"nested":{"inner":"value"}}'
      );
    });

    it('should handle arrays with undefined and null', () => {
      expect(safeStringify([1, undefined, null, 3])).toBe('[1,null,null,3]');
    });

    it('should handle deeply nested structures', () => {
      const deep = { level1: { level2: { level3: { value: 'deep' } } } };
      expect(safeStringify(deep)).toBe('{"level1":{"level2":{"level3":{"value":"deep"}}}}');
    });
  });
});
