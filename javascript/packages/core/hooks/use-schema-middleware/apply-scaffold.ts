import { merge } from 'lodash';
import YAML from 'yaml';
import { YAMLError } from 'yaml/util';

import { getObjectValue } from '#core/utils/object-utils';

import type { MiddlewareSchema } from './types';

export function applyScaffold<T extends object>(data: T, schema: MiddlewareSchema): T {
  if (!schema.scaffold && !schema.scaffoldBySubType) return data;

  if (schema.scaffoldBySubType) {
    const subType = getObjectValue<string>(data, schema.subTypePath!) ?? '';
    return merge({}, parseYaml(schema.scaffoldBySubType[subType]), data);
  }

  return merge({}, parseYaml(schema.scaffold!), data);
}

function parseYaml(scaffold: string): unknown {
  try {
    return YAML.parse(scaffold);
  } catch (error) {
    if (error instanceof YAMLError) {
      throw new Error('Request requires scaffolding, but found invalid YAML scaffold');
    }
    throw error;
  }
}
