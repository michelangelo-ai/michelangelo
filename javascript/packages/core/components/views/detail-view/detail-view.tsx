import { useStyletron } from 'baseui';

import { DetailViewHeader } from './components/detail-view-header/detail-view-header';

import type { DetailViewProps } from './types/detail-view-component-types';

export function DetailView({
  title,
  subtitle,
  onGoBack,
  headerContent,
  children,
  actions,
  record,
  loading,
  titleEnhancer,
}: DetailViewProps) {
  const [css, theme] = useStyletron();

  return (
    <div className={css({ display: 'flex', flexDirection: 'column', gap: theme.sizing.scale800 })}>
      <DetailViewHeader
        title={title}
        subtitle={subtitle}
        titleEnhancer={titleEnhancer}
        onGoBack={onGoBack}
        actions={actions}
        record={record}
        loading={loading}
      >
        {headerContent}
      </DetailViewHeader>

      {children}
    </div>
  );
}
