// Display mappings only. Never use these labels as API/database values.
export type Pair = readonly [string, string];
export const labels: Record<string, Pair> = {
  unknown: ['未知', 'Unknown'], ok: ['正常', 'Healthy'], ready: ['就绪', 'Ready'], available: ['可用', 'Available'], unavailable: ['不可用', 'Unavailable'],
  connected: ['已连接', 'Connected'], connecting: ['连接中', 'Connecting'], reconnecting: ['重新连接中', 'Reconnecting'], disconnected: ['已断开', 'Disconnected'], unsupported: ['不支持', 'Unsupported'],
  pending: ['待处理', 'Pending'], not_started: ['尚未开始', 'Not started'], running: ['运行中', 'Running'], paused: ['已暂停', 'Paused'], retrying: ['重试中', 'Retrying'], failed: ['失败', 'Failed'], completed: ['已完成', 'Completed'], cancelled: ['已停止', 'Cancelled'],
  pause: ['暂停', 'Pause'], stop: ['停止', 'Stop'], resume: ['继续', 'Resume'], retry: ['重试', 'Retry'], save: ['保存', 'Save'], check: ['检查', 'Check'], accept: ['接受', 'Accept'], reject: ['拒绝', 'Reject'], restore: ['恢复', 'Restore'], sync: ['同步', 'Sync'],
  foundation: ['创作基础规划', 'Story foundation'], plan: ['章节规划', 'Chapter planning'], chapter_plan: ['章节计划', 'Chapter plan'], planning: ['规划', 'Planning'], generate: ['生成', 'Generate'], writing: ['写作', 'Writing'], writer: ['写作', 'Writer'], architect: ['创作规划', 'Architect'], planner: ['章节规划', 'Planner'], librarian: ['事实整理', 'Librarian'], continuity: ['一致性检查', 'Continuity'], editor: ['审稿', 'Editor'], arbiter: ['协调', 'Arbiter'], review: ['审阅', 'Review'], polish: ['润色', 'Polish'], rewrite: ['重写', 'Rewrite'], finalize: ['定稿', 'Finalize'], done: ['完成', 'Done'], waiting: ['等待中', 'Waiting'],
  draft: ['草稿', 'Draft'], final: ['定稿', 'Final'], continuity_fix: ['一致性修订', 'Continuity revision'], editor_revision: ['审稿修订', 'Editor revision'], human_revision: ['人工修订', 'Human revision'], human: ['人工', 'Human'], model: ['模型', 'Model'], generated: ['模型生成', 'Generated'], accepted: ['已接受', 'Accepted'], rejected: ['已拒绝', 'Rejected'],
  human_final: ['人工定稿', 'Human final'], generated_final: ['模型定稿', 'Generated final'], generated_final_chapter: ['模型定稿章节', 'Generated final chapter'], current_chapter_plan: ['当前章节计划', 'Current chapter plan'], arc_plan: ['情节弧计划', 'Arc plan'], volume_plan: ['卷计划', 'Volume plan'], story_compass: ['故事方向', 'Story compass'], llm_suggestion: ['模型建议', 'Model suggestion'],
  pass: ['通过', 'Pass'], warn: ['警告', 'Warning'], fail: ['未通过', 'Fail'], hold: ['等待处理', 'On hold'], info: ['提示', 'Information'], warning: ['警告', 'Warning'], error: ['错误', 'Error'], severe: ['严重', 'Severe'], blocking: ['阻断', 'Blocking'],
  low: ['低', 'Low'], medium: ['中', 'Medium'], high: ['高', 'High'], critical: ['关键', 'Critical'], normal: ['普通', 'Normal'],
  planned: ['计划中', 'Planned'], planted: ['已埋下', 'Planted'], progressing: ['推进中', 'Progressing'], resolved: ['已回收', 'Resolved'], abandoned: ['已放弃', 'Abandoned'], contradicted: ['存在矛盾', 'Contradicted'], overdue: ['已逾期', 'Overdue'],
  private: ['未公开', 'Private'], public: ['已公开', 'Public'], skill: ['写作技能', 'Skill'], style: ['风格', 'Style'], knowledge: ['参考资料', 'Reference'],
  copilot: ['人机协作', 'Human collaboration'], autopilot: ['自动创作', 'Automatic writing'], every_chapter: ['每章审阅', 'Every chapter'], every_n: ['每隔若干章审阅', 'Every N chapters'], full_automatic: ['全自动', 'Fully automatic'],
  imported: ['已导入', 'Imported'], saved: ['已保存', 'Saved'], analyzed: ['已分析', 'Analyzed'], staged: ['已暂存', 'Staged'], analyzing: ['分析中', 'Analyzing'], importing: ['导入中', 'Importing'],
  healthy: ['正常', 'Healthy'], degraded: ['状态异常', 'Degraded'], cooldown: ['冷却中', 'Cooling down'], cooling_down: ['冷却中', 'Cooling down'], manually_paused: ['手动暂停', 'Manually paused'],
  started: ['已开始', 'Started'], succeeded: ['成功', 'Succeeded'], success: ['成功', 'Success'], failed_unknown: ['失败或结果未知', 'Failed or unknown'], unknown_outcome: ['结果未知', 'Unknown outcome'],
  provider: ['服务商返回', 'Provider reported'], price_table: ['价格表估算', 'Price-table estimate'], price_snapshot: ['价格快照估算', 'Price snapshot'], estimated: ['估算', 'Estimated'], manual: ['手动操作', 'Manual'], reconciled: ['人工对账', 'Reconciled'], manual_reconciliation: ['人工对账', 'Manual reconciliation'], free: ['明确免费', 'Explicitly free'], none: ['无', 'None'],
  stale: ['已过期', 'Stale'], invalidated: ['已失效', 'Invalidated'], rebuilding: ['重建中', 'Rebuilding'], sync_required: ['需要同步', 'Sync required'], clean: ['已同步', 'Synchronized'],
  true: ['是', 'Yes'], false: ['否', 'No'],
  'server.ready': ['本地服务已就绪', 'Local server ready'], 'autopilot.changed': ['自动创作任务已更新', 'Writing job updated'], 'observability.changed': ['诊断与费用已更新', 'Diagnostics and cost updated'],
  'project.created': ['项目已创建', 'Project created'], 'project.updated': ['项目已更新', 'Project updated'], 'project.deleted': ['项目已删除', 'Project deleted'], 'project.archived': ['项目已归档', 'Project archived'], 'project.unarchived': ['项目已恢复', 'Project unarchived'],
  truth_store: ['事实存储', 'Truth store'], narrative_ledger: ['叙事账本', 'Narrative ledger'], context_compiler: ['上下文编译', 'Context compiler'], chapter_versions: ['章节版本', 'Chapter versions'], quality_gate: ['质量检查', 'Quality gate'],
  projects: ['项目管理', 'Projects'], chapters: ['章节管理', 'Chapters'], models: ['模型目录', 'Models'], foundation_request: ['基础规划请求', 'Foundation request'], foundation_requests: ['基础规划请求', 'Foundation requests'],
  authoring: ['写作技能与资料库', 'Writing skills and libraries'], lifecycle: ['导入导出与备份', 'Import, export and backup'], observability: ['诊断与费用', 'Diagnostics and cost'], diagnostics: ['诊断', 'Diagnostics'],
  quality_model: ['质量模型服务', 'Quality model service'], model_available: ['模型已配置', 'Model configured'], worker_available: ['任务执行器可用', 'Worker available'], autopilot_worker: ['自动创作执行器', 'Automatic writing worker'],
  sse: ['实时事件', 'Live events'], events: ['事件流', 'Event stream'], web: ['网页工作台', 'Web workspace'], web_ui: ['网页工作台', 'Web workspace'], import: ['导入', 'Import'], export: ['导出', 'Export'], backup: ['备份', 'Backup'],
  REVIEW_REQUIRED: ['等待人工审阅', 'Review required'],
};
export const errorCodes: Record<string, Pair> = {
  UNKNOWN: ['操作未完成，请检查项目状态、配置和诊断记录后重试。', 'The operation did not complete. Check project state, configuration and diagnostics before retrying.'],
  INVALID_INPUT: ['请求参数无效，请检查必填项、章节范围和输入格式。', 'Invalid request. Check required fields, chapter ranges and input format.'],
  NOT_FOUND: ['所选项目或记录不存在，请刷新列表后重新选择。', 'The selected project or record was not found. Refresh the list and select it again.'],
  CONFLICT: ['当前状态已变化或存在操作冲突，请刷新后检查任务和版本状态。', 'State changed or the operation conflicts with current state. Refresh and inspect job/version status.'],
  UNAVAILABLE: ['所需服务暂不可用，请检查服务端模型配置和任务执行器。', 'The required service is unavailable. Check server-side model configuration and the task worker.'],
  REVIEW_REQUIRED: ['请先查看当前候选稿，明确批准后再继续定稿。', 'Inspect the current candidate and explicitly approve it before continuing finalization.'],
  PROJECT_NOT_FOUND: ['项目不存在，请刷新项目列表。', 'Project not found. Refresh the project list.'],
  PROJECT_ARCHIVED: ['项目已归档，请先取消归档再修改。', 'This project is archived. Unarchive it before editing.'],
  PROJECT_BUSY: ['项目正在执行任务，请先暂停任务并等待当前步骤结束。', 'The project is busy. Pause its job and let the current step finish.'],
  AUTOPILOT_BUSY: ['自动创作任务尚未停止，请先暂停并等待当前步骤结束。', 'Automatic writing is active. Pause and wait for the current step to finish.'],
  AUTOPILOT_ACTIVE: ['项目已有未结束任务，请先处理该任务。', 'This project already has an unfinished job. Resolve it first.'],
  WORKER_UNAVAILABLE: ['任务执行器未运行，请按部署指南启用后再启动任务。', 'The task worker is not running. Enable it as described in the deployment guide.'],
  MODEL_UNAVAILABLE: ['模型服务未配置或不可用，请检查服务端配置。', 'The model service is not configured or unavailable. Check server configuration.'],
  QUALITY_MODEL_UNAVAILABLE: ['质量模型服务不可用，请配置事实整理和审稿模型。', 'Quality model services are unavailable. Configure the librarian and editor models.'],
  BUDGET_EXCEEDED: ['费用预算已用尽，请在诊断与费用页面核对后调整预算。', 'The cost budget is exhausted. Review and adjust it in Diagnostics and cost.'],
  UNKNOWN_COST: ['存在未知费用，请暂停任务并核对服务商账单。', 'Unknown costs remain. Pause the job and reconcile provider records.'],
  PRICE_REQUIRED: ['缺少模型价格，请在诊断与费用页面填写准确单价。', 'Model prices are missing. Enter exact prices in Diagnostics and cost.'],
  PROVIDER_PAUSED: ['服务商已暂停，请检查暂停策略后手动恢复。', 'The provider is paused. Review the pause policy before resuming manually.'],
  PROVIDER_COOLDOWN: ['服务商处于冷却期，请检查连续失败原因后重试。', 'The provider is cooling down. Inspect consecutive failures before retrying.'],
  IDEMPOTENCY_CONFLICT: ['请求标识与既有请求不一致，请刷新状态后重新操作。', 'The request identity conflicts with an earlier request. Refresh state before trying again.'],
  STALE_REVISION: ['编辑基于旧修订，请保留输入并载入最新状态后再保存。', 'This edit is based on an old revision. Preserve your input and reload current state before saving.'],
  SYNC_REQUIRED: ['检测到外部文件修改，请先同步并重新检查。', 'External file changes were detected. Sync and rerun checks first.'],
  CONTINUITY_FAILED: ['一致性检查未通过，请修订阻断问题后重新检查。', 'Continuity checks failed. Fix blocking issues and run checks again.'],
  FOUNDATION_REQUIRED: ['尚无创作基础规划请求，请先完成新建小说向导。', 'No foundation request exists. Complete the new-novel wizard first.'],
};
export const serverMessages: Record<string, Pair> = {
  'not_started': ['尚未开始', 'Not started'], 'ready': ['就绪', 'Ready'],
};

