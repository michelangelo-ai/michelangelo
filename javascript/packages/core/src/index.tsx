import { useEffect, useState } from 'react';
import { createClient, type Interceptor } from '@connectrpc/connect';
import { createGrpcWebTransport } from '@connectrpc/connect-web';
import { AppNavBar } from 'baseui/app-nav-bar';

import { ProjectService } from '@ma/gen-api/v2/project_svc_pb';
import { MainViewContainer } from '@/components/views/main-view-container';
import { ThemeProvider } from '@/themes/provider';

import type { Project } from '@ma/gen-api/v2/project_pb';

import '@/styles/main.css';

export function CoreApp() {
  const [projects, setProjects] = useState<Project[]>([]);

  useEffect(() => {
    const listProjects = async () => {
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
      const projectResponse = await client.listProject({});
      setProjects(projectResponse.projectList?.items ?? []);
    };

    listProjects().catch((error) => {
      console.error('Error listing projects:', error);
    });
  }, []);

  return (
    <ThemeProvider>
      <AppNavBar title="Michelangelo Studio" />
      <MainViewContainer>Hi! {projects[0]?.metadata?.name}</MainViewContainer>
    </ThemeProvider>
  );
}
