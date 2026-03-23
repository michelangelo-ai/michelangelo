import { useStyletron } from 'baseui';

import { CircleExclamationMarkKind } from './types';

import type { CircleExclamationMarkProps } from './types';

export function CircleExclamationMark({
  width = '64',
  height = '64',
  kind = CircleExclamationMarkKind.ERROR,
}: CircleExclamationMarkProps) {
  const [, theme] = useStyletron();
  const ringColor =
    kind === CircleExclamationMarkKind.ERROR
      ? theme.colors.borderNegative
      : theme.colors.contentStateDisabled;

  return (
    <svg
      aria-label="Circle Exclamation Mark icon"
      viewBox="0 0 38 38"
      width={width}
      height={height}
      fill="none"
      xmlns="http://www.w3.org/2000/svg"
    >
      <path
        d="M19 37.8891C29.4321 37.8891 37.8889 29.4323 37.8889 19.0002C37.8889 8.56817 29.4321 0.111328 19 0.111328C8.56796 0.111328 0.111115 8.56817 0.111115 19.0002C0.111115 29.4323 8.56796 37.8891 19 37.8891Z"
        fill={ringColor}
      />
      <path
        d="M18.9999 35.3133C28.0094 35.3133 35.3131 28.0096 35.3131 19.0001C35.3131 9.99065 28.0094 2.68701 18.9999 2.68701C9.99044 2.68701 2.6868 9.99065 2.6868 19.0001C2.6868 28.0096 9.99044 35.3133 18.9999 35.3133Z"
        fill="black"
      />
      <path
        d="M20.3652 24.5036C20.3652 25.2677 19.7728 25.8601 19.0087 25.8601C18.2446 25.8601 17.635 25.2677 17.635 24.5036C17.635 23.7394 18.2531 23.147 19.0087 23.147C19.7643 23.147 20.3652 23.7394 20.3652 24.5036ZM17.9441 21.9536L17.7552 12.1313H20.2536L20.0647 21.9536H17.9441Z"
        fill="white"
      />
    </svg>
  );
}
