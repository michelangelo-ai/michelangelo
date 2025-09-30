import { LabelSmall } from 'baseui/typography';

import { CollapsibleBox } from '#core/components/box/collapsible-box';
import { TextEditor } from '#core/components/text-editor/text-editor';
import { decodeStruct, isStruct } from '#core/utils/proto/struct-utils';

import type { TaskBodyStructProps } from './types';

export function TaskBodyStruct(props: TaskBodyStructProps) {
  const { label, value } = props;

  const decodedValue = isStruct(value) ? decodeStruct(value) : value;
  const prettyJson = JSON.stringify(decodedValue, null, 2);

  return (
    <CollapsibleBox title={<LabelSmall>{label}</LabelSmall>}>
      <TextEditor readOnly language="json" value={prettyJson} onChange={() => null} />
    </CollapsibleBox>
  );
}
