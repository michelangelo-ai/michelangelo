import type { RepeatedLayoutState } from '#core/providers/repeated-layout-provider/types';

/**
 * Builds a field ID with array indices for repeated layouts.
 *
 * @example
 * entityId: 'spec.messages.contents.text'
 * rootFieldPath: 'spec.messages[3].contents'
 * index: 1
 * output: 'spec.messages[3].contents[1].text'
 */
export function buildIndexedFieldId({
  entityId,
  rootFieldPath,
  index,
}: RepeatedLayoutState & { entityId: string }): string {
  const rootWithoutIndices = rootFieldPath.replace(/\[\d+\]/g, '');
  const suffix = entityId.replace(rootWithoutIndices, '');
  return `${rootFieldPath}[${index}]${suffix}`;
}
