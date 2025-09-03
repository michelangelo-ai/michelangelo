import type { DetailPageConfig } from '#core/components/views/detail-view/types/detail-view-schema-types';

export interface PageRendererProps<T extends object = object> {
  page: DetailPageConfig<T>;
  data: T | undefined;
  isLoading: boolean;
}
