import { getObjectSymbols, getObjectValue } from '../object-utils';

describe('object-utils', () => {
  describe('getObjectValue', () => {
    const testObject = {
      name: 'John',
      address: {
        street: '123 Main St',
        city: 'Boston',
      },
      scores: [1, 2, 3],
    };

    describe('with function accessor', () => {
      it('should return value when function returns truthy value', () => {
        const accessor = (obj: typeof testObject) => obj.name;
        expect(getObjectValue(testObject, accessor)).toBe('John');
      });

      it('should return default value when function returns falsy value', () => {
        // @ts-expect-error Testing accessing non-existent property
        const accessor = (obj: typeof testObject) => obj.nonExistent as unknown;
        expect(getObjectValue(testObject, accessor, 'default')).toBe('default');
      });

      it('should return undefined when function returns falsy value and no default provided', () => {
        // @ts-expect-error Testing accessing non-existent property
        const accessor = (obj: typeof testObject) => obj.nonExistent as unknown;
        expect(getObjectValue(testObject, accessor)).toBeUndefined();
      });
    });

    describe('with string accessor', () => {
      it('should return value for simple path', () => {
        expect(getObjectValue(testObject, 'name')).toBe('John');
      });

      it('should return value for nested path', () => {
        expect(getObjectValue(testObject, 'address.city')).toBe('Boston');
      });

      it('should return value for array index', () => {
        expect(getObjectValue(testObject, 'scores.1')).toBe(2);
      });

      it('should return default value for non-existent path', () => {
        expect(getObjectValue(testObject, 'nonExistent', 'default')).toBe('default');
      });

      it('should return undefined for non-existent path with no default', () => {
        expect(getObjectValue(testObject, 'nonExistent')).toBeUndefined();
      });
    });

    describe('with invalid accessor', () => {
      it('should return undefined when accessor is neither function nor string', () => {
        // @ts-expect-error Testing invalid accessor type
        expect(getObjectValue(testObject, 123)).toBeUndefined();
      });
    });

    describe('with invalid object', () => {
      it('should return undefined when object is null', () => {
        expect(getObjectValue(null, 'name')).toBeUndefined();
      });

      it('should return undefined when object is undefined', () => {
        expect(getObjectValue(undefined, 'name')).toBeUndefined();
      });

      it('should return undefined when object is a primitive', () => {
        expect(getObjectValue(123, 'name')).toBeUndefined();
      });
    });
  });

  describe('getObjectSymbols', () => {
    test('extracts React symbols from objects', () => {
      const reactSymbol = Symbol.for('react.element');
      const customSymbol = Symbol('custom');

      const input = {
        [reactSymbol]: 'react-metadata',
        [customSymbol]: 'custom-value',
        normalProp: 'normal-value',
      };

      const symbols = getObjectSymbols(input);

      expect(symbols[reactSymbol]).toBe('react-metadata');
      expect(symbols[customSymbol]).toBe('custom-value');
      expect(Object.keys(symbols)).toHaveLength(0); // No string keys
      expect(Object.getOwnPropertySymbols(symbols)).toHaveLength(2);
    });

    test('returns empty object for null/undefined', () => {
      expect(getObjectSymbols(null)).toEqual({});
      expect(getObjectSymbols(undefined)).toEqual({});
    });

    test('returns empty object for primitives', () => {
      expect(getObjectSymbols('string')).toEqual({});
      expect(getObjectSymbols(123)).toEqual({});
      expect(getObjectSymbols(true)).toEqual({});
    });

    test('extracts multiple symbols', () => {
      const sym1 = Symbol('first');
      const sym2 = Symbol('second');
      const sym3 = Symbol.for('global');

      const input = {
        [sym1]: 'value1',
        [sym2]: 'value2',
        [sym3]: 'value3',
        regularProp: 'ignored',
      };

      const symbols = getObjectSymbols(input);

      expect(symbols[sym1]).toBe('value1');
      expect(symbols[sym2]).toBe('value2');
      expect(symbols[sym3]).toBe('value3');
      expect(Object.getOwnPropertySymbols(symbols)).toHaveLength(3);
    });

    test('handles objects with no symbols', () => {
      const input = {
        prop1: 'value1',
        prop2: 'value2',
      };

      const symbols = getObjectSymbols(input);

      expect(symbols).toEqual({});
      expect(Object.getOwnPropertySymbols(symbols)).toHaveLength(0);
    });

    test('extracts well-known symbols', () => {
      const input = {
        [Symbol.iterator]: function* () {
          yield 1;
        },
        [Symbol.toStringTag]: 'CustomObject',
        regularProp: 'value',
      };

      const symbols = getObjectSymbols(input);

      expect(symbols[Symbol.iterator]).toBe(input[Symbol.iterator]);
      expect(symbols[Symbol.toStringTag]).toBe('CustomObject');
      expect(Object.getOwnPropertySymbols(symbols)).toHaveLength(2);
    });
  });
});
