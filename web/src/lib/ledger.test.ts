import { describe, expect, it } from 'vitest'

import {
  compareForeshadows,
  effectiveForeshadowStatus,
  secretPublicAt,
  type Foreshadow,
} from './ledger'

describe('Narrative Ledger temporal helpers', () => {
  it('computes OVERDUE without mutating the stored lifecycle', () => {
    expect(effectiveForeshadowStatus('planted', 2, 3)).toBe('overdue')
    expect(effectiveForeshadowStatus('revealed', 2, 3)).toBe('revealed')
  })

  it('orders overdue and critical obligations deterministically', () => {
    const items: Foreshadow[] = [
      {
        id: 'normal',
        key: 'normal',
        title: 'Normal',
        priority: 'normal',
        status: 'planted',
        effective_status: 'planted',
        due_chapter: 5,
        updated_chapter: 1,
      },
      {
        id: 'critical',
        key: 'critical',
        title: 'Critical',
        priority: 'critical',
        status: 'planned',
        effective_status: 'planned',
        due_chapter: 8,
        updated_chapter: 1,
      },
      {
        id: 'overdue',
        key: 'overdue',
        title: 'Overdue',
        priority: 'low',
        status: 'planted',
        effective_status: 'overdue',
        due_chapter: 1,
        updated_chapter: 1,
      },
    ]
    expect(items.sort(compareForeshadows).map((item) => item.key)).toEqual([
      'overdue',
      'critical',
      'normal',
    ])
  })

  it('evaluates secret public state at Chapter N', () => {
    expect(secretPublicAt(5, 4)).toBe(false)
    expect(secretPublicAt(5, 5)).toBe(true)
    expect(secretPublicAt(null, 99)).toBe(false)
  })
})
