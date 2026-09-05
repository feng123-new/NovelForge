import type { ProjectDetail } from './types';

export interface ManuscriptImport {
 id: string; project_id: string; filename: string; source_sha: string;
 start_chapter: number; total: number; saved: number; analyzed: number;
 next_save: number; next_analysis: number; created_at: string;
}
export interface ImportedChapter {
 chapter: number; title: string; characters: number; version_id: string;
 state: 'pending' | 'saved' | 'analyzed'; error_code?: string;
}
export interface ImportCollection { imports: ManuscriptImport[]; limit: number; offset: number; model_available: boolean }
export interface ImportDetail { import: ManuscriptImport; chapters: ImportedChapter[]; limit: number; offset: number }
export interface RestoreResult { project: ProjectDetail; replayed: boolean; requires_configuration: boolean; jobs_resumed: false }
export interface MigrationResult { from_format: number; to_format: number; schema_version: number; backup_id: string; changed: boolean }
