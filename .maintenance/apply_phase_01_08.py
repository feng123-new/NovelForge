#!/usr/bin/env python3
"""One-shot helper for an isolated worktree; never included in product commits."""
import hashlib, json, pathlib, shutil, subprocess, sys
BASE='d8309f2de31b1ccee4f514c99f764837f3e9e270'
source=pathlib.Path(__file__).resolve().parents[1]
root=pathlib.Path(sys.argv[1]).resolve()
assert subprocess.check_output(['git','rev-parse','HEAD'],cwd=root,text=True).strip()==BASE
assert not subprocess.check_output(['git','status','--porcelain'],cwd=root,text=True).strip()
touched=[]
def edit(path, sha, replacements):
    p=root/path
    data=p.read_bytes()
    actual=hashlib.sha1(b'blob '+str(len(data)).encode()+b'\0'+data).hexdigest()
    if sha: assert actual==sha,(path,actual,sha)
    text=data.decode()
    for old,new,count in replacements:
        assert text.count(old)==count,(path,old,text.count(old),count)
        text=text.replace(old,new)
    p.write_text(text)
    touched.append(path)
def one(path,sha,old,new): edit(path,sha,[(old,new,1)])

one('internal/contextcompiler/fts.go','09bb45fb6666d003263771ec5232cb4e186d90d6','\tlimit := 20\n\tquery := quoteFTSQuery(request.Query)','\tif containsHan(request.Query) {\n\t\treturn s.collectCharacters(ctx, request)\n\t}\n\tlimit := 20\n\tquery := quoteFTSQuery(request.Query)')
one('internal/project/context_migration.go','204181a4ed36f2e51c1d6204cfde3d6823a124e6','projectMigrations = append(projectMigrations, contextcompiler.Migration())','projectMigrations = append(projectMigrations, contextcompiler.Migration(), contextcompiler.CharacterSearchMigration())')
one('internal/contextcompiler/fts_test.go','f24a65e40e99ecb725c2800bede56ee50d5a0672','Migrations: []migrate.Migration{Migration()}','Migrations: []migrate.Migration{Migration(), CharacterSearchMigration()}')
one('docker-compose.yml','919c0432a666339a51b862b76efc75d103393e30','"48090:48090"','"127.0.0.1:48090:48090"')
edit('internal/bootstrap/config.go','ba65c559e82577d6f4e140092c08fd48a72d5a5f',[
 ('"writer":            true,','"writer":            true,\n\t"librarian":         true,',1),
 ('valid: architect/writer/editor/import_segment/import_analyze/import_synthesize','valid: architect/writer/librarian/editor/import_segment/import_analyze/import_synthesize',1)])
edit('cmd/novelforge/server.go','b6380831ff4340acb09db31e8c072be8a59b0c13',[
 ('"github.com/voocel/ainovel-cli/internal/server"','"github.com/voocel/ainovel-cli/internal/server"\n\t"github.com/voocel/ainovel-cli/internal/compat"',1),
 ('workspace := flags.String("workspace", ".", "directory containing NovelForge or ainovel projects")','workspace := flags.String("workspace", ".", "directory containing NovelForge or ainovel projects")\n\tconfigPath := flags.String("config", "", "explicit server model configuration file")',1),
 ('Usage: novelforge server [--host HOST] [--port PORT] [--workspace DIR]','Usage: novelforge server [--host HOST] [--port PORT] [--workspace DIR] [--config FILE]',1),
 ('\tapp, err := server.New(server.Config{','\tif *configPath != "" {\n\t\tif err := compat.SetExplicitConfigPath(*configPath); err != nil {\n\t\t\tfmt.Fprintln(os.Stderr, "server: invalid explicit configuration path")\n\t\t\treturn 2\n\t\t}\n\t}\n\tapp, err := server.New(server.Config{',1),
 ('\t\tVersion:   versionInfo().Version,','\t\tVersion:   versionInfo().Version,\n\t\tQualityConfigEnabled: true,\n\t\tQualityConfigPath: compat.ExplicitConfigPath(),',1)])
