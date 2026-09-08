import { useStyletron } from 'baseui';
import { LabelSmall } from 'baseui/typography';

import type { ReactNode } from 'react';

/** A labeled value within a DeploymentRevisionCard's metadata grid. */
export function DeploymentRevisionCardField({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  const [, theme] = useStyletron();

  return (
    <div>
      <LabelSmall color={theme.colors.contentSecondary} marginBottom={theme.sizing.scale200}>
        {label}
      </LabelSmall>
      {children}
    </div>
  );
}
