import { StatefulPanel } from 'baseui/accordion';
import { LabelSmall } from 'baseui/typography';

import { Row } from '#core/components/row/row';

import type { TaskBodyMetadataProps } from './types';

export function TaskBodyMetadata(props: TaskBodyMetadataProps) {
  const { label, cells, value } = props;

  return (
    <StatefulPanel title={<LabelSmall>{label}</LabelSmall>}>
      <Row items={cells} record={value ?? undefined} />
    </StatefulPanel>
  );
}
