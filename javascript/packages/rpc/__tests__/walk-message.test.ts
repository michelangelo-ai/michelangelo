import { describe, expect, it } from 'vitest';

import { createTestSchemas } from '../__fixtures__/test-schemas';
import { walkMessage } from '../walk-message';

import type { MessageVisitor } from '../types';

describe('walkMessage', () => {
  it('visits singular, repeated, and nested message values with their descriptors', () => {
    const { TreeSchema } = createTestSchemas();
    const visited: string[] = [];
    const visit: MessageVisitor = (desc, value, descend) => {
      visited.push(desc.typeName);
      return descend(desc, value);
    };

    walkMessage(
      TreeSchema,
      { leaf: { name: 'a' }, children: [{ leaf: { name: 'b' } }], payload: 'x' },
      visit
    );

    expect(visited).toEqual(['test.Leaf', 'test.Tree', 'test.Leaf', 'google.protobuf.Any']);
  });

  it('visits map values and puts the visitor result in their place', () => {
    const { TreeSchema } = createTestSchemas();
    const result = walkMessage(
      TreeSchema,
      { extras: { foo: 'bar', replicas: 3 } },
      (desc, value) => `${desc.typeName}:${String(value)}`
    );

    expect(result).toEqual({
      extras: { foo: 'google.protobuf.Any:bar', replicas: 'google.protobuf.Any:3' },
    });
  });

  it('copies scalar fields and keys that are not fields unchanged', () => {
    const { TreeSchema } = createTestSchemas();
    const result = walkMessage(TreeSchema, { label: 'x', notAField: { nested: true } }, () => {
      throw new Error('no message fields to visit');
    });

    expect(result).toEqual({ label: 'x', notAField: { nested: true } });
  });
});
