export type AuthoringKind = 'skill' | 'style' | 'knowledge';
export type SkillRole = 'writing' | 'review' | 'polish' | 'planning';
export interface AuthoringEntry { id: string; kind: AuthoringKind; role: string; title: string; markdown: string; source: string; enabled: boolean; pinned: boolean; priority: number; from_chapter: number; pov: string }
export interface AuthoringRules { enabled: boolean; phrases: string[]; max_phrase_occurrences: number; max_sentence_repeats: number; min_sentence_runes: number; previous_chapters: number }
export interface AuthoringState { revision: number; entries: AuthoringEntry[]; builtins: AuthoringEntry[]; rules: AuthoringRules; total: number; limit: number; offset: number }
export interface AuthoringMutation { expected_revision: number; entry?: AuthoringEntry; delete_id?: string; rules?: AuthoringRules }
export interface AuthoringChange { revision: number; entry_id?: string; replayed: boolean }
export interface AuthoringSearch { entries: AuthoringEntry[]; limit: number; offset: number }
export interface RuleFinding { code: string; phrase: string; count: number; limit: number; message: string }
export interface AuthoringLint { revision: number; report: { findings: RuleFinding[]; advisory: boolean; truncated: boolean } }
export function emptyEntry(kind: AuthoringKind): AuthoringEntry { return {id:'',kind,role:kind==='skill'?'writing':'',title:'',markdown:'',source:'',enabled:true,pinned:false,priority:50,from_chapter:0,pov:''}; }
