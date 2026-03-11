import { useEffect, useRef } from 'react';
import { useFieldArray } from 'react-final-form-arrays';

import type { ArrayFieldOptions } from '#core/components/form/types';

/**
 * @param rootFieldPath - Dot-notation path to the root field of the array.
 *
 * @returns An object containing the following properties:
 * - `entries`: one entry per array item, each with a stable `id` (UUID) for use as a React
 * `key` prop and an `indexedFieldPath` for constructing nested field names. Guaranteed to have at
 * least `minItems` entries after the initial effect runs.
 *
 * - `add`: Pushes one new empty item to the array.
 *
 * - `remove`: Removes an item from the array.
 *
 * - `isRemovable`: Indicates if an array item can be removed.
 *
 * @example
 * ```tsx
 * const { entries, add, remove, isRemovable } = useArrayField('contacts', { minItems: 1 });
 *
 * return (
 *   <>
 *     {entries.map(({ id, indexedFieldPath }, index) => (
 *       <div key={id}>
 *         <StringField name={`${indexedFieldPath}.email`} label="Email" />
 *         {isRemovable && <button onClick={() => remove(index)}>Remove</button>}
 *       </div>
 *     ))}
 *     <button onClick={add}>Add more</button>
 *   </>
 * );
 * ```
 */
export function useArrayField(
  rootFieldPath: string,
  { minItems = 0, readOnly = false }: ArrayFieldOptions = {}
) {
  const { fields } = useFieldArray(rootFieldPath);

  const entryIds = useRef<string[]>(
    Array.from({ length: fields.length ?? 0 }, () => crypto.randomUUID())
  );

  useEffect(() => {
    const currentLength = fields.length ?? 0;
    if (currentLength < minItems) {
      for (let i = currentLength; i < minItems; i++) {
        entryIds.current.push(crypto.randomUUID());
        fields.push({});
      }
    }
  }, [fields, minItems]);

  const add = () => {
    entryIds.current.push(crypto.randomUUID());
    fields.push({});
  };

  const remove = (index: number) => {
    entryIds.current.splice(index, 1);
    fields.remove(index);
  };

  return {
    entries: fields.map((indexedFieldPath, index) => ({
      id: entryIds.current[index],
      indexedFieldPath,
    })),
    add,
    remove,
    isRemovable: (fields.length ?? 0) > minItems && !readOnly,
  };
}
