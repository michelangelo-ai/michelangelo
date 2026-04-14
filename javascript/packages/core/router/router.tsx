import { Route, Routes } from 'react-router-dom-v5-compat';

import { MainViewContainer } from '#core/components/views/main-view-container';
import { ProjectDetail } from '#core/components/views/project/project-detail';
import { ProjectList } from '#core/components/views/project/project-list';
import { Sandbox } from '#core/components/views/sandbox/sandbox';
import { CATEGORIES } from '#core/config/categories';
import { DATA_PHASE } from '#core/config/phases/data';
import { DEPLOY_PHASE } from '#core/config/phases/deploy';
import { TRAIN_PHASE } from '#core/config/phases/train';
import { EntityDetailRoute } from './entity-detail-route';
import { PhaseListRoute } from './phase-list-route';
import { StudioBar } from './studio-bar';

const PROJECT_PHASES = [DATA_PHASE, TRAIN_PHASE, DEPLOY_PHASE];

export function Router() {
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
      <Route element={<StudioBar categories={CATEGORIES} />}>
        <Route
          path=":projectId"
          element={
            <MainViewContainer>
              <ProjectDetail phases={PROJECT_PHASES} />
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
