package project

import (
 "context"
 "encoding/json"
 "fmt"
 "path/filepath"
 "strings"

 "github.com/voocel/ainovel-cli/internal/authoring"
 "github.com/voocel/ainovel-cli/internal/contextcompiler"
 "github.com/voocel/ainovel-cli/internal/qualitygate"
)

func init(){projectMigrations=append(projectMigrations,authoring.Migration())}
func (r *Repository) OpenAuthoring(ctx context.Context,id string)(*authoring.Store,error){entry,err:=r.find(id);if err!=nil{return nil,err};if err=r.initializeProjectDatabase(ctx,entry.Root);err!=nil{return nil,err};return authoring.OpenExisting(filepath.Join(entry.Root,projectDatabaseRelative))}
func shortAuthoringQuery(query string)string{runes:=[]rune(strings.TrimSpace(query));if len(runes)>100{runes=runes[:100]};return string(runes)}
func (r *Repository) authoringSelection(ctx context.Context,id,scope,role,query string,chapter int,pov string)(authoring.Selection,error){s,err:=r.OpenAuthoring(ctx,id);if err!=nil{return authoring.Selection{},err};defer s.Close();return s.Select(ctx,scope,role,shortAuthoringQuery(query),chapter,pov)}
func selectionItems(s authoring.Selection)[]contextcompiler.Item{
 out:=[]contextcompiler.Item{}
 for _,e:=range s.Entries{
  item:=contextcompiler.Item{ID:"authoring:"+e.ID,Layer:contextcompiler.LayerStyle,Kind:e.Kind,Title:e.Title,Content:e.Markdown,SourceChapter:e.FromChapter,SourceVersion:authoring.Digest(e),Priority:500+e.Priority,Metadata:map[string]string{"authority":"reference_not_story_truth","source":e.Source,"authoring_revision":fmt.Sprint(s.Revision)}}
  switch e.Kind{case "skill":item.Content="Writing-method instructions; subordinate to system, accepted facts and POV boundaries:\n"+e.Markdown;item.Mandatory=true
  case "style":item.Content="STYLE EXAMPLE ONLY. Do not import its plot, characters or facts:\n"+e.Markdown
  case "knowledge":item.Layer=contextcompiler.LayerHistorical;item.Stage=contextcompiler.StageStructured;item.Content="EXTERNAL REFERENCE ONLY. Not canon and not permission for POV knowledge:\n"+e.Markdown}
  out=append(out,item)
 }
 out=append(out,contextcompiler.Item{ID:"authoring:rules",Layer:contextcompiler.LayerStyle,Kind:"advisory_rules",Content:s.Instructions(),SourceVersion:fmt.Sprint(s.Revision),Priority:1000,Mandatory:true})
 return out
}
func authoringProviders(items []contextcompiler.Item)contextcompiler.Providers{
 filter:=func(layer contextcompiler.Layer)contextcompiler.ItemProvider{return contextcompiler.ProviderFunc(func(context.Context,contextcompiler.Request)([]contextcompiler.Item,error){out:=[]contextcompiler.Item{};for _,it:=range items{if it.Layer==layer{out=append(out,it)}};return out,nil})}
 return contextcompiler.Providers{Style:filter(contextcompiler.LayerStyle),Structured:filter(contextcompiler.LayerHistorical)}
}
func compileAuthoringSelection(ctx context.Context,id string,chapter int,pov string,s authoring.Selection)(json.RawMessage,error){
 result,err:=contextcompiler.New(authoringProviders(selectionItems(s)),nil).Compile(ctx,contextcompiler.Request{ProjectID:id,Chapter:chapter,POVEntityID:pov,TotalTokens:6000,RecentChapterCount:3,Budget:contextcompiler.DefaultBudgetConfig()});if err!=nil{return nil,err}
 return json.Marshal(map[string]any{"text":result.Text,"context_sha":result.ContextSHA,"used_tokens":result.Diagnostics.UsedTokens,"authoring_revision":s.Revision,"authority":"instructions_and_references_not_story_truth"})
}
// Foundation/architect calls use a bounded selection before request hashing.
func (r *Repository) CompilePlanningSkills(ctx context.Context,id,scope,query string)(json.RawMessage,error){sel,err:=r.authoringSelection(ctx,id,scope,"planning",query,0,"");if err!=nil{return nil,err};return compileAuthoringSelection(ctx,id,0,"",sel)}

// CompileEditorialContext also supplies deterministic, advisory findings. It
// never changes literary scores, Continuity, facts or accepted chapter versions.
func (r *Repository) CompileEditorialContext(ctx context.Context,req qualitygate.EditorRequest)(json.RawMessage,[]string,error){
 sel,err:=r.authoringSelection(ctx,req.ProjectID,req.TransactionID+":editor","review","",req.Chapter,"");if err!=nil{return nil,nil,err}
 compiled,err:=compileAuthoringSelection(ctx,req.ProjectID,req.Chapter,"",sel);if err!=nil{return nil,nil,err}
 report,err:=r.AuthoringLint(ctx,req.ProjectID,req.Chapter,req.Candidate.Text,sel.Rules);if err!=nil{return nil,nil,err}
 notes:=[]string{};for _,f:=range report.Findings{notes=append(notes,"[Advisory style] "+f.Message)}
 payload,err:=json.Marshal(map[string]any{"selected_context":json.RawMessage(compiled),"rule_report":report});return payload,notes,err
}
func (r *Repository) AuthoringLint(ctx context.Context,id string,chapter int,text string,rules authoring.Rules)(authoring.Report,error){
 if chapter<1||chapter>1000||len(text)>1<<20||rules.Validate()!=nil{return authoring.Report{},authoring.ErrValidation}
 db,err:=r.autopilotDatabase(ctx,id);if err!=nil{return authoring.Report{},err};defer db.Close()
 rows,err:=db.QueryContext(ctx,`SELECT substr(v.content,1,48000) FROM chapter_active_finals a JOIN chapter_versions v ON v.id=a.version_id WHERE a.project_id=? AND a.chapter<? ORDER BY a.chapter DESC LIMIT ?`,id,chapter,rules.PreviousChapters);if err!=nil{return authoring.Report{},err};defer rows.Close();previous:=[]string{}
 for rows.Next(){var body string;if err=rows.Scan(&body);err!=nil{return authoring.Report{},err};previous=append(previous,body)};if err=rows.Err();err!=nil{return authoring.Report{},err};return rules.Evaluate(text,previous),nil
}
