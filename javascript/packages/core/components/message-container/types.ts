import type { Override } from 'baseui/overrides';

export enum MessageLevel {
  ERROR,
  WARNING,
  INFO,
}

export type MessageContainerProps = {
  message: string;
  level?: MessageLevel;
  overrides?: MessageContainerOverrides;
};

type MessageContainerOverrides = {
  Root?: Override;
  Content?: Override;
};
