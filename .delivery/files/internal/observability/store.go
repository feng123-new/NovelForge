package observability

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "strings"
 "time"
)

type queryer interface { QueryRowContext(context.Context,string,...any) *sql.Row }
func readState(ctx context.Context,q queryer) (State,error) {var out State;var raw string;err:=q.QueryRowContext(ctx,`SELECT revision,payload_json FROM observation_policy WHERE id=1`).Scan(&out.Revision,&raw);if err!=nil{return out,err};out.Policy=DefaultPolicy();if err=json.Unmarshal([]byte(raw),&out.Policy);err!=nil{return out,err};out.Policy.Normalize();return out,out.Policy.Validate()}
func (s *Store) State(ctx context.Context) (State,error) {return readState(ctx,s.DB)}
func (s *Store) Bind(ctx context.Context,id Identity) error {_,err:=s.DB.ExecContext(ctx,`INSERT INTO observation_links(call_key,logical_id) VALUES(?,?) ON CONFLICT(call_key) DO NOTHING`,id.LogicalKey,digest(id.LogicalKey));return err}
func (s *Store) Replay(ctx context.Context,key string) {_,_=s.DB.ExecContext(ctx,`INSERT INTO observation_replays(logical_id,count) VALUES(?,1) ON CONFLICT(logical_id) DO UPDATE SET count=count+1`,digest(key))}

