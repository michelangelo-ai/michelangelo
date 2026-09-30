import { describe, expect, it } from 'vitest';

import { CriterionOperationSchema } from '../gen/michelangelo/api/list_pb';
import { DeploymentStatusSchema } from '../gen/michelangelo/api/v2/deployment_pb';
import { walkMessage } from '../walk-message';

import type { MessageVisitor } from '../types';

describe('walkMessage', () => {
  it('visits repeated and nested message values with their descriptors', () => {
    const visited: string[] = [];
    const visit: MessageVisitor = (desc, value, descend) => {
      visited.push(desc.typeName);
      return descend(desc, value);
    };

    walkMessage(
      CriterionOperationSchema,
      {
        criterion: [{ fieldName: 'a', matchValue: 'x' }],
        subOperations: [{ criterion: [{ fieldName: 'b' }] }],
      },
      visit
    );

    expect(visited).toEqual([
      'michelangelo.api.Criterion',
      'google.protobuf.Any',
      'michelangelo.api.CriterionOperation',
      'michelangelo.api.Criterion',
    ]);
  });

  it('visits map values and puts the visitor result in their place', () => {
    const result = walkMessage(
      DeploymentStatusSchema,
      { providerStatus: { foo: 'bar', replicas: 3 } },
      (desc, value) => `${desc.typeName}:${String(value)}`
    );

    expect(result).toEqual({
      providerStatus: { foo: 'google.protobuf.Any:bar', replicas: 'google.protobuf.Any:3' },
    });
  });

  it('copies scalar fields and keys that are not fields unchanged', () => {
    const result = walkMessage(
      CriterionOperationSchema,
      { logicalOperator: 'LOGICAL_OPERATOR_AND', notAField: { nested: true } },
      () => {
        throw new Error('no message fields to visit');
      }
    );

    expect(result).toEqual({
      logicalOperator: 'LOGICAL_OPERATOR_AND',
      notAField: { nested: true },
    });
  });
});