one('internal/server/server.go','41a8edc21ece48e3527b7e2285f53219ad3b6f18','\tQualityModel      qualitygate.ModelInvoker','\t// Explicit injection is retained for embedders; CLI enables per-project config.\n\tQualityConfigEnabled bool\n\tQualityConfigPath string\n\tQualityModel      qualitygate.ModelInvoker')
edit('internal/server/quality.go','c16e7b00f177dd307e74845c9271884b56c95901',[
 ('func (s *Server) qualityConfigured() bool {\n\treturn (s.cfg.QualityWriter != nil && s.cfg.QualityLibrarian != nil && s.cfg.QualityEditor != nil) || s.cfg.QualityModel != nil\n}\n\n','',1),
 ('\tif s.cfg.QualityModel != nil {\n\t\tinvoker := qualitygate.ModelInvoker(s.cfg.QualityModel)','\tmodel := s.cfg.QualityModel\n\tif model == nil && (writer == nil || librarian == nil || editor == nil) {\n\t\tvar modelErr error\n\t\tmodel, modelErr = s.configuredQualityModel(r.Context(), projectID)\n\t\tif modelErr != nil {\n\t\t\tcleanup()\n\t\t\tfailure := qualityUnavailable()\n\t\t\treturn nil, func() {}, &failure\n\t\t}\n\t}\n\tif model != nil {\n\t\tinvoker := qualitygate.ModelInvoker(model)',1),
 ('writer = qualitygate.ModelWriterService{Caller: caller}','contextTokens := 25000\n\t\t\tif budgeted, ok := model.(interface{ WriterContextTokens() int }); ok {\n\t\t\t\tcontextTokens = budgeted.WriterContextTokens()\n\t\t\t}\n\t\t\twriter = qualitygate.ModelWriterService{Caller: caller, Context: s.projects, ContextTokens: contextTokens}',1),
 ('Actions: qualityActions{Generate: s.qualityConfigured()}','Actions: qualityActions{Generate: s.qualityConfigured(projectID)}',1),
 ('configured := s.qualityConfigured()','configured := s.qualityConfigured(snapshot.Transaction.ProjectID)',1),
 ('if !s.qualityConfigured() {','if !s.qualityConfigured(projectID) {',2),
 ('if requiresAgents && !s.qualityConfigured() {','if requiresAgents && !s.qualityConfigured(projectID) {',1)])
edit('internal/qualitygate/model_services.go','b3f76bf5fddf9e014c709a0a8fa15c35b094fd19',[
 ('type ModelWriterService struct {\n\tCaller *IdempotentModelCaller\n}','type ModelWriterService struct {\n\tCaller *IdempotentModelCaller\n\tContext WriterContextCompiler\n\tContextTokens int\n}',1),
 ('\tpayload, err := json.Marshal(struct {\n\t\tPlan          ChapterPlan','\tvar compiled json.RawMessage\n\tif s.Context != nil {\n\t\tvar err error\n\t\tcompiled, err = s.Context.CompileWriterContext(ctx, req, s.ContextTokens)\n\t\tif err != nil {\n\t\t\treturn WriterResult{}, fmt.Errorf("compile writer context: %w", err)\n\t\t}\n\t}\n\tpayload, err := json.Marshal(struct {\n\t\tPlan          ChapterPlan',1),
 ('\t\tFeedback      []string    `json:"feedback,omitempty"`\n\t}{req.Plan, req.PreviousDraft, req.Feedback})','\t\tFeedback      []string    `json:"feedback,omitempty"`\n\t\tCompiledContext json.RawMessage `json:"compiled_context,omitempty"`\n\t}{req.Plan, req.PreviousDraft, req.Feedback, compiled})',1)])