func (s *Store) begin(ctx context.Context,id Identity,provider,model string,inputEstimate int,boundary string) (*Ticket,error) {
 if s==nil||s.DB==nil||id.LogicalKey==""||id.RequestHash==""||id.TransactionID==""{return nil,gate("OBSERVATION_IDENTITY_INVALID")}
 provider,model=Label(provider),Label(model);if id.TaskID==""{id.TaskID="manual"}
 tx,err:=s.DB.BeginTx(ctx,nil);if err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")};defer tx.Rollback()
 // Acquire SQLite's write lock before reading quotas: admissions cannot race.
 if _,err=tx.ExecContext(ctx,`UPDATE observation_policy SET revision=revision WHERE id=1`);err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")}
 state,err:=readState(ctx,tx);if err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")};p:=state.Policy
 if inputEstimate<0||inputEstimate>p.MaxInputEstimate{return nil,gate("INPUT_ESTIMATE_LIMIT")}
 for _,v:=range p.PausedProviders{if v==provider{return nil,gate("PROVIDER_PAUSED")}}
 rows,err:=tx.QueryContext(ctx,`SELECT state,ended_at FROM observation_attempts WHERE project_id=? AND provider=? AND state!='pending' ORDER BY seq DESC LIMIT ?`,s.ProjectID,provider,p.FailureThreshold);if err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")}
 consecutive:=0;latest:=time.Time{};for rows.Next(){var st string;var end sql.NullString;if err=rows.Scan(&st,&end);err!=nil{break};if consecutive==0&&end.Valid{latest,_=time.Parse(time.RFC3339Nano,end.String)};if st!="failed"{break};consecutive++};readErr:=rows.Err();rows.Close();if err!=nil||readErr!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")}
 if consecutive>=p.FailureThreshold&&s.now().Before(latest.Add(time.Duration(p.CooldownSeconds)*time.Second)){return nil,gate("PROVIDER_COOLDOWN")}
 totals,err:=readTotals(ctx,tx,` WHERE project_id=?`,[]any{s.ProjectID});if err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")}
 if totals.Pending>0{return nil,gate("UNRESOLVED_MODEL_ATTEMPT")}
 budgeted:=p.ProjectBudgetMicros>0||p.TaskBudgetMicros>0
 if (p.BlockUnknownCost||budgeted)&&totals.UnknownCost>0{return nil,gate("COST_RECONCILIATION_REQUIRED")}
 if p.ProjectMaxCalls>0&&totals.Calls>=p.ProjectMaxCalls{return nil,gate("PROJECT_CALL_LIMIT")}
 task,err:=readTotals(ctx,tx,` WHERE project_id=? AND task_id=?`,[]any{s.ProjectID,id.TaskID});if err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")}
 if p.TaskMaxCalls>0&&task.Calls>=p.TaskMaxCalls{return nil,gate("TASK_CALL_LIMIT")}
 rate:=p.price(provider,model);if rate==nil&&(p.RequireKnownPrice||budgeted){return nil,gate("MODEL_PRICE_REQUIRED")}
 reserve:=estimateCost(inputEstimate,p.MaxOutputTokens,rate);reserved:=int64(0);if reserve!=nil{reserved=*reserve}
 if p.ProjectBudgetMicros>0&&totals.CostMicros+totals.ReservedMicros+reserved>p.ProjectBudgetMicros{return nil,gate("PROJECT_BUDGET_EXCEEDED")}
 if p.TaskBudgetMicros>0&&task.CostMicros+task.ReservedMicros+reserved>p.TaskBudgetMicros{return nil,gate("TASK_BUDGET_EXCEEDED")}
 attemptID,err:=opaqueID();if err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")}
 a:=Attempt{ID:attemptID,LogicalID:digest(id.LogicalKey),RequestHash:id.RequestHash,TaskID:Label(id.TaskID),TransactionID:Label(id.TransactionID),Chapter:id.Chapter,Agent:Label(id.Agent),Operation:Label(id.Operation),Provider:provider,Model:model,State:"pending",StartedAt:s.now(),InputEstimate:inputEstimate,OutputLimit:p.MaxOutputTokens,Currency:p.Currency,PriceRevision:state.Revision,Price:rate,ReservedMicros:reserved,UsageSource:"unknown",CostSource:"unknown",Boundary:boundary}
 raw,err:=json.Marshal(a);if err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")}
 _,err=tx.ExecContext(ctx,`INSERT INTO observation_attempts(id,project_id,task_id,logical_id,chapter,agent,operation,provider,model,state,started_at,reserved_micros,payload_json) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,a.ID,s.ProjectID,a.TaskID,a.LogicalID,a.Chapter,a.Agent,a.Operation,a.Provider,a.Model,a.State,a.StartedAt.Format(time.RFC3339Nano),reserved,string(raw));if err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")};if err=tx.Commit();err!=nil{return nil,gate("OBSERVATION_STORAGE_FAILED")};if s.Notify!=nil{s.Notify("attempt_started",a.ID)};return &Ticket{s,a},nil
}

const totalColumns=`count(*),coalesce(sum(state='completed'),0),coalesce(sum(state='failed' OR state='abandoned'),0),coalesce(sum(state='pending'),0),coalesce(sum(cost_micros IS NULL AND state!='pending'),0),coalesce(sum(json_extract(payload_json,'$.input_tokens') IS NULL),0),coalesce(sum(json_extract(payload_json,'$.input_tokens')),0),coalesce(sum(json_extract(payload_json,'$.output_tokens')),0),coalesce(sum(cost_micros),0),coalesce(sum(reserved_micros),0)`
func totalTargets(t *Totals) []any{return []any{&t.Calls,&t.Completed,&t.Failed,&t.Pending,&t.UnknownCost,&t.UnknownUsage,&t.InputTokens,&t.OutputTokens,&t.CostMicros,&t.ReservedMicros}}
func readTotals(ctx context.Context,q queryer,where string,args []any) (Totals,error){var t Totals;err:=q.QueryRowContext(ctx,`SELECT `+totalColumns+` FROM observation_attempts`+where,args...).Scan(totalTargets(&t)...);return t,err}
func (s *Store) Page(ctx context.Context,task string,chapter,limit,offset int) (Page,error) {
 out:=Page{Groups:[]Group{},Attempts:[]Attempt{},FilterTask:task,FilterChapter:chapter};if limit<1||limit>100||offset<0||offset>1000000||chapter<0||chapter>1000||len(task)>128{return out,ErrInvalid};out.Limit=limit;out.Offset=offset
 state,err:=s.State(ctx);if err!=nil{return out,err};out.State=state
 where:=` WHERE project_id=?`;args:=[]any{s.ProjectID};if task!=""{where+=` AND task_id=?`;args=append(args,task)};if chapter>0{where+=` AND chapter=?`;args=append(args,chapter)}
 out.Totals,err=readTotals(ctx,s.DB,where,args);if err!=nil{return out,err};out.Total=out.Totals.Calls
 rows,err:=s.DB.QueryContext(ctx,`SELECT agent,provider,model,`+totalColumns+` FROM observation_attempts`+where+` GROUP BY agent,provider,model ORDER BY agent,provider,model LIMIT 500`,args...);if err!=nil{return out,err}
 for rows.Next(){var g Group;dest:=append([]any{&g.Agent,&g.Provider,&g.Model},totalTargets(&g.Totals)...);if err=rows.Scan(dest...);err!=nil{break};out.Groups=append(out.Groups,g)};readErr:=rows.Err();rows.Close();if err!=nil{return out,err};if readErr!=nil{return out,readErr}
 pageArgs:=append(append([]any{},args...),limit,offset);rows,err=s.DB.QueryContext(ctx,`SELECT payload_json FROM observation_attempts`+where+` ORDER BY seq DESC LIMIT ? OFFSET ?`,pageArgs...);if err!=nil{return out,err}
 for rows.Next(){var raw string;var a Attempt;if err=rows.Scan(&raw);err!=nil{break};if err=json.Unmarshal([]byte(raw),&a);err!=nil{break};out.Attempts=append(out.Attempts,a)};readErr=rows.Err();rows.Close();if err!=nil{return out,err};if readErr!=nil{return out,readErr}
 // Replay/legacy figures are project-wide, explicitly separate from filters.
 if err=s.DB.QueryRowContext(ctx,`SELECT coalesce(sum(count),0) FROM observation_replays`).Scan(&out.Replays);err!=nil{return out,err}
 err=s.DB.QueryRowContext(ctx,`SELECT count(*) FROM model_calls m WHERE NOT EXISTS(SELECT 1 FROM observation_links l WHERE l.call_key=m.idempotency_key)`).Scan(&out.LegacyCalls)
 return out,err
}

type Reconciliation struct { AttemptID string `json:"attempt_id"`; CostMicros int64 `json:"cost_micros"`; Confirm bool `json:"confirm"` }
type Mutation struct { ExpectedRevision int64 `json:"expected_revision"`; Policy *Policy `json:"policy,omitempty"`; Reconcile *Reconciliation `json:"reconcile,omitempty"` }
type Change struct { Revision int64 `json:"revision"`; Policy *Policy `json:"policy,omitempty"`; Before *Attempt `json:"before,omitempty"`; After *Attempt `json:"after,omitempty"` }
// Mutate requires the caller's project execution lock. Unknown attempts can only
// be reconciled explicitly; the observed preimage is retained in an immutable log.
func (s *Store) Mutate(ctx context.Context,key string,m Mutation) (Change,error) {
 out:=Change{};if len(key)<1||len(key)>128||m.ExpectedRevision<1||(m.Policy==nil)==(m.Reconcile==nil){return out,ErrInvalid};request,_:=json.Marshal(m);hash:=digest(string(request))
 tx,err:=s.DB.BeginTx(ctx,nil);if err!=nil{return out,err};defer tx.Rollback();if _,err=tx.ExecContext(ctx,`UPDATE observation_policy SET revision=revision WHERE id=1`);err!=nil{return out,err}
 var oldHash,oldResult string;err=tx.QueryRowContext(ctx,`SELECT request_hash,result_json FROM observation_changes WHERE idempotency_key=?`,key).Scan(&oldHash,&oldResult);if err==nil{if oldHash!=hash{return out,ErrConflict};err=json.Unmarshal([]byte(oldResult),&out);return out,err};if !errors.Is(err,sql.ErrNoRows){return out,err}
 state,err:=readState(ctx,tx);if err!=nil{return out,err};if state.Revision!=m.ExpectedRevision{return out,ErrConflict};out.Revision=state.Revision+1
 if m.Policy!=nil {p:=*m.Policy;p.Normalize();if err=p.Validate();err!=nil{return out,err};var n int;if err=tx.QueryRowContext(ctx,`SELECT count(*) FROM observation_attempts`).Scan(&n);err!=nil{return out,err};if n>0&&p.Currency!=state.Policy.Currency{return out,ErrConflict};raw,_:=json.Marshal(p);_,err=tx.ExecContext(ctx,`UPDATE observation_policy SET revision=?,payload_json=? WHERE id=1`,out.Revision,string(raw));out.Policy=&p
 } else {r:=m.Reconcile;if !r.Confirm||r.CostMicros<0||r.CostMicros>1000000000000{return out,ErrInvalid};var raw string;err=tx.QueryRowContext(ctx,`SELECT payload_json FROM observation_attempts WHERE id=? AND project_id=?`,r.AttemptID,s.ProjectID).Scan(&raw);if errors.Is(err,sql.ErrNoRows){return out,ErrNotFound};if err!=nil{return out,err};var a Attempt;if err=json.Unmarshal([]byte(raw),&a);err!=nil{return out,err};if a.CostMicros!=nil||a.State=="abandoned"{return out,ErrConflict};before:=a;out.Before=&before;if a.State=="pending"{a.State="abandoned";now:=s.now();a.EndedAt=&now;a.ErrorCode="MANUALLY_RECONCILED_UNKNOWN"};a.CostMicros=&r.CostMicros;a.CostSource="manual_reconciliation";a.ReservedMicros=0;out.After=&a;encoded,_:=json.Marshal(a);_,err=tx.ExecContext(ctx,`UPDATE observation_attempts SET state=?,cost_micros=?,reserved_micros=0,ended_at=?,error_code=?,payload_json=? WHERE id=?`,a.State,a.CostMicros,a.EndedAt,a.ErrorCode,string(encoded),a.ID);if err==nil{_,err=tx.ExecContext(ctx,`UPDATE observation_policy SET revision=? WHERE id=1`,out.Revision)} }
 if err!=nil{return Change{},err};raw,err:=json.Marshal(out);if err!=nil{return Change{},err};_,err=tx.ExecContext(ctx,`INSERT INTO observation_changes VALUES(?,?,?,?)`,key,hash,string(raw),s.now().Format(time.RFC3339Nano));if err!=nil{return Change{},err};if err=tx.Commit();err!=nil{return Change{},err};if s.Notify!=nil{s.Notify("policy_changed","")};return out,nil
}

type Finding struct { Code string `json:"code"`; Severity string `json:"severity"`; Count int `json:"count"`; Chapter int `json:"chapter,omitempty"`; TaskID string `json:"task_id,omitempty"`; Message string `json:"message"`; Action string `json:"action"` }
func Explain(code string) (string,string) {
 switch code {
 case "PROJECT_BUDGET_EXCEEDED","TASK_BUDGET_EXCEEDED": return "下一次请求的预计费用超过预算。已保存内容不受影响。","核对调用记录，调整预算后显式继续任务。"
 case "PROJECT_CALL_LIMIT","TASK_CALL_LIMIT":return "已达到配置的模型尝试次数上限。","检查重试原因，再调整上限或停止任务。"
 case "MODEL_PRICE_REQUIRED":return "当前模型没有对应单价，无法进行预算检查。","填写服务商别名和精确模型名对应的价格。"
 case "UNRESOLVED_MODEL_ATTEMPT","COST_RECONCILIATION_REQUIRED":return "存在未完成记录或未知费用；不会把它当作零费用继续调用。","先暂停任务并核对服务商记录，明确确认未知请求的费用后继续。"
 case "PROVIDER_PAUSED","PROVIDER_COOLDOWN":return "服务商已被手动暂停或近期连续失败。","检查配置，等待冷却或解除暂停；不会自动发起付费探测。"
 case "INPUT_ESTIMATE_LIMIT":return "本次请求的估算输入超过安全上限。","检查上下文和资料规模，勿直接绕过必需信息检查。"
 case "OBSERVATION_STORAGE_FAILED":return "调用记录或预算状态无法可靠保存。","检查磁盘和数据库，保留现有成果；不要删除历史后重试。"
 case "REVIEW_REQUIRED":return "等待人工审阅当前候选稿。","打开 Autopilot 详情，查看当前版本后批准。"
 case "CHAPTER_CONTEXT_CHANGED","CONTEXT_BASELINE_REQUIRED":return "当前计划依赖的事实或资料版本已变化。","在 Versions 中检查、接受并定稿保留内容后继续；不要强制覆盖。"
 case "PLANNING_CONTEXT_FAILED","AUTHORING_CONTEXT_FAILED":return "规划所需上下文未能安全编译。","核对必需事实、视角及上下文预算。"
 case "EXISTING_DRAFT_REQUIRES_REVIEW","IMPORT_REQUIRES_REVIEW":return "发现已有未接受的正文或导入候选。","恢复原任务，或通过 Versions 明确审核和定稿。"
 case "QUALITY_HOLD","CONTINUITY_REWRITE_EXHAUSTED","REWRITE_BUDGET_EXHAUSTED":return "质量门禁或重写上限阻止继续。","检查一致性和评审，不得用成本设置绕过质量门禁。"
 default:return "任务保留了可检查的错误状态。","查看任务详情和对应版本；提交问题时使用脱敏诊断包。"
 }
}
func (s *Store) Findings(ctx context.Context) ([]Finding,error) {
 out:=[]Finding{}
 checks:=[]struct{code,sql,message,action string}{
 {"UNRESOLVED_MODEL_ATTEMPT",`SELECT count(*) FROM observation_attempts WHERE state='pending'`,"存在执行中或中断后未确认的模型请求。","运行中请等待；确认进程停止后核对费用，勿直接重复调用。"},
 {"UNKNOWN_COST",`SELECT count(*) FROM observation_attempts WHERE state!='pending' AND cost_micros IS NULL`,"部分请求费用未知，已知费用小计不是完整账单。","核对价格和用量，必要时进行明确对账。"},
 {"INCOMPLETE_FINAL",`SELECT count(*) FROM chapter_finalize_sagas WHERE state!='completed'`,"存在尚未完整提交的定稿。","恢复原定稿操作，不能仅凭 Active Final 判断完成。"},
 {"DERIVED_STATE_PENDING",`SELECT count(*) FROM derived_state_rebuilds WHERE state!='completed'`,"派生状态等待重建，摘要或检索可能不是最新版本。","通过版本恢复/重建流程处理，再继续写作。"},
 {"UNRESOLVED_TRUTH_CONFLICT",`SELECT count(*) FROM truth_conflicts WHERE status='unresolved'`,"事实库保留了未解决冲突。","核对来源和章节范围，通过显式事实修正处理。"},
 {"QUALITY_HOLD",`SELECT count(*) FROM chapter_transactions WHERE state='hold' OR state='failed'`,"章节质量流程处于阻断状态。","检查候选稿、一致性结果与重写上限。"},
 }
 for _,c:=range checks{var n int;if err:=s.DB.QueryRowContext(ctx,c.sql).Scan(&n);err!=nil{return nil,fmt.Errorf("diagnostic query: %w",err)};if n>0{out=append(out,Finding{Code:c.code,Severity:"warning",Count:n,Message:c.message,Action:c.action})}}
 return out,nil
}
// Redact excludes free-form configuration notes and all content. Identifiers and
// provider/model labels are hashed because even a user-chosen alias can be secret.
func Redact(p Page) map[string]any {
 attempts:=make([]map[string]any,0,len(p.Attempts));for _,a:=range p.Attempts{attempts=append(attempts,map[string]any{"id":digest(a.ID),"logical_id":a.LogicalID,"task":digest(a.TaskID),"chapter":a.Chapter,"agent":a.Agent,"operation":a.Operation,"provider":digest(a.Provider),"model":digest(a.Model),"state":a.State,"started_at":a.StartedAt,"ended_at":a.EndedAt,"usage_source":a.UsageSource,"cost_source":a.CostSource,"input_tokens":a.InputTokens,"output_tokens":a.OutputTokens,"cost_micros":a.CostMicros,"error_code":a.ErrorCode,"boundary":a.Boundary})}
 return map[string]any{"schema":1,"currency":p.State.Policy.Currency,"totals":p.Totals,"legacy_untracked_calls":p.LegacyCalls,"replays":p.Replays,"attempts":attempts,"truncated":p.Total>len(attempts),"coverage":"Web/Autopilot Generate boundary; provider-internal retries and old TUI are not independently counted","excludes":[]string{"prose","prompts","credentials","paths","configuration","free-form notes"}}
}

var _ = strings.TrimSpace
