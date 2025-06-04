import { CellType } from '#core/components/cell/constants';
import { DefaultCellRenderer } from '#core/components/cell/renderers/default-cell-renderer';
import { DescriptionHierarchy } from '#core/components/cell/renderers/description/constants';
import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';
import { Link } from '#core/components/link/link';
import { BEHAVIOR, COLOR, SIZE } from '#core/components/tag/constants';
import { Tag } from '#core/components/tag/tag';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { useStudioQuery } from '#core/hooks/use-studio-query';

export function ProjectDetail() {
  const { projectId } = useStudioParams('base');
  const { data } = useStudioQuery<{ project: Record<string, string> }>({
    queryName: 'GetProject',
    serviceOptions: {
      name: projectId,
      namespace: projectId,
    },
  });

  return (
    <div>
      <DefaultCellRenderer
        column={{ id: 'spec.bool', type: CellType.BOOLEAN }}
        record={{ spec: { bool: true } }}
        value={true}
      />
      <DefaultCellRenderer
        column={{ id: 'spec.date', type: CellType.DATE }}
        record={{ spec: { date: Date.now() / 1000 } }}
        value={String(Date.now() / 1000)}
      />
      <br />
      <DefaultCellRenderer
        column={{ id: 'spec.description', type: CellType.DESCRIPTION }}
        record={{ spec: { description: 'Descriptive text in the column' } }}
        value={'Descriptive text in the column'}
      />
      <br />
      <Icon name="arrowLaunch" kind={IconKind.ACCENT} size={24} />
      <br />
      <DefaultCellRenderer
        column={{ id: 'spec.link', type: CellType.LINK, url: 'https://www.google.com' }}
        record={{ spec: { link: 'https://www.google.com' } }}
        value={'https://www.google.com'}
      />
      <br />
      External link <Link href="https://www.google.com">Google</Link>
      <br />
      Router link
      <Link href="/">Home</Link>
      <br />
      Multi cell:
      <DefaultCellRenderer
        column={{
          id: 'spec.multi',
          type: CellType.MULTI,
          items: [
            { id: 'metadata.name', type: CellType.LINK, url: '/pipelines/123' },
            {
              id: 'spec.revisionId',
              type: CellType.DESCRIPTION,
              hierarchy: DescriptionHierarchy.SECONDARY,
            },
            { id: 'spec.revisionId' },
          ],
        }}
        record={{
          metadata: { name: 'Pipeline 1' },
          spec: { revisionId: '123' },
        }}
        value={{
          metadata: { name: 'Pipeline 1' },
          spec: {
            revisionId: '123',
          },
        }}
      />
      <br />
      Tag:{' '}
      <Tag closeable={false} size={SIZE.xSmall} color={COLOR.gray} behavior={BEHAVIOR.display}>
        Tag
      </Tag>
      <br />
      <DefaultCellRenderer
        column={{
          id: 'spec.state',
          type: CellType.STATE,
          stateColorMap: { PIPELINE_STATE_BUILDING: 'blue' },
        }}
        record={{ spec: { state: 'PIPELINE_STATE_BUILDING' } }}
        value="PIPELINE_STATE_BUILDING"
      />
      <br />
      {/* The project type will not be directly exposed to the @michelangelo/core package. */}
      {/* eslint-disable-next-line @typescript-eslint/dot-notation */}
      Project Name: {data?.project?.metadata?.['name']}
    </div>
  );
}
