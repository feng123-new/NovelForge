export type ForeshadowStatus =
  | 'planned'
  | 'planted'
  | 'reinforced'
  | 'revealed'
  | 'abandoned'
  | 'overdue'

export type ForeshadowPriority = 'critical' | 'high' | 'normal' | 'low'

export interface Foreshadow {
  id: string
  key: string
  title: string
  description?: string
  priority: ForeshadowPriority
  status: Exclude<ForeshadowStatus, 'overdue'>
  effective_status: ForeshadowStatus
  planted_chapter?: number | null
  due_chapter?: number | null
  reveal_chapter?: number | null
  updated_chapter: number
}

export interface Secret {
  id: string
  key: string
  title: string
  description?: string
  status: 'hidden' | 'hinted' | 'revealed' | 'retired'
  public_from_chapter?: number | null
  public: boolean
  holders: string[]
  updated_chapter: number
}

export interface Page<T> {
  items: T[]
  total: number
  limit: number
  offset: number
  next_offset?: number | null
}

export interface LedgerDashboard {
  chapter: number
  foreshadows_total: number
  foreshadows_active: number
  foreshadows_critical: number
  foreshadows_overdue: number
  foreshadows_upcoming: number
  secrets_total: number
  secrets_public: number
  secrets_hidden: number
}

const priorityRank: Record<ForeshadowPriority, number> = {
  critical: 0,
  high: 1,
  normal: 2,
  low: 3,
}

export function effectiveForeshadowStatus(
  status: Exclude<ForeshadowStatus, 'overdue'>,
  dueChapter: number | null | undefined,
  chapter: number,
): ForeshadowStatus {
  if (
    (status === 'planned' || status === 'planted' || status === 'reinforced') &&
    dueChapter !== null &&
    dueChapter !== undefined &&
    dueChapter < chapter
  ) {
    return 'overdue'
  }
  return status
}

export function compareForeshadows(left: Foreshadow, right: Foreshadow): number {
  if (left.effective_status !== right.effective_status) {
    if (left.effective_status === 'overdue') return -1
    if (right.effective_status === 'overdue') return 1
  }
  const priority = priorityRank[left.priority] - priorityRank[right.priority]
  if (priority !== 0) return priority
  const leftDue = left.due_chapter ?? Number.MAX_SAFE_INTEGER
  const rightDue = right.due_chapter ?? Number.MAX_SAFE_INTEGER
  if (leftDue !== rightDue) return leftDue - rightDue
  return left.key.localeCompare(right.key)
}

export function secretPublicAt(publicFromChapter: number | null | undefined, chapter: number): boolean {
  return publicFromChapter !== null && publicFromChapter !== undefined && publicFromChapter <= chapter
}

export function newIdempotencyKey(scope: string): string {
  const random = globalThis.crypto?.randomUUID?.() ?? `${Date.now()}-${Math.random()}`
  return `${scope}-${random}`
}