Object.assign(labels, {
  "plan_context": [
    "规划上下文",
    "Planning context"
  ],
  "active": [
    "进行中",
    "Active"
  ],
  "drafting": [
    "生成草稿中",
    "Drafting"
  ],
  "draft_ready": [
    "草稿就绪",
    "Draft ready"
  ],
  "librarian_pending": [
    "等待事实整理",
    "Fact extraction pending"
  ],
  "facts_proposed": [
    "已提出事实建议",
    "Facts proposed"
  ],
  "continuity_pending": [
    "等待一致性检查",
    "Continuity check pending"
  ],
  "continuity_checked": [
    "一致性检查完成",
    "Continuity checked"
  ],
  "editor_pending": [
    "等待审稿",
    "Editor review pending"
  ],
  "reviewed": [
    "审稿完成",
    "Reviewed"
  ],
  "rewrite_pending": [
    "等待重写",
    "Rewrite pending"
  ],
  "final_candidate": [
    "定稿候选",
    "Final candidate"
  ],
  "truth_commit_pending": [
    "等待提交权威事实",
    "Fact commit pending"
  ],
  "checkpoint_pending": [
    "等待保存检查点",
    "Checkpoint pending"
  ],
  "last_attempt_succeeded": [
    "最近请求成功",
    "Last attempt succeeded"
  ],
  "in_flight_or_interrupted": [
    "执行中或中断待确认",
    "In progress or interrupted"
  ],
  "last_attempt_failed_or_unknown": [
    "最近请求失败或结果未知",
    "Last attempt failed or outcome unknown"
  ],
  "rate_card_estimate": [
    "价格快照估算",
    "Price snapshot estimate"
  ],
  "review_or_replan": [
    "审阅或重新规划",
    "Review or replan"
  ],
  "project_lifecycle": [
    "项目生命周期",
    "Project lifecycle"
  ],
  "durable_events": [
    "持久化事件",
    "Persistent events"
  ],
  "formal_web_workspace": [
    "网页工作台",
    "Web workspace"
  ],
  "foundation_request_storage": [
    "基础规划请求存储",
    "Foundation request storage"
  ],
  "foundation_worker_available": [
    "基础规划执行器可用",
    "Foundation worker available"
  ],
  "autopilot_worker_available": [
    "自动创作执行器可用",
    "Automatic writing worker available"
  ],
  "chapter_quality_gate": [
    "章节质量检查",
    "Chapter quality checks"
  ],
  "chapter_inline_diff": [
    "章节行内比较",
    "Inline chapter comparison"
  ],
  "chapter_side_by_side_diff": [
    "章节并排比较",
    "Side-by-side chapter comparison"
  ],
  "human_edit_sync": [
    "人工修改同步",
    "Human edit synchronization"
  ],
  "chapter_boundary_rebuild": [
    "章节边界重建",
    "Chapter boundary rebuild"
  ],
  "quality_model_available": [
    "质量模型服务可用",
    "Quality model service available"
  ],
  "quality_max_rewrites": [
    "质量重写次数上限",
    "Maximum quality rewrites"
  ],
  "quality_threshold": [
    "审稿评分阈值",
    "Review score threshold"
  ],
  "credentials_exposed_to_web": [
    "向网页暴露凭据",
    "Credentials exposed to Web"
  ],
  "authoritative_state_is_server": [
    "服务端保存权威状态",
    "Server holds authoritative state"
  ],
  "project.duplicated": [
    "项目已复制",
    "Project duplicated"
  ],
  "chapter.version.created": [
    "章节版本已创建",
    "Chapter version created"
  ],
  "chapter.finalized": [
    "章节已定稿",
    "Chapter finalized"
  ],
  "foundation.requested": [
    "基础规划请求已保存",
    "Foundation request saved"
  ]
});

