import type { TagProps as BaseTagProps } from 'baseui/tag';
import type { StyleFunction } from '#core/types/style-types';
import type { TAG_BEHAVIOR, TAG_COLOR, TAG_HIERARCHY, TAG_SIZE } from './constants';

export type TagSize = keyof typeof TAG_SIZE;
export type TagBehavior = keyof typeof TAG_BEHAVIOR;
export type TagColor = keyof typeof TAG_COLOR;
export type TagHierarchy = keyof typeof TAG_HIERARCHY;

export interface Props extends Omit<BaseTagProps, 'size' | 'kind'> {
  size?: TagSize;
  behavior?: TagBehavior;
  color?: TagColor;
  hierarchy?: TagHierarchy;
}

export type ColorOverrides = Record<
  TagHierarchy,
  Record<TagBehavior, Partial<Record<TagColor, StyleFunction>>>
>;