old='''func finalizeContextPayload(result map[string]any, chapter, budget int) (json.RawMessage, error) {
	if err := trimByBudget(result, budget); err != nil {
		return nil, err
	}
	result["_loading_summary"] = buildLoadingSummary(result, chapter)
	if err := trimByBudget(result, budget); err != nil {
		return nil, err
	}
	result["_loading_summary"] = buildLoadingSummary(result, chapter)
	attachContextCompilerDiagnostics(result, chapter, budget)

	data, err := json.Marshal(result)
	if err != nil {
		return nil, fmt.Errorf("marshal context payload: %w", err)
	}
	if len(data) > budget {
		return nil, fmt.Errorf("context payload exceeds budget after summary rebuild: size=%d budget=%d", len(data), budget)
	}
	return data, nil
}'''
new='''func finalizeContextPayload(result map[string]any, chapter, budget int) (json.RawMessage, error) {
    return finalizeCompiledContextPayload(context.Background(), result, chapter, budget)
}

func finalizeCompiledContextPayload(ctx context.Context, result map[string]any, chapter, budget int) (json.RawMessage, error) {
    if budget <= 0 { return nil, fmt.Errorf("context byte budget must be positive") }
    tokenBudget := budget / 4
    if tokenBudget < 1 { tokenBudget = 1 }
    // Only the compiler trims. Mandatory errors never fall back to raw data.
    for attempt := 0; attempt < 4; attempt++ {
        compiled, err := contextcompiler.CompileLegacyMap(ctx, result, contextcompiler.Request{
            Chapter: chapter, TotalTokens: tokenBudget, RecentChapterCount: 3,
            Budget: contextcompiler.DefaultBudgetConfig(),
        }, nil)
        if err != nil { return nil, fmt.Errorf("compile context: %w", err) }
        selected, err := contextcompiler.SelectLegacyMap(result, compiled)
        if err != nil { return nil, err }
        selected["_loading_summary"] = buildLoadingSummary(selected, chapter)
        selected["_context_compiler"] = map[string]any{
            "version": 2, "status": "applied", "context_sha": compiled.ContextSHA,
            "total_tokens": compiled.Diagnostics.TotalTokens,
            "used_tokens": compiled.Diagnostics.UsedTokens,
            "system_tokens": compiled.Diagnostics.SystemTokens,
            "layers": compiled.Diagnostics.Layers,
        }
        data, err := json.Marshal(selected)
        if err != nil { return nil, fmt.Errorf("marshal context payload: %w", err) }
        if len(data) <= budget { return data, nil }
        reduction := (len(data)-budget+1)/2
        if reduction < 256 { reduction = 256 }
        tokenBudget -= reduction
        if tokenBudget < 1 { break }
    }
    return nil, fmt.Errorf("compiled context and diagnostics exceed byte budget %d", budget)
}'''
edit('internal/tools/novel_context.go','bbe5849e80ef555941656cf9ac3cce31ae47b38d',[
 ('func (t *ContextTool) Execute(_ context.Context, args json.RawMessage)','func (t *ContextTool) Execute(ctx context.Context, args json.RawMessage)',1),
 ('return finalizeContextPayload(result, a.Chapter, budget)','return finalizeCompiledContextPayload(ctx, result, a.Chapter, budget)',1),
 (old,new,1)])
