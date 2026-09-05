export interface AutopilotJob {
  id: string;
  project_id: string;
  state: 'pending' | 'running' | 'paused' | 'retrying' | 'failed' | 'completed' | 'cancelled';
  stage: string;
  chapter: number;
  completed_through: number;
  start_chapter: number;
  target_chapter: number;
  review_every: number;
  max_rewrites: number;
  max_retries: number;
  retries: number;
  control: string;
  error_code: string;
  revision: number;
  review_candidate_id?: string;
  updated_at: string;
  next_run: string;
  actions: { pause: boolean; stop: boolean; resume: boolean };
}
export interface AutopilotPage { jobs: AutopilotJob[]; worker_available: boolean; model_available: boolean; limit: number; offset: number; next_chapter?: number }
export interface AutopilotStart { start_chapter: number; target_chapter: number; review_every?: number; max_rewrites?: number; max_retries?: number }
export interface AutopilotDetail { job: AutopilotJob; foundation: unknown; chapter_plan: unknown; candidate_text: string; candidate_id: string; quality: unknown }

export interface AutopilotApproval { expected_revision?: number; review_candidate_id?: string }
