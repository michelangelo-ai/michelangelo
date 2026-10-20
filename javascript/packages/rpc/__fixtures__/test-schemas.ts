import { create, createFileRegistry } from '@bufbuild/protobuf';
import {
  FieldDescriptorProto_Label,
  FieldDescriptorProto_Type,
  file_google_protobuf_any,
  FileDescriptorProtoSchema,
} from '@bufbuild/protobuf/wkt';

const { OPTIONAL, REPEATED } = FieldDescriptorProto_Label;
const { MESSAGE, STRING } = FieldDescriptorProto_Type;

/**
 * Builds the registry and `Tree` schema for a test-only proto file that covers every field kind
 * a schema walk handles, without depending on the generated API schemas:
 *
 * ```proto
 * package test;
 * message Leaf { string name = 1; }
 * message Tree {
 *   Leaf leaf = 1;
 *   repeated Tree children = 2;
 *   map<string, google.protobuf.Any> extras = 3;
 *   string label = 4;
 *   google.protobuf.Any payload = 5;
 * }
 * ```
 */
export function createTestSchemas() {
  const registry = createFileRegistry(
    create(FileDescriptorProtoSchema, {
      name: 'test.proto',
      package: 'test',
      syntax: 'proto3',
      dependency: ['google/protobuf/any.proto'],
      messageType: [
        { name: 'Leaf', field: [{ name: 'name', number: 1, type: STRING, label: OPTIONAL }] },
        {
          name: 'Tree',
          field: [
            { name: 'leaf', number: 1, type: MESSAGE, typeName: '.test.Leaf', label: OPTIONAL },
            { name: 'children', number: 2, type: MESSAGE, typeName: '.test.Tree', label: REPEATED },
            {
              name: 'extras',
              number: 3,
              type: MESSAGE,
              typeName: '.test.Tree.ExtrasEntry',
              label: REPEATED,
            },
            { name: 'label', number: 4, type: STRING, label: OPTIONAL },
            {
              name: 'payload',
              number: 5,
              type: MESSAGE,
              typeName: '.google.protobuf.Any',
              label: OPTIONAL,
            },
          ],
          nestedType: [
            {
              name: 'ExtrasEntry',
              options: { mapEntry: true },
              field: [
                { name: 'key', number: 1, type: STRING, label: OPTIONAL },
                {
                  name: 'value',
                  number: 2,
                  type: MESSAGE,
                  typeName: '.google.protobuf.Any',
                  label: OPTIONAL,
                },
              ],
            },
          ],
        },
      ],
    }),
    (protoFileName) =>
      protoFileName === 'google/protobuf/any.proto' ? file_google_protobuf_any : undefined
  );

  const TreeSchema = registry.getMessage('test.Tree');
  if (!TreeSchema) throw new Error('test.Tree is not in the test registry');
  return { registry, TreeSchema };
}
