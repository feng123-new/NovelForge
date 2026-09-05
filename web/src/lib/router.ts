export type RouteName = 'observability' | 'lifecycle' | 'authoring' | 'autopilot' | 'dashboard' | 'projects' | 'new' | 'chapters' | 'versions' | 'foreshadows' | 'secrets' | 'models' | 'logs' | 'settings';

export interface Route {
  name: RouteName;
  path: string;
  query: URLSearchParams;
}

const knownRoutes: Record<string, RouteName> = {
  '/': 'dashboard',
  '/lifecycle': 'lifecycle',
  '/dashboard': 'dashboard',
 '/observability': 'observability',
  '/projects': 'projects',
  '/autopilot': 'autopilot',
  '/authoring': 'authoring',
  '/new': 'new',
  '/chapters': 'chapters',
  '/versions': 'versions',
  '/foreshadows': 'foreshadows',
  '/secrets': 'secrets',
  '/models': 'models',
  '/logs': 'logs',
  '/settings': 'settings'
};

export function parseRoute(hash: string): Route {
  const raw = hash.replace(/^#/, '') || '/dashboard';
  const [rawPath, rawQuery = ''] = raw.split('?', 2);
  const path = rawPath.startsWith('/') ? rawPath : `/${rawPath}`;
  return {
    name: knownRoutes[path] ?? 'dashboard',
    path: knownRoutes[path] ? path : '/dashboard',
    query: new URLSearchParams(rawQuery)
  };
}

export function currentRoute(): Route {
  return parseRoute(globalThis.location?.hash ?? '#/dashboard');
}

export function navigate(path: string): void {
  const normalized = path.startsWith('/') ? path : `/${path}`;
  globalThis.location.hash = normalized;
}

export function subscribeRoute(callback: (route: Route) => void): () => void {
  const handler = () => callback(currentRoute());
  globalThis.addEventListener('hashchange', handler);
  callback(currentRoute());
  return () => globalThis.removeEventListener('hashchange', handler);
}
