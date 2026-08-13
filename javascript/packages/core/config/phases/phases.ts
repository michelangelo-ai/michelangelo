import { DATA_PHASE } from './data';
import { DEPLOY_PHASE } from './deploy';
import { MONITOR_DEBUG_PHASE } from './monitor-debug';
import { TRAIN_PHASE } from './train';

export const PHASES = {
  data: DATA_PHASE,
  train: TRAIN_PHASE,
  deploy: DEPLOY_PHASE,
  'monitor-debug': MONITOR_DEBUG_PHASE,
};
