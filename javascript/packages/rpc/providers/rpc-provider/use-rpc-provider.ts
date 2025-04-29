import { useContext } from 'react';

import { RpcContext } from './rpc-context';

export function useRpcProvider() {
  return useContext(RpcContext);
}