Object.assign(errorCodes, {
  "WORKSPACE_UNAVAILABLE": [
    "工作区暂不可用，请检查本地服务和工作区目录。",
    "The workspace is unavailable. Check the local server and workspace directory."
  ],
  "LEDGER_UNAVAILABLE": [
    "叙事账本暂不可用，请检查项目数据库后重试。",
    "The narrative ledger is unavailable. Check the project database before retrying."
  ],
  "SECRET_STORE_UNAVAILABLE": [
    "秘密存储暂不可用，请检查项目数据库后重试。",
    "The secret store is unavailable. Check the project database before retrying."
  ],
  "INVALID_RESPONSE": [
    "服务端响应格式无效，请检查服务版本和诊断记录。",
    "The server returned an invalid response. Check the server version and diagnostics."
  ],
  "CLIENT_ERROR": [
    "请求未完成，请检查本地服务连接后重试。",
    "The request did not complete. Check the local server connection before retrying."
  ],
  "NOT_ALLOWED": [
    "该操作不被允许，请检查项目状态和服务访问配置。",
    "This operation is not permitted. Check project state and server access configuration."
  ],
  "REQUEST_BODY_TOO_LARGE": [
    "提交内容超过大小限制，请减少文件或正文大小后重试。",
    "The submitted content exceeds the size limit. Reduce the file or text size before retrying."
  ],
  "QUALITY_NO_SAFE_CANDIDATE": [
    "没有满足一致性要求的候选稿，请修订并重新检查。",
    "No candidate meets continuity requirements. Revise and rerun checks."
  ],
  "QUALITY_REWRITE_LIMIT": [
    "已达到重写次数上限，请人工审阅候选稿。",
    "The rewrite limit has been reached. Review the candidate manually."
  ],
  "PROJECT_JOB_UNFINISHED": [
    "项目仍有未结束任务，请先暂停、停止或完成任务。",
    "This project has an unfinished job. Pause, stop or complete it first."
  ],
  "LIFECYCLE_LIMIT": [
    "作品文件或章节规模超过限制，请按导入说明拆分后重试。",
    "The manuscript file or chapter size exceeds a limit. Split it according to the import instructions."
  ]
});