# Existing documentation is read at the fixed worktree commit before modification.
one('docs/IMPLEMENTATION_STATUS.md',None,'# NovelForge Implementation Status\n','# NovelForge Implementation Status\n\n> Historical acceptance archive: prior PR/SHA/CI records are unchanged. Current Phase 1–8 maintenance changes and limited validation are recorded in [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md). Phase 9–13 remain frozen.\n')
one('docs/WEB.md',None,'# NovelForge Web Workspace\n','# NovelForge Web Workspace\n\n> Maintenance update (2026-09-05): the CLI server now loads project/global quality model configuration and accepts `server --config`. Writer requests include compiled project context. Provider availability is a configuration condition, not a health probe. Workers remain unavailable. See [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md) for current scope and verification limits.\n')
one('docs/CONTEXT_COMPILER.md',None,'# Context Compiler and Hybrid Retrieval\n','# Context Compiler and Hybrid Retrieval\n\n> Maintenance update (2026-09-05): `novel_context` now returns compiler-selected records, and Web Writer hashes include compiled project context. Compilation errors stop the call. Additive Migration 8 supports character search without changing Migration 5. Older diagnostic-only integration notes below describe historical Phase 7 delivery; see [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md).\n')
one('docs/PHASE_01_08_REVIEW.md',None,'# Phase 1–8 逻辑链复核与维护范围\n','# Phase 1–8 逻辑链复核与维护范围\n\n> 本文保留 5dbcadfa 基线的静态复核结果。后续五项源码修复与有限验证见 [PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md)；不要将以下历史“未修复”状态当作新提交状态。Phase 9–13 仍冻结。\n')
edit('docs/ROADMAP.md',None,[
 ('Active work is limited to consolidating Phase 1–8:', 'Active work is limited to fixing and consolidating Phase 1–8:',1),
 ('[PHASE_01_08_REVIEW.md](PHASE_01_08_REVIEW.md) is the current maintenance checklist.', '[PHASE_01_08_FIXES.md](PHASE_01_08_FIXES.md) records current fixes and limited verification. [PHASE_01_08_REVIEW.md](PHASE_01_08_REVIEW.md) preserves the original maintenance review.',1),
 ('Current focus: distinguish existing CLI configuration support from the missing default Web quality-model injection.', 'Current focus: default Web quality services now use project/global configuration and server --config; preserve configuration isolation.',1),
 ('Delivered module; default Web connection incomplete','Delivered module; default Web configuration connected',1),
 ('Current focus: supply the model services through the normal startup/configuration path without weakening validation or allowing a blocking FAIL to finalize.', 'Current focus: model services are connected through normal startup/configuration without weakening validation or allowing a blocking FAIL to finalize; verify affected paths only.',1),
 ('Delivered module; input integration incomplete','Delivered module; selected-input integration repaired',1),
 ('Current focus: the existing `novel_context` still returns the legacy payload with compiler diagnostics. Trace how actual Writer input will consume the compiler result and real project-scoped providers. Chinese FTS recall is a targeted follow-up, not a completed acceptance result.', 'Current focus: legacy output now contains only selected records, and Web Writer receives compiled project context. Additive character FTS supports Chinese substring terms. Verify selected inputs and small retrieval samples, not full-book scale.',1)])
copy_paths='''README.md
docs/PHASE_01_08_FIXES.md
internal/contextcompiler/character_search.go
internal/contextcompiler/character_search_test.go
internal/contextcompiler/legacy_selection.go
internal/contextcompiler/legacy_selection_test.go
internal/project/runtime_config.go
internal/project/writer_context.go
internal/qualitygate/writer_context.go
internal/qualitygate/writer_context_test.go
internal/server/qualityruntime/runtime.go
internal/server/qualityruntime/runtime_test.go
internal/server/runtime_quality.go
internal/server/runtime_quality_test.go
cmd/novelforge/server_config_test.go'''.splitlines()
for path in copy_paths:
    target=root/path
    if path!='README.md': assert not target.exists(),path
    target.parent.mkdir(parents=True,exist_ok=True)
    shutil.copyfile(source/path,target)
    touched.append(path)
subprocess.run(['gofmt','-w']+[str(root/p) for p in touched if p.endswith('.go')],check=True)
assert len(touched)==len(set(touched))
subprocess.run(['git','add','--']+touched,cwd=root,check=True)
actual=subprocess.check_output(['git','diff','--cached','--name-only'],cwd=root,text=True).splitlines()
assert set(actual)==set(touched),(actual,touched)
subprocess.run(['git','diff','--cached','--check'],cwd=root,check=True)
print(json.dumps({'base':BASE,'changed_files':actual},indent=2))
