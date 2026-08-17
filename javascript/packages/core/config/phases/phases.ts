import { DATA_PHASE } from './data';
import { DEPLOY_PHASE } from './deploy';
import { MONITOR_DEBUG_PHASE } from './monitor-debug';
import { RETRAIN_PHASE } from './retrain';
import { TRAIN_PHASE } from './train';

export const PHASES = {
  data: DATA_PHASE,
  train: TRAIN_PHASE,
  deploy: DEPLOY_PHASE,
  retrain: RETRAIN_PHASE,
  'monitor-debug': MONITOR_DEBUG_PHASE,
};
