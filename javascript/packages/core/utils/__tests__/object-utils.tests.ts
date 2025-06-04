import { getObjectValue } from '../object-utils';

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
});
