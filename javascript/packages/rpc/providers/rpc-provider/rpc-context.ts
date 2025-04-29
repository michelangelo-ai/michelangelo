import { createContext } from 'react';

import { RpcContextType } from './types';

export const RpcContext = createContext<RpcContextType>({});
