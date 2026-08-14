import { Route, Routes } from 'react-router-dom-v5-compat';

import { MainViewContainer } from '#core/components/views/main-view-container';
import { ProjectDetail } from '#core/components/views/project/project-detail';
import { ProjectList } from '#core/components/views/project/project-list';
import { Sandbox } from '#core/components/views/sandbox/sandbox';
import { useStudioConfig } from '#core/providers/config-provider/use-studio-config';
import { EntityDetailRoute } from './entity-detail-route';
import { PhaseListRoute } from './phase-list-route';
import { StudioBar } from './studio-bar';

export function Router() {
  const { categories } = useStudioConfig();
  const phases = categories.flatMap((c) => c.phases);

  return (
    <Routes>
      <Route
        index
        element={
          <MainViewContainer>
            <ProjectList />
          </MainViewContainer>
        }
      />
      <Route
        path="sandbox"
        element={
          <MainViewContainer>
            <Sandbox />
          </MainViewContainer>
        }
      />
      <Route element={<StudioBar categories={categories} />}>
        <Route
          path=":projectId"
          element={
            <MainViewContainer>
              <ProjectDetail phases={phases} />
            </MainViewContainer>
          }
        />
        <Route
          path=":projectId/:phase/:entity/:entityId/:entityTab?"
          element={
            <MainViewContainer>
              <EntityDetailRoute />
            </MainViewContainer>
          }
        />
        <Route
          path=":projectId/:phase/:entity?"
          element={
            <MainViewContainer>
              <PhaseListRoute />
            </MainViewContainer>
          }
        />
      </Route>
    </Routes>
  );
}
