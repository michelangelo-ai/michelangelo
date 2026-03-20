import type { IllustrationProps } from '../types';

export enum CircleExclamationMarkKind {
  ERROR,
  PRIMARY,
}

export type CircleExclamationMarkProps = Partial<IllustrationProps> & {
  kind?: CircleExclamationMarkKind;
};