Object.assign(serverMessages, {
  "rewrite the draft or explicitly supersede the authoritative fact after acceptance": [
    "修订草稿，或在接受后明确替代原有权威事实",
    "rewrite the draft or explicitly supersede the authoritative fact after acceptance"
  ],
  "review_or_replan": [
    "审阅或重新规划",
    "review_or_replan"
  ],
  "writer requested": [
    "已请求生成草稿",
    "writer requested"
  ],
  "writer failed": [
    "草稿生成失败",
    "writer failed"
  ],
  "draft persistence failed": [
    "草稿保存失败",
    "draft persistence failed"
  ],
  "failure after draft persistence; draft retained": [
    "草稿保存后发生错误，草稿已保留",
    "failure after draft persistence; draft retained"
  ],
  "resume from persisted editor review": [
    "从已保存的审稿结果继续",
    "resume from persisted editor review"
  ],
  "resume from persisted continuity result": [
    "从已保存的一致性结果继续",
    "resume from persisted continuity result"
  ],
  "resume from persisted fact proposal": [
    "从已保存的事实建议继续",
    "resume from persisted fact proposal"
  ],
  "resume fact extraction": [
    "恢复事实提取",
    "resume fact extraction"
  ],
  "extract fact proposal": [
    "提取事实建议",
    "extract fact proposal"
  ],
  "librarian failed; draft retained": [
    "事实整理失败，草稿已保留",
    "librarian failed; draft retained"
  ],
  "fact proposal validation or persistence failed": [
    "事实建议校验或保存失败",
    "fact proposal validation or persistence failed"
  ],
  "fact proposal persisted": [
    "事实建议已保存",
    "fact proposal persisted"
  ],
  "deterministic Chapter-N continuity check": [
    "按当前章节边界执行一致性检查",
    "deterministic Chapter-N continuity check"
  ],
  "continuity service failed; proposal retained": [
    "一致性检查服务失败，事实建议已保留",
    "continuity service failed; proposal retained"
  ],
  "continuity blocks finalization": [
    "一致性检查阻止定稿",
    "continuity blocks finalization"
  ],
  "continuity FAIL with rewrite budget exhausted": [
    "一致性检查未通过，重写次数已用尽",
    "continuity FAIL with rewrite budget exhausted"
  ],
  "literary review": [
    "进行文学质量审稿",
    "literary review"
  ],
  "editor failed; draft, proposal and continuity retained": [
    "审稿失败，草稿、事实建议和一致性结果均已保留",
    "editor failed; draft, proposal and continuity retained"
  ],
  "editor review persisted": [
    "审稿结果已保存",
    "editor review persisted"
  ],
  "continuity accepted and editor score met threshold": [
    "一致性检查通过，审稿评分达到阈值",
    "continuity accepted and editor score met threshold"
  ],
  "continuity accepted and editor score met threshold; WARN allowed by deterministic policy": [
    "一致性结果与审稿评分满足要求，现有策略允许警告",
    "continuity accepted and editor score met threshold; WARN allowed by deterministic policy"
  ],
  "editor score below threshold": [
    "审稿评分低于阈值",
    "editor score below threshold"
  ],
  "no continuity-safe candidate after rewrite limit": [
    "达到重写上限后，仍无满足一致性要求的候选稿",
    "no continuity-safe candidate after rewrite limit"
  ],
  "highest editor score among continuity-safe candidates": [
    "从满足一致性要求的候选稿中选择审稿评分最高者",
    "highest editor score among continuity-safe candidates"
  ],
  "highest editor score among continuity-safe candidates after rewrite limit": [
    "达到重写上限后，从满足一致性要求的候选稿中选择审稿评分最高者",
    "highest editor score among continuity-safe candidates after rewrite limit"
  ],
  "explicit bounded rewrite requested": [
    "已明确请求有限重写",
    "explicit bounded rewrite requested"
  ],
  "rewrite attempt": [
    "尝试重写",
    "rewrite attempt"
  ],
  "rewrite failed; previous drafts retained": [
    "重写失败，旧草稿已保留",
    "rewrite failed; previous drafts retained"
  ],
  "rewrite persistence failed; previous drafts retained": [
    "重写稿保存失败，旧草稿已保留",
    "rewrite persistence failed; previous drafts retained"
  ],
  "finalize refused: no continuity-safe candidate": [
    "拒绝定稿：没有满足一致性要求的候选稿",
    "finalize refused: no continuity-safe candidate"
  ],
  "continuity policy blocks finalization": [
    "一致性策略阻止定稿",
    "continuity policy blocks finalization"
  ],
  "accepted Final candidate committing Truth": [
    "正在提交已接受定稿的权威事实",
    "accepted Final candidate committing Truth"
  ],
  "Truth and chapter file committed; checkpoint pending": [
    "权威事实与章节文件已提交，等待保存检查点",
    "Truth and chapter file committed; checkpoint pending"
  ],
  "Final, Truth and checkpoint committed": [
    "定稿、权威事实和检查点均已提交",
    "Final, Truth and checkpoint committed"
  ]
});
for (const code of ["AUTHORING_INPUT_INVALID", "CHAPTER_BOUNDARY_INVALID", "CHAPTER_VERSION_VALIDATION_FAILED", "IDEMPOTENCY_KEY_INVALID", "IDEMPOTENCY_KEY_REQUIRED", "JOB_INPUT_INVALID", "LAST_EVENT_ID_INVALID", "LEDGER_FILTER_INVALID", "LEDGER_PROJECT_INVALID", "LEDGER_RESOURCE_INVALID", "LEDGER_VALIDATION_FAILED", "LIFECYCLE_INVALID", "OBSERVATION_INVALID", "PAGINATION_INVALID", "QUALITY_CHAPTER_INVALID", "QUALITY_CHAPTER_MISMATCH", "QUALITY_PLAN_INVALID", "REQUEST_BODY_INVALID", "REQUEST_BODY_REQUIRED", "TRUTH_PAGINATION_INVALID", "TRUTH_PROJECT_ID_REQUIRED", "TRUTH_QUERY_INVALID"]) errorCodes[code] = errorCodes.INVALID_INPUT;
for (const code of ["AUTHORING_NOT_FOUND", "JOB_NOT_FOUND", "LEDGER_NOT_FOUND", "LIFECYCLE_NOT_FOUND", "OBSERVATION_NOT_FOUND", "QUALITY_NOT_FOUND", "TRUTH_PROJECT_NOT_FOUND", "API_ROUTE_NOT_FOUND"]) errorCodes[code] = errorCodes.NOT_FOUND;
for (const code of ["AUTHORING_REVISION_CONFLICT", "IDEMPOTENCY_REQUEST_IN_PROGRESS", "JOB_STATE_CONFLICT", "LEDGER_IDEMPOTENCY_CONFLICT", "LEDGER_STATE_CONFLICT", "LIFECYCLE_CONFLICT", "OBSERVATION_CONFLICT", "QUALITY_IDEMPOTENCY_CONFLICT", "QUALITY_STATE_CONFLICT"]) errorCodes[code] = errorCodes.CONFLICT;
for (const code of ["AUTHORING_STORAGE_ERROR", "AUTOPILOT_STORAGE_ERROR", "AUTOPILOT_UNAVAILABLE", "QUALITY_SERVICE_UNAVAILABLE", "TRUTH_STORE_UNAVAILABLE"]) errorCodes[code] = errorCodes.UNAVAILABLE;
for (const code of ["PROJECT_AUTOPILOT_BUSY"]) errorCodes[code] = errorCodes.PROJECT_BUSY;
