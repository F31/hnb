import { getRouterManager } from './RouterManager'

export type { RouterManager } from './RouterManager'
export { getRouterManager }

export function createAppRouter() {
  return getRouterManager().getRouter()
}
