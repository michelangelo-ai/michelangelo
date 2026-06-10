import { createRegistry, fromJson } from '@bufbuild/protobuf';
import { AnySchema } from '@bufbuild/protobuf/wkt';
import { describe, expect, it } from 'vitest';

import { TypedStructSchema } from '../gen/michelangelo/api/typed_struct_pb';

const anyJson = {
  '@type': 'type.googleapis.com/michelangelo.api.TypedStruct',
  typeUrl: 'type.googleapis.com/michelangelo.UniFlowConf',
  value: {},
};

describe('type registry', () => {
  it('throws without a registry when decoding Any containing a custom type', () => {
    expect(() => fromJson(AnySchema, anyJson)).toThrow(
      'type.googleapis.com/michelangelo.api.TypedStruct is not in the type registry'
    );
  });

  it('decodes Any containing a custom type when the registry is provided', () => {
    const registry = createRegistry(TypedStructSchema);
    const result = fromJson(AnySchema, anyJson, { registry });
    expect(result.typeUrl).toBe('type.googleapis.com/michelangelo.api.TypedStruct');
  });
});
