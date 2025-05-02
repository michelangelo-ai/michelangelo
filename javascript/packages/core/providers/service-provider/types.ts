/**
 * @description
 * The service context provided to the application to connect to the services injected
 * into the application.
 *
 * @remarks
 * Since the available requestIds are injected into the application, the parameters and
 * return types are unknown.
 */
export type ServiceContextType = {
  request: (requestId: string, args: unknown) => Promise<unknown>;
};
