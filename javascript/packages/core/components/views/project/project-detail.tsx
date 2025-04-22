import { useEffect } from 'react';
import { useState } from 'react';
import { createClient, Interceptor } from '@connectrpc/connect';
import { createGrpcWebTransport } from '@connectrpc/connect-web';
import { useStudioParams } from '#core/hooks/routing/use-studio-params/use-studio-params';
import { Project } from '#gen-api/v2/project_pb';
import { ProjectService } from '#gen-api/v2/project_svc_pb';

export function ProjectDetail() {
  const [project, setProject] = useState<Project>();
  const { projectId } = useStudioParams('base');

  useEffect(() => {
    const getProject = async () => {
      const callerInterceptor: Interceptor = (next) => async (req) => {
        req.header.set('context-Ttl-Ms', '10000');
        req.header.set('grpc-timeout', '1000000m');
        req.header.set('Rpc-Caller', 'ma-studio');
        req.header.set('Rpc-Encoding', 'proto');
        req.header.set('Rpc-Service', 'ma-apiserver');

        return await next(req);
      };

      const transport = createGrpcWebTransport({
        baseUrl: 'http://localhost:8081',
        interceptors: [callerInterceptor],
        useBinaryFormat: true,
      });

      const client = createClient(ProjectService, transport);
      const projectResponse = await client.getProject({
        name: projectId,
        namespace: projectId,
      });
      setProject(projectResponse.project);
    };

    getProject().catch((error) => {
      console.error('Error getting project:', error);
    });
  }, [projectId]);

  return <div>{project?.metadata?.name}</div>;
}
