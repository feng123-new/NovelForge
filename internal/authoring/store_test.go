package authoring_test

import (
 "errors"
 "strings"
 "testing"

 "github.com/voocel/ainovel-cli/internal/authoring"
 "github.com/voocel/ainovel-cli/internal/project"
)
func setup(t *testing.T)(*authoring.Store,*project.Repository,string){t.Helper();r,err:=project.NewRepository(t.TempDir());if err!=nil{t.Fatal(err)};p,err:=r.Create(t.Context(),project.CreateInput{Title:"Authoring"});if err!=nil{t.Fatal(err)};s,err:=r.OpenAuthoring(t.Context(),p.ID);if err!=nil{t.Fatal(err)};t.Cleanup(func(){s.Close()});return s,r,p.ID}
func entry(kind,text string)authoring.Entry{return authoring.Entry{Kind:kind,Title:"青云宗史料",Markdown:text,Enabled:true,Pinned:true,Priority:50}}
func TestAuthoringCRUDRevisionReplayAndChineseFTS(t *testing.T){
 s,r,_:=setup(t);ctx:=t.Context();e:=entry("knowledge","张三在青云宗获得玄铁剑。参考资料，不是小说事实。")
 m:=authoring.Mutation{ExpectedRevision:1,Entry:&e};created,err:=s.Mutate(ctx,"create",m);if err!=nil{t.Fatal(err)}
 replay,err:=s.Mutate(ctx,"create",m);if err!=nil||!replay.Replayed||replay.Revision!=created.Revision{t.Fatal("replay",replay,err)}
 if _,err=s.Mutate(ctx,"stale",m);!errors.Is(err,authoring.ErrConflict){t.Fatal("stale edit permitted",err)}
 for _,q:=range []string{"张三","青云宗","玄铁剑"}{found,err:=s.Search(ctx,"knowledge",q,1,"",10,0);if err!=nil||len(found)!=1||found[0].Markdown!=e.Markdown{t.Fatal("Chinese FTS",q,found,err)}}
 wrong,err:=s.Search(ctx,"style","张三",1,"",10,0);if err!=nil||len(wrong)!=0{t.Fatal("library kinds mixed",err)}
 p2,err:=r.Create(ctx,project.CreateInput{Title:"Other project"});if err!=nil{t.Fatal(err)};other,err:=r.OpenAuthoring(ctx,p2.ID);if err!=nil{t.Fatal(err)};defer other.Close();found,err:=other.Search(ctx,"knowledge","张三",1,"",10,0);if err!=nil||len(found)!=0{t.Fatal("cross project leakage",err)}
 e.ID=created.EntryID;e.Markdown="李四在白云城寻找古琴。";updated,err:=s.Mutate(ctx,"update",authoring.Mutation{ExpectedRevision:2,Entry:&e});if err!=nil{t.Fatal(err)}
 found,err=s.Search(ctx,"knowledge","玄铁剑",1,"",10,0);if err!=nil||len(found)!=0{t.Fatal("stale index",err)}
 if _,err=s.Mutate(ctx,"delete",authoring.Mutation{ExpectedRevision:updated.Revision,DeleteID:e.ID});err!=nil{t.Fatal(err)}
 found,err=s.Search(ctx,"knowledge","古琴",1,"",10,0);if err!=nil||len(found)!=0{t.Fatal("delete index stale",err)}
}
func TestAuthoringSelectionPinsContentAndFiltersScope(t *testing.T){
 s,_,_:=setup(t);ctx:=t.Context();e:=entry("knowledge","秘密档案内容");e.FromChapter=10;e.POV="Mira";created,err:=s.Mutate(ctx,"secret-ref",authoring.Mutation{ExpectedRevision:1,Entry:&e});if err!=nil{t.Fatal(err)}
 early,err:=s.Select(ctx,"early","writing","档案",9,"Mira");if err!=nil{t.Fatal(err)};wrong,err:=s.Select(ctx,"wrong","writing","档案",10,"Other");if err!=nil{t.Fatal(err)};for _,sel:=range []authoring.Selection{early,wrong}{for _,item:=range sel.Entries{if item.ID==created.EntryID{t.Fatal("chapter/POV leak")}}}
 selected,err:=s.Select(ctx,"stable","writing","档案",10,"Mira");if err!=nil{t.Fatal(err)}
 e.ID=created.EntryID;e.Markdown="修改后的资料";if _,err=s.Mutate(ctx,"change",authoring.Mutation{ExpectedRevision:2,Entry:&e});err!=nil{t.Fatal(err)}
 again,err:=s.Select(ctx,"stable","writing","档案",10,"Mira");if err!=nil||authoring.Digest(again)!=authoring.Digest(selected){t.Fatal("replay changed selected inputs",err)}
 next,err:=s.Select(ctx,"new","writing","档案",10,"Mira");if err!=nil||next.Revision<=selected.Revision{t.Fatal("new scope ignored update",err)}
 polish,err:=s.Select(ctx,"rewrite","polish","",1,"");if err!=nil{t.Fatal(err)};roles:=map[string]bool{};for _,e:=range polish.Entries{roles[e.Role]=true};if !roles["writing"]||!roles["polish"]{t.Fatal("missing polish/writing skill")}
}
func TestAuthoringRulesAreBoundedAdvisories(t *testing.T){
 rules:=authoring.DefaultRules();rules.MinSentenceRunes=4;rules.Phrases=[]string{"不由得"};text:="不由得，不由得。窗外的钟声响了。窗外的钟声响了。"
 report:=rules.Evaluate(text,[]string{"窗外的钟声响了。"});codes:=map[string]bool{};for _,f:=range report.Findings{codes[f.Code]=true};if !report.Advisory||!codes["PHRASE_OVERUSE"]||!codes["SENTENCE_REPEATED"]||!codes["RECENT_SENTENCE_REUSED"]{t.Fatal(report)}
 rules.Enabled=false;if len(rules.Evaluate(text,nil).Findings)!=0{t.Fatal("disabled rules fired")}
 rules.Phrases=[]string{strings.Repeat("x",161)};if rules.Validate()==nil{t.Fatal("unbounded phrase")}
 s,_,_:=setup(t);e:=entry("knowledge",strings.Repeat("x",authoring.MaxMarkdownBytes+1));if _,err:=s.Mutate(t.Context(),"large",authoring.Mutation{ExpectedRevision:1,Entry:&e});!errors.Is(err,authoring.ErrValidation){t.Fatal("oversize accepted",err)}
 state,err:=s.State(t.Context(),"",10,0);if err!=nil||state.Revision!=1{t.Fatal("failed write mutated state",state,err)}
}
