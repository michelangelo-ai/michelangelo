import { sentenceCaseEnumValue } from '#core/utils/string-utils';
import { TYPE_LIKE_PREFIXES } from './constants';

import type { CellToStringParams } from '#core/components/cell/types';
import type { TypeCellConfig } from './types';

export const typeCellToString = ({
  value,
  column,
}: CellToStringParams<string, TypeCellConfig>): string => {
  if (!value) return '';

  const knownTranslation = column.typeTextMap?.[value];
  if (knownTranslation) return knownTranslation;

  for (const typeLikePrefix of TYPE_LIKE_PREFIXES) {
    // We expect the `value` to be a string type here, possessing an `.includes()` method.
    // However, it's possible for `value` to be a number, which occurs when the .proto
    // definitions on the backend are updated (e.g., new pipeline types are introduced), but the
    // frontend hasn't been updated yet. In such cases, calling `value.includes()` causes the
    // web page to crash. To prevent this, we ensure `value` is a string and possesses the
    // `.includes()` method.
    if (typeof value === 'string' && value.includes(`_${typeLikePrefix}_`)) {
      return sentenceCaseEnumValue(value, new RegExp(`(\\w+_)${typeLikePrefix}_`));
    }
  }

  return sentenceCaseEnumValue(value);
};
