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

  // The project type will not be directly exposed to the @michelangelo/core package.
  // eslint-disable-next-line @typescript-eslint/dot-notation
  return <div>Project Name: {data?.project?.metadata?.['name']}</div>;
}
