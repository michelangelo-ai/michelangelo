import { Route, Routes } from 'react-router';
import { BrowserRouter } from 'react-router-dom';

import { MainViewContainer } from '@/components/views/main-view-container';
import { ProjectDetail } from '@/components/views/project/project-detail';

export function Router() {
  return (
    <BrowserRouter basename={'/'}>
      <Routes>
        <Route
          path={':projectId'}
          element={
            <MainViewContainer hasBreadcrumb={false}>
              <ProjectDetail />
            </MainViewContainer>
          }
        />
      </Routes>
    </BrowserRouter>
  );
}
