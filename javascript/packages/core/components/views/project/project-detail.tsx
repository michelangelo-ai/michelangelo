import { CellType } from '#core/components/cell/constants';
import { BooleanCell } from '#core/components/cell/renderers/boolean/boolean-cell';
import { DateCell } from '#core/components/cell/renderers/date/date-cell';
import { DescriptionHierarchy } from '#core/components/cell/renderers/description/constants';
import { DescriptionCell } from '#core/components/cell/renderers/description/description-cell';
import { LinkCell } from '#core/components/cell/renderers/link/link-cell';
import { MultiCell } from '#core/components/cell/renderers/multi/multi-cell';
import { Icon } from '#core/components/icon/icon';
import { IconKind } from '#core/components/icon/types';
import { Link } from '#core/components/link/link';
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
      <BooleanCell column={{ id: 'spec.bool' }} record={{ spec: { bool: true } }} value={true} />
      <DateCell
        column={{ id: 'spec.date' }}
        record={{ spec: { date: Date.now() / 1000 } }}
        value={String(Date.now() / 1000)}
      />
      <br />
      <DescriptionCell
        column={{ id: 'spec.description', hierarchy: DescriptionHierarchy.SECONDARY }}
        record={{ spec: { description: 'Descriptive text in the column' } }}
        value={'Descriptive text in the column'}
      />
      <br />
      <Icon name="arrowLaunch" kind={IconKind.ACCENT} size={24} />
      <br />
      <LinkCell
        column={{ id: 'spec.link', url: 'https://www.google.com' }}
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
      <MultiCell
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
      {/* The project type will not be directly exposed to the @michelangelo/core package. */}
      {/* eslint-disable-next-line @typescript-eslint/dot-notation */}
      Project Name: {data?.project?.metadata?.['name']}
    </div>
  );
}
