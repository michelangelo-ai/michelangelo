import { safeLocalStorageGetItem, safeLocalStorageSetItem } from '../local-storage-utils';

describe('local-storage-utils', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  describe('safeLocalStorageSetItem', () => {
    it('should store an item in localStorage', () => {
      const testValue = { foo: 'bar', count: 42 };

      safeLocalStorageSetItem('test-key', testValue);

      expect(localStorage.getItem('test-key')).toBe(JSON.stringify(testValue));
    });

    it('should handle localStorage errors gracefully', () => {
      const originalSetItem = (...args: Parameters<typeof localStorage.setItem>) =>
        localStorage.setItem(...args);
      localStorage.setItem = vi.fn(() => {
        throw new Error('QuotaExceededError');
      });

      expect(() => {
        safeLocalStorageSetItem('test-key', { data: 'test' });
      }).not.toThrow();

      localStorage.setItem = originalSetItem;
    });

    it('should store different data types', () => {
      const testCases = [
        { key: 'string-key', value: 'simple string', expected: '"simple string"' },
        { key: 'number-key', value: 123, expected: '123' },
        { key: 'boolean-key', value: true, expected: 'true' },
        { key: 'array-key', value: [1, 2, 3], expected: '[1,2,3]' },
        { key: 'null-key', value: null, expected: 'null' },
        { key: 'object-key', value: { foo: 'bar' }, expected: '{"foo":"bar"}' },
      ];

      testCases.forEach(({ key, value, expected }) => {
        safeLocalStorageSetItem(key, value);
        expect(localStorage.getItem(key)).toBe(expected);
      });
    });
  });

  describe('safeLocalStorageGetItem', () => {
    it('should retrieve an item from localStorage', () => {
      const testValue = { foo: 'bar', count: 42 };
      localStorage.setItem('test-key', JSON.stringify(testValue));

      const result = safeLocalStorageGetItem('test-key', {});

      expect(result).toEqual(testValue);
    });

    it('should return default value when item does not exist', () => {
      const defaultValue = { default: true };

      const result = safeLocalStorageGetItem('non-existent-key', defaultValue);

      expect(result).toBe(defaultValue);
    });

    it('should return default value when localStorage item is null', () => {
      localStorage.setItem('null-key', 'null');
      const defaultValue = { default: true };

      const result = safeLocalStorageGetItem('null-key', defaultValue);

      expect(result).toBe(defaultValue);
    });

    it('should handle JSON parsing errors gracefully', () => {
      localStorage.setItem('invalid-json', 'invalid json string');
      const defaultValue = { default: true };

      const result = safeLocalStorageGetItem('invalid-json', defaultValue);

      expect(result).toBe(defaultValue);
    });

    it('should handle localStorage errors gracefully', () => {
      const originalGetItem = (...args: Parameters<typeof localStorage.getItem>) =>
        localStorage.getItem(...args);
      localStorage.getItem = vi.fn(() => {
        throw new Error('SecurityError');
      });

      const defaultValue = { default: true };
      const result = safeLocalStorageGetItem('test-key', defaultValue);

      expect(result).toBe(defaultValue);

      localStorage.getItem = originalGetItem;
    });

    it('should handle different data types correctly', () => {
      const testCases = [
        {
          key: 'string-key',
          storedValue: '"simple string"',
          defaultValue: '',
          expected: 'simple string',
        },
        { key: 'number-key', storedValue: '123', defaultValue: 0, expected: 123 },
        { key: 'boolean-key', storedValue: 'true', defaultValue: false, expected: true },
        { key: 'array-key', storedValue: '[1,2,3]', defaultValue: [], expected: [1, 2, 3] },
        {
          key: 'object-key',
          storedValue: '{"foo":"bar"}',
          defaultValue: {},
          expected: { foo: 'bar' },
        },
      ];

      testCases.forEach(({ key, storedValue, defaultValue, expected }) => {
        localStorage.setItem(key, storedValue);
        const result = safeLocalStorageGetItem(key, defaultValue);
        expect(result).toEqual(expected);
      });
    });
  });
});
