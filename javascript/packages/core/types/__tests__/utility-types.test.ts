import type { DeepPartial } from '../utility-types';

describe('utility-types', () => {
  describe('DeepPartial', () => {
    test('makes all properties optional', () => {
      interface TestInterface {
        required: string;
        nested: {
          alsoRequired: number;
          deepNested: {
            value: boolean;
          };
        };
        array: string[];
      }

      // This should compile without errors
      const partial: DeepPartial<TestInterface> = {};
      const partialWithSome: DeepPartial<TestInterface> = {
        required: 'test',
      };
      const partialWithNested: DeepPartial<TestInterface> = {
        nested: {
          alsoRequired: 42,
        },
      };
      const partialWithDeepNested: DeepPartial<TestInterface> = {
        nested: {
          deepNested: {
            value: true,
          },
        },
      };

      // Type assertions to ensure the types work correctly
      expect(partial).toBeDefined();
      expect(partialWithSome.required).toBe('test');
      expect(partialWithNested.nested?.alsoRequired).toBe(42);
      expect(partialWithDeepNested.nested?.deepNested?.value).toBe(true);
    });

    test('preserves primitive types', () => {
      interface SimpleInterface {
        str: string;
        num: number;
        bool: boolean;
      }

      const partial: DeepPartial<SimpleInterface> = {
        str: 'test', // Should still be string, not string | undefined
      };

      expect(typeof partial.str).toBe('string');
    });
  });
});
