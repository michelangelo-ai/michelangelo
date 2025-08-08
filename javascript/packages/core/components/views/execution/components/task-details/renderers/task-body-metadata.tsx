import { LabelSmall } from 'baseui/typography';

import { Row } from '#core/components/row/row';
import { TaskPanel } from '#core/components/views/execution/styled-components';

import type { TaskBodyMetadataProps } from './types';

export function TaskBodyMetadata(props: TaskBodyMetadataProps) {
  const { label, cells, value } = props;

  return (
    <TaskPanel title={<LabelSmall>{label}</LabelSmall>}>
      <Row items={cells} record={value ?? undefined} />
    </TaskPanel>
  );
}
