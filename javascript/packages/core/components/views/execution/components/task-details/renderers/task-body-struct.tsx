import { LabelSmall } from 'baseui/typography';

import { TextEditor } from '#core/components/text-editor/text-editor';
import { TaskPanel } from '#core/components/views/execution/styled-components';
import { decodeStruct, isStruct } from '#core/utils/proto/struct-utils';

import type { TaskBodyStructProps } from './types';

export function TaskBodyStruct(props: TaskBodyStructProps) {
  const { label, value } = props;

  const decodedValue = isStruct(value) ? decodeStruct(value) : value;
  const prettyJson = JSON.stringify(decodedValue, null, 2);

  return (
    <TaskPanel title={<LabelSmall>{label}</LabelSmall>}>
      <TextEditor readOnly language="json" value={prettyJson} onChange={() => null} />
    </TaskPanel>
  );
}
