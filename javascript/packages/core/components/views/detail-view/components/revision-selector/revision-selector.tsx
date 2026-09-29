import { useStyletron } from 'baseui';
import { Button, KIND, SIZE } from 'baseui/button';
import { PLACEMENT, StatefulPopover } from 'baseui/popover';

import { DateTime } from '#core/components/date-time/date-time';
import { Icon } from '#core/components/icon/icon';
import { TAG_COLOR, TAG_SIZE } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';
import { formatRevisionLabel } from '#core/utils/revision-utils';
import { capitalizeFirstLetter } from '#core/utils/string-utils';
import {
  RevisionColumns,
  RevisionLabel,
  RevisionList,
  RevisionListHeader,
  RevisionRow,
} from './styled-components';

import type { RevisionOption, RevisionSelectorProps } from './types';

/** `SORT_ORDER_DESC` from `proto/api/list.proto` — protobuf-es encodes enums by number. */
const SORT_ORDER_DESC = 2;

/** `CRITERION_OPERATOR_EQUAL` from `proto/api/list.proto`. */
const CRITERION_OPERATOR_EQUAL = 1;

/**
 * Header dropdown listing the Revision snapshots of a revisioned entity. The Revision that
 * `latestRevisionName` names is badged "Latest", both in the list and on the trigger when it
 * is the one shown.
 */
export function RevisionSelector({
  service,
  entityId,
  selectedRevisionId,
  latestRevisionName,
  onSelect,
}: RevisionSelectorProps) {
  const [css, theme] = useStyletron();
  const { projectId } = useStudioParams('base');

  const { data } = useStudioQuery<{ revisionList?: { items?: RevisionOption[] } }>({
    queryName: 'ListRevision',
    serviceOptions: {
      namespace: projectId,
      listOptionsExt: {
        operation: {
          criterion: [
            {
              fieldName: 'revision.base_type',
              operator: CRITERION_OPERATOR_EQUAL,
              matchValue: capitalizeFirstLetter(service),
            },
            {
              fieldName: 'revision.base_resource_name',
              operator: CRITERION_OPERATOR_EQUAL,
              matchValue: entityId,
            },
          ],
        },
        orderBy: [{ field: 'metadata.update_timestamp', dir: SORT_ORDER_DESC }],
      },
    },
  });

  const revisions = data?.revisionList?.items ?? [];
  const isLatest = (revision: RevisionOption) => revision.metadata.name === latestRevisionName;
  const selected =
    revisions.find((revision) => revision.spec.revisionId === selectedRevisionId) ?? revisions[0];

  if (!selected) return null;

  return (
    <StatefulPopover
      focusLock
      placement={PLACEMENT.bottomLeft}
      content={({ close }) => (
        <RevisionList role="listbox" aria-label="Revisions">
          <RevisionListHeader>
            <RevisionColumns>
              <span>Revision</span>
              <span>Branch</span>
              <span>Last updated</span>
            </RevisionColumns>
          </RevisionListHeader>
          {revisions.map((revision) => {
            const isSelected = revision.spec.revisionId === selected.spec.revisionId;
            return (
              <RevisionRow
                key={revision.metadata.name}
                role="option"
                aria-selected={isSelected}
                $isSelected={isSelected}
                onClick={() => {
                  onSelect(revision.spec.revisionId);
                  close();
                }}
              >
                <RevisionColumns>
                  {renderRevisionCells(revision, isLatest(revision))}
                </RevisionColumns>
              </RevisionRow>
            );
          })}
        </RevisionList>
      )}
    >
      <Button
        kind={KIND.secondary}
        size={SIZE.compact}
        aria-label="Select revision"
        endEnhancer={<Icon name="chevronDown" size={theme.sizing.scale600} />}
        overrides={{
          BaseButton: { style: { ...theme.typography.LabelMedium, fontWeight: 'normal' } },
        }}
      >
        <span className={css({ display: 'flex', gap: theme.sizing.scale800 })}>
          {renderRevisionCells(selected, isLatest(selected))}
        </span>
      </Button>
    </StatefulPopover>
  );
}

function renderRevisionCells(revision: RevisionOption, latest: boolean) {
  return (
    <>
      <RevisionLabel>
        {formatRevisionLabel(revision.spec.revisionId)}
        {latest && (
          <Tag color={TAG_COLOR.green} size={TAG_SIZE.xSmall} closeable={false}>
            Latest
          </Tag>
        )}
      </RevisionLabel>
      <span>{revision.spec.gitCommit?.branch ?? '—'}</span>
      <span>
        <DateTime timestamp={revision.metadata.creationTimestamp?.seconds} />
      </span>
    </>
  );
}
