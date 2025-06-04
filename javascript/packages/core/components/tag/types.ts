import type { TagProps as BaseTagProps } from 'baseui/tag';
import type { StyleFunction } from '#core/types/style-types';
import type { BEHAVIOR, COLOR, HIERARCHY, SIZE } from './constants';

export type TagSize = keyof typeof SIZE;
export type TagBehavior = keyof typeof BEHAVIOR;
export type TagColor = keyof typeof COLOR;
export type TagHierarchy = keyof typeof HIERARCHY;

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
