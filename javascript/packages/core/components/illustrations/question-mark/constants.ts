import { QuestionMarkKind } from './types';

import type { Theme } from 'baseui/theme';

export const QUESTION_MARK_COLORS: Record<
  QuestionMarkKind,
  {
    outlineColor: keyof Theme['colors'];
    questionMarkColor: keyof Theme['colors'];
    backgroundColor: keyof Theme['colors'];
  }
> = {
  [QuestionMarkKind.DEFAULT]: {
    outlineColor: 'safety',
    questionMarkColor: 'white',
    backgroundColor: 'black',
  },
  [QuestionMarkKind.GREY]: {
    outlineColor: 'tableFilter',
    questionMarkColor: 'white',
    backgroundColor: 'black',
  },
};
