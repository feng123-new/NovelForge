import pathlib,sys,subprocess,json,re,urllib.parse
root=pathlib.Path(sys.argv[1]);changed=set()
def edit(path,old,new,count=1):
 p=root/path;s=p.read_text();assert s.count(old)==count,(path,s.count(old));p.write_text(s.replace(old,new));changed.add(path)
p='internal/authoring/store.go'
edit(p,'raw, _ := json.Marshal(m.Rules)','rules := *m.Rules\n\t\trules.Phrases = append([]string{},rules.Phrases...)\n\t\tfor i := range rules.Phrases { rules.Phrases[i]=strings.TrimSpace(rules.Phrases[i]) }\n\t\traw, _ := json.Marshal(rules)')
p='internal/authoring/store_test.go'
s=(root/p).read_text();s+='''
func TestAuthoringEmptyPhraseRulesAndNoCanonicalWrites(t *testing.T) {
 s,_,_:=setup(t)
 rules:=authoring.DefaultRules();rules.Phrases=nil
 result,err:=s.Mutate(t.Context(),"empty-rules",authoring.Mutation{ExpectedRevision:1,Rules:&rules})
 if err!=nil {t.Fatal(err)}
 state,err:=s.State(t.Context(),"",10,0)
 if err!=nil||state.Rules.Phrases==nil||len(state.Rules.Phrases)!=0 {t.Fatal("rules must serialize an empty array, never break UI rendering",state.Rules,err)}
 rules.Phrases=[]string{"  不由得  "}
 if _,err=s.Mutate(t.Context(),"trim-rules",authoring.Mutation{ExpectedRevision:result.Revision,Rules:&rules});err!=nil{t.Fatal(err)}
 state,err=s.State(t.Context(),"",10,0)
 if err!=nil||state.Rules.Phrases[0]!="不由得" {t.Fatal("configured literal was not normalized",err)}
 e:=entry("knowledge","张三在青云宗获得玄铁剑")
 if _,err=s.Mutate(t.Context(),"reference",authoring.Mutation{ExpectedRevision:state.Revision,Entry:&e});err!=nil{t.Fatal(err)}
 var count int
 if err=s.DB.QueryRowContext(t.Context(),"SELECT (SELECT count(*) FROM truth_events)+(SELECT count(*) FROM foreshadow_events)+(SELECT count(*) FROM secret_events)+(SELECT count(*) FROM chapter_versions)").Scan(&count);err!=nil||count!=0{t.Fatal("reference edits wrote story authority",count,err)}
}
''';(root/p).write_text(s);changed.add(p)
p='internal/server/authoring_test.go'
s=(root/p).read_text();s+='''
type tinyCraftModel struct{ craftCapture }
func (m *tinyCraftModel) WriterContextTokens() int { return 64 }
func TestAuthoringMandatoryBudgetStopsBeforeModel(t *testing.T) {
 model:=&tinyCraftModel{craftCapture{payloads:map[string][]byte{}}}
 s,err:=New(Config{Workspace:t.TempDir(),QualityModel:model});if err!=nil{t.Fatal(err)};defer s.Close()
 p,err:=s.projects.Create(t.Context(),project.CreateInput{Title:"Bounded craft"});if err!=nil{t.Fatal(err)}
 store,err:=s.projects.OpenAuthoring(t.Context(),p.ID);if err!=nil{t.Fatal(err)};defer store.Close()
 e:=authoring.Entry{Kind:"skill",Role:"writing",Title:"Large instruction",Markdown:strings.Repeat("Write precise action. ",500),Enabled:true}
 if _,err=store.Mutate(t.Context(),"large-skill",authoring.Mutation{ExpectedRevision:1,Entry:&e});err!=nil{t.Fatal(err)}
 c,cleanup,failure:=s.qualityCoordinator(httptest.NewRequest(http.MethodGet,"/",nil),p.ID);if failure!=nil{t.Fatal(failure)};defer cleanup()
 var plan qualitygate.ChapterPlan;if err=json.Unmarshal(apiPlan(1),&plan);err!=nil{t.Fatal(err)}
 if _,err=c.Writer.Write(t.Context(),qualitygate.WriterRequest{ProjectID:p.ID,Chapter:1,TransactionID:"budget-test",Plan:plan});err==nil{t.Fatal("mandatory overflow did not fail closed")}
 if model.count!=0{t.Fatal("oversized context reached model")}
}
''';(root/p).write_text(s);changed.add(p)
p='docs/AUTHORING.md'
s=(root/p).read_text();s+='''

### Delivery evidence and input normalization

The initial product `6b0d0d852dd0d48bbaab1c5fa823606c3f2850e6` (tree `e05aa2363e70128cac0e56b86987d6e14ba6100c`) passed Actions `33967464713`, job `101309986739`: Go 1.25.5 no-CGO entry builds, eight named Go tests, five tests across three focused frontend files, Svelte/TypeScript with zero errors/warnings, and exact Vite asset generation. The new feature's request test captures the actual model-call payload; the retained Phase 9 two-chapter test also exercises a configured fake HTTP Provider. No paid model was called.

Final review normalizes an omitted/null phrase list to `[]` and trims phrase whitespace before storage; this keeps the API response renderable and literal checks consistent. Additional named checks cover this behavior, absence of canonical writes and compiler rejection before a model call. Final checked commit/run/merge identifiers are recorded in the delivery PR rather than assumed here.

Existing pinned requests and cached Foundation outputs are not automatically regenerated after edits. Newly created planning/writing scopes use the current library; regenerate a cached Foundation only through its explicit new-request workflow. A deleted library entry can remain in immutable historical request selections and operation records. These records are not a secure-erasure facility.
''';(root/p).write_text(s);changed.add(p)
for p in sorted(changed):
 if p.endswith('.go'):subprocess.run(['gofmt','-w',str(root/p)],check=True)
subprocess.run(['git','add','--']+sorted(changed),cwd=root,check=True)
# Static repository hygiene: case-insensitive checkout names and local Markdown file targets.
paths=subprocess.check_output(['git','ls-files'],cwd=root,text=True).splitlines();seen={};collisions=[]
for name in paths:
 folded=name.casefold()
 if folded in seen:collisions.append([seen[folded],name])
 seen[folded]=name
missing=[]
for name in paths:
 if not name.endswith('.md'):continue
 text=(root/name).read_text(errors='replace');text=re.sub(r'```.*?```','',text,flags=re.S)
 for match in re.finditer(r'!?\[[^\]]*\]\(([^\s)]+)(?:\s+[^)]*)?\)',text):
  target=match.group(1).strip('<>');parsed=urllib.parse.urlsplit(target)
  if parsed.scheme or target.startswith(('#','/')):continue
  local=urllib.parse.unquote(parsed.path)
  if local and not (root/name).parent.joinpath(local).exists():missing.append([name,target])
print('STATIC_AUTHORING='+json.dumps({'casefold_collisions':collisions,'missing_local_markdown_targets':missing}))
assert not collisions and not missing
subprocess.run(['git','diff','--cached','--check'],cwd=root,check=True)
