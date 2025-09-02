import { useStyletron } from 'baseui';

import { DetailViewHeader } from './components/detail-view-header/detail-view-header';

import type { DetailViewProps } from './types';

export function DetailView({
  title,
  subtitle,
  onGoBack,
  headerContent,
  children,
}: DetailViewProps) {
  const [css, theme] = useStyletron();

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale800 })}>
      <DetailViewHeader title={title} subtitle={subtitle} onGoBack={onGoBack}>
        {headerContent}
      </DetailViewHeader>

      {children}
    </div>
  );
}
