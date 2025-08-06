import { StatefulPanel } from 'baseui/accordion';
import { LabelSmall } from 'baseui/typography';

import { TextEditor } from '#core/components/text-editor/text-editor';
import { decodeStruct, isStruct } from '#core/utils/proto/struct-utils';

export function TaskBodyStruct(props: { label: string; value: object | undefined | null }) {
  const { label, value } = props;

  const decodedValue = isStruct(value) ? decodeStruct(value) : value;
  const prettyJson = JSON.stringify(decodedValue, null, 2);

  return (
    <StatefulPanel title={<LabelSmall>{label}</LabelSmall>}>
      <TextEditor readOnly language="json" value={prettyJson} onChange={() => null} />
    </StatefulPanel>
  );
}
