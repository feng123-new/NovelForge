package server

import (
 "encoding/json"
 "fmt"
 "os"
 "runtime"
 "testing"
 "time"

 "github.com/voocel/ainovel-cli/internal/chapterversion"
 "github.com/voocel/ainovel-cli/internal/observability"
 "github.com/voocel/ainovel-cli/internal/project"
)

// Explicit local scale fixture: version storage, accounting and portable backup.
// It is NOT a thousand-chapter model run or a claim of literary consistency.
func TestCandidateSyntheticChapterScale(t *testing.T) {
 if os.Getenv("NOVELFORGE_SCALE_TEST")!="1" {t.Skip("explicit local scale mode only")}
 for _,count:=range []int{100,500,1000}{t.Run(fmt.Sprint(count),func(t *testing.T){
  started:=time.Now();app,err:=New(Config{Workspace:t.TempDir()});if err!=nil{t.Fatal(err)};defer app.Close()
  p,err:=app.projects.Create(t.Context(),project.CreateInput{Title:fmt.Sprintf("Synthetic %d",count),TargetChapters:count});if err!=nil{t.Fatal(err)}
  versions,err:=app.projects.OpenChapterVersionStore(t.Context(),p.ID);if err!=nil{t.Fatal(err)};defer versions.Close()
  observations,close,err:=app.projects.OpenObservations(t.Context(),p.ID);if err!=nil{t.Fatal(err)};defer close()
  policy:=observability.DefaultPolicy();policy.Prices=[]observability.Price{{Provider:"synthetic",Model:"no-model"}}
  if _,err=observations.Mutate(t.Context(),"free-fixture",observability.Mutation{ExpectedRevision:1,Policy:&policy});err!=nil{t.Fatal(err)}
  for chapter:=1;chapter<=count;chapter++{
   content:=fmt.Sprintf("第%d章\n\n这只是离线存储规模样本，不是已接受的小说事实。",chapter)
   if _,err=versions.Create(t.Context(),chapter,chapterversion.CreateInput{Content:content,Type:chapterversion.TypeHumanRevision,AuthorType:chapterversion.AuthorHuman});err!=nil{t.Fatal(err)}
   ctx:=observability.WithCall(t.Context(),observations,observability.Identity{LogicalKey:fmt.Sprintf("fixture-%d",chapter),RequestHash:fmt.Sprintf("%064x",chapter),TransactionID:fmt.Sprintf("fixture-%d",chapter),Chapter:chapter,Agent:"fixture",Operation:"synthetic"})
   ticket,err:=observability.Start(ctx,"synthetic","no-model",256,"synthetic_fixture_not_network");if err!=nil{t.Fatal(err)};if err=ticket.Finish(t.Context(),0,0,true,"");err!=nil{t.Fatal(err)}
  }
  generated:=time.Since(started);queryStart:=time.Now();page,err:=observations.Page(t.Context(),"",0,50,0);if err!=nil||page.Total!=count||len(page.Attempts)!=50{t.Fatal("bounded page",err,page.Total)};queryDuration:=time.Since(queryStart)
  last,err:=versions.Latest(t.Context(),count,true);if err!=nil||last==nil{t.Fatal("last version",err)}
  backupStart:=time.Now();backup,err:=app.projects.BackupLifecycle(t.Context(),p.ID);if err!=nil{t.Fatal(err)}
  var memory runtime.MemStats;runtime.ReadMemStats(&memory)
  report,_:=json.Marshal(map[string]any{"chapters":count,"fixture_seconds":generated.Seconds(),"observation_page_seconds":queryDuration.Seconds(),"backup_seconds":time.Since(backupStart).Seconds(),"backup_bytes":len(backup),"heap_bytes":memory.HeapAlloc,"model_requests":0,"scope":"synthetic versions/accounting/backup; separate temporal-index fixture covers facts"});t.Log(string(report))
 })}
}
