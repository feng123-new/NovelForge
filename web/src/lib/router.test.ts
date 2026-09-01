import { describe, expect, it } from 'vitest';
import { parseRoute } from './router';

describe('router', () => {
  it('parses known routes and query values', () => {
    const route = parseRoute('#/chapters?project=opaque-1');
    expect(route.name).toBe('chapters');
    expect(route.query.get('project')).toBe('opaque-1');
  });

  it('falls back to the dashboard for unknown paths', () => {
    expect(parseRoute('#/not-real').name).toBe('dashboard');
  });
});
