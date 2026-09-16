import { create, toBinary } from '@bufbuild/protobuf';
import { anyPack } from '@bufbuild/protobuf/wkt';
import { expect, it, vi } from 'vitest';

import { TypedStructSchema } from '../gen/michelangelo/api/typed_struct_pb';
import {
  PipelineManifest_Type,
  PipelineSchema,
  PipelineType,
} from '../gen/michelangelo/api/v2/pipeline_pb';
import { request } from '../request';

vi.mock('../handlers', () => ({
  getRpcHandlers: vi.fn(),
}));

const { getRpcHandlers } = await import('../handlers');
const mockGetRpcHandlers = getRpcHandlers as ReturnType<typeof vi.fn>;

function mockHandler(response: unknown) {
  mockGetRpcHandlers.mockResolvedValue({
    GetPipelineRun: vi.fn().mockResolvedValue(response),
  });
}

it('strips $typeName and $unknown from response', async () => {
  mockHandler({ $typeName: 'foo.Bar', $unknown: [], name: 'test' });

  expect(await request('GetPipelineRun', {} as never)).toEqual({ name: 'test' });
});

it('recursively strips protobuf internals from nested objects', async () => {
  mockHandler({
    $typeName: 'outer',
    nested: { $typeName: 'inner', value: 1 },
  });

  expect(await request('GetPipelineRun', {} as never)).toEqual({ nested: { value: 1 } });
});

it('preserves Uint8Array fields without corrupting them into plain objects', async () => {
  const bytes = new Uint8Array([1, 2, 3]);
  mockHandler({ $typeName: 'foo.Any', typeUrl: 'type.googleapis.com/foo', value: bytes });

  const result = await request('GetPipelineRun', {} as never);

  expect((result as { value: Uint8Array }).value).toBeInstanceOf(Uint8Array);
  expect((result as { value: Uint8Array }).value).toEqual(bytes);
});

it('handles arrays containing objects with protobuf internals', async () => {
  mockHandler({
    items: [
      { $typeName: 'foo', x: 1 },
      { $typeName: 'bar', x: 2 },
    ],
  });

  expect(await request('GetPipelineRun', {} as never)).toEqual({ items: [{ x: 1 }, { x: 2 }] });
});

it('unpacks a registered Any payload (Pipeline) into a plain object', async () => {
  const pipeline = create(PipelineSchema, {
    metadata: { name: 'my-pipeline' },
    spec: { type: PipelineType.DATA_PREP, commit: { branch: 'main' } },
  });
  mockHandler({
    $typeName: 'michelangelo.api.v2.Revision',
    spec: {
      $typeName: 'michelangelo.api.v2.RevisionSpec',
      revisionId: 'abc',
      content: {
        $typeName: 'google.protobuf.Any',
        typeUrl: 'type.googleapis.com/michelangelo.api.v2.Pipeline',
        value: toBinary(PipelineSchema, pipeline),
      },
    },
  });

  const result = (await request('GetPipelineRun', {} as never)) as {
    spec: {
      content: { metadata: { name: string }; spec: { type: number; commit: { branch: string } } };
    };
  };

  expect(result.spec.content.metadata.name).toBe('my-pipeline');
  expect(result.spec.content.spec.type).toBe(PipelineType.DATA_PREP);
  expect(result.spec.content.spec.commit.branch).toBe('main');
  expect(result.spec.content).not.toHaveProperty('$typeName');
});

it('unpacks a registered Any payload whose own fields contain a nested TypedStruct Any', async () => {
  // The Pipeline packed into Revision.spec.content (a registry-typed Any) has its own
  // manifest.content field, which is a TypedStruct. Both must unpack in one pass: toPlainObject
  // recurses into the decoded Pipeline and finds the inner Any too.
  const manifestContent = anyPack(
    TypedStructSchema,
    create(TypedStructSchema, {
      typeUrl: 'type.googleapis.com/michelangelo.pipeline.dataprep.Config',
      value: { source: 'hive' },
    })
  );
  const pipeline = create(PipelineSchema, {
    metadata: { name: 'my-pipeline' },
    spec: {
      type: PipelineType.DATA_PREP,
      commit: { branch: 'main' },
      manifest: {
        type: PipelineManifest_Type.PIPELINE_MANIFEST_TYPE_YAML,
        content: manifestContent,
      },
    },
  });
  mockHandler({
    $typeName: 'michelangelo.api.v2.Revision',
    spec: {
      $typeName: 'michelangelo.api.v2.RevisionSpec',
      revisionId: 'abc',
      content: {
        $typeName: 'google.protobuf.Any',
        typeUrl: 'type.googleapis.com/michelangelo.api.v2.Pipeline',
        value: toBinary(PipelineSchema, pipeline),
      },
    },
  });

  const result = (await request('GetPipelineRun', {} as never)) as {
    spec: {
      content: {
        spec: { manifest: { content: { typeUrl: string; value: { source: string } } } };
      };
    };
  };

  expect(result.spec.content.spec.manifest.content).toEqual({
    typeUrl: 'type.googleapis.com/michelangelo.pipeline.dataprep.Config',
    value: { source: 'hive' },
  });
});

it('leaves an unregistered Any payload untouched', async () => {
  const bytes = new Uint8Array([9, 9]);
  mockHandler({
    $typeName: 'google.protobuf.Any',
    typeUrl: 'type.googleapis.com/unknown.Type',
    value: bytes,
  });

  expect(await request('GetPipelineRun', {} as never)).toEqual({
    typeUrl: 'type.googleapis.com/unknown.Type',
    value: bytes,
  });
});
