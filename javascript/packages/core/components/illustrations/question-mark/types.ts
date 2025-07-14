import type { IllustrationProps } from '../types';

export enum QuestionMarkKind {
  DEFAULT,
  GREY,
}

export type QuestionMarkProps = Partial<IllustrationProps> & {
  kind?: QuestionMarkKind;
};
