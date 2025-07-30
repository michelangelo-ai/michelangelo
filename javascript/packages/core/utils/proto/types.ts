// Core type definitions for protocol buffer structs
export type Struct = {
  fields: Fields;
};

export type Fields = Record<string, ProtobufValue>;

export type ProtobufValue = {
  boolValue?: boolean;
  listValue?: { values: ProtobufValue[] };
  nullValue?: null;
  numberValue?: number;
  stringValue?: string;
  structValue?: Struct;
};

export type DecodedStruct = unknown;
