package observability

import (
 "context"
 "encoding/json"
 "path/filepath"
 "strings"
 "sync"
 "testing"
 "time"

 "github.com/voocel/ainovel-cli/internal/db/migrate"
)
func testStore(t *testing.T)*Store{t.Helper();path:=filepath.Join(t.TempDir(),"project.db");err:=(migrate.Runner{Path:path,Migrations:[]migrate.Migration{{Version:1,Name:"test_legacy_calls",SQL:`CREATE TABLE model_calls(idempotency_key TEXT PRIMARY KEY);`},Migration()}}).Run(t.Context());if err!=nil{t.Fatal(err)};db,err:=migrate.Open(path,time.Second);if err!=nil{t.Fatal(err)};t.Cleanup(func(){_ = db.Close()});return &Store{DB:db,ProjectID:"test-project"}}
func attempt(t *testing.T,s *Store,key string)(*Ticket,error){t.Helper();return Start(WithCall(t.Context(),s,Identity{LogicalKey:key,RequestHash:digest(key),TransactionID:"tx",Agent:"writer",Operation:"draft",Chapter:1}),"primary","model",300,"sdk_generate")}
func setPolicy(t *testing.T,s *Store,p Policy){t.Helper();state,err:=s.State(t.Context());if err!=nil{t.Fatal(err)};if _,err=s.Mutate(t.Context(),"policy-"+time.Now().Format("150405.000000000"),Mutation{ExpectedRevision:state.Revision,Policy:&p});err!=nil{t.Fatal(err)}}
func TestObservationBudgetsPriceSnapshotsAndReplay(t *testing.T){s:=testStore(t);p:=DefaultPolicy();p.Prices=[]Price{{Provider:"primary",Model:"model",InputMicrosPerMillion:1000000,OutputMicrosPerMillion:2000000,Note:"private note"}};setPolicy(t,s,p);a,err:=attempt(t,s,"first");if err!=nil{t.Fatal(err)};if err=a.Finish(t.Context(),10,20,true,"");err!=nil{t.Fatal(err)};s.Replay(t.Context(),"first");page,err:=s.Page(t.Context(),"",0,50,0);if err!=nil||page.Totals.Calls!=1||page.Replays!=1||page.Totals.CostMicros!=50{t.Fatal(page,err)}
 p.Prices[0].OutputMicrosPerMillion=3000000;p.ProjectBudgetMicros=50;setPolicy(t,s,p);if _,err=attempt(t,s,"blocked");ControlCode(err)!="PROJECT_BUDGET_EXCEEDED"{t.Fatal(err)};page,err=s.Page(t.Context(),"",0,50,0);if err!=nil||*page.Attempts[0].CostMicros!=50||page.Attempts[0].Price.OutputMicrosPerMillion!=2000000{t.Fatal("historical repricing",page,err)}
 state,_:=s.State(t.Context());p.Currency="JPY";if _,err=s.Mutate(t.Context(),"currency",Mutation{ExpectedRevision:state.Revision,Policy:&p});err!=ErrConflict{t.Fatal("currency drift",err)}
}
func TestObservationPendingReconcileAndUnknown(t *testing.T){s:=testStore(t);a,err:=attempt(t,s,"interrupted");if err!=nil{t.Fatal(err)};if _,err=attempt(t,s,"second");ControlCode(err)!="UNRESOLVED_MODEL_ATTEMPT"{t.Fatal(err)};state,_:=s.State(t.Context());m:=Mutation{ExpectedRevision:state.Revision,Reconcile:&Reconciliation{AttemptID:a.Attempt.ID,CostMicros:75,Confirm:true}};result,err:=s.Mutate(t.Context(),"reconcile",m);if err!=nil||result.Before.State!="pending"||result.After.State!="abandoned"{t.Fatal(result,err)};replayed,err:=s.Mutate(t.Context(),"reconcile",m);if err!=nil||replayed.Revision!=result.Revision{t.Fatal("mutation replay",err)};if err=a.Finish(t.Context(),1,1,true,"");ControlCode(err)!="OBSERVATION_STORAGE_FAILED"{t.Fatal("stale completion overwrote reconciliation",err)}
 b,err:=attempt(t,s,"after");if err!=nil{t.Fatal(err)};if err=b.Finish(t.Context(),0,0,false,"");err!=nil{t.Fatal(err)};page,err:=s.Page(t.Context(),"manual",0,50,0);if err!=nil||page.Totals.UnknownCost!=1||page.Attempts[0].InputTokens!=nil||page.Attempts[0].CostMicros!=nil{t.Fatal("unknown presented as zero",page,err)}
 p:=DefaultPolicy();p.BlockUnknownCost=true;setPolicy(t,s,p);if _,err=attempt(t,s,"unknown-block");ControlCode(err)!="COST_RECONCILIATION_REQUIRED"{t.Fatal(err)}
}
func TestObservationCooldownPrivacyAndIsolation(t *testing.T){s:=testStore(t);now:=time.Date(2026,9,6,0,0,0,0,time.UTC);s.Now=func()time.Time{return now};p:=DefaultPolicy();p.FailureThreshold=2;p.Prices=[]Price{{Provider:"primary",Model:"model",Note:"SECRET_CANARY",InputMicrosPerMillion:1,OutputMicrosPerMillion:1}};setPolicy(t,s,p)
 for _,key:=range []string{"failed-one","failed-two"}{a,err:=attempt(t,s,key);if err!=nil{t.Fatal(err)};if err=a.Finish(t.Context(),0,0,false,"PROVIDER_TIMEOUT");err!=nil{t.Fatal(err)}};if _,err:=attempt(t,s,"cooldown");ControlCode(err)!="PROVIDER_COOLDOWN"{t.Fatal(err)};health,err:=s.Health(t.Context(),p);if err!=nil||len(health)!=1||health[0].State!="cooldown"{t.Fatal(health,err)};now=now.Add(61*time.Second);a,err:=attempt(t,s,"cooled");if err!=nil{t.Fatal(err)};if err=a.Finish(t.Context(),1,1,true,"");err!=nil{t.Fatal(err)};page,err:=s.Page(t.Context(),"",0,50,0);if err!=nil{t.Fatal(err)};raw,_:=json.Marshal(Redact(page));if strings.Contains(string(raw),"SECRET_CANARY")||strings.Contains(string(raw),"primary")||strings.Contains(string(raw),"failed-one"){t.Fatal("sensitive fields in share report",string(raw))};foreign:=*s;foreign.ProjectID="other";foreignPage,err:=foreign.Page(t.Context(),"",0,50,0);if err!=nil||foreignPage.Totals.Calls!=0{t.Fatal("project isolation",err)}
}
func TestObservationAtomicAdmission(t *testing.T){s:=testStore(t);var wg sync.WaitGroup;results:=make(chan *Ticket,2);for _,key:=range []string{"one","two"}{wg.Add(1);go func(key string){defer wg.Done();ticket,_:=Start(WithCall(context.Background(),s,Identity{LogicalKey:key,RequestHash:digest(key),TransactionID:"tx",Agent:"writer",Operation:"draft"}),"primary","model",300,"sdk_generate");results<-ticket}(key)};wg.Wait();close(results);accepted:=0;for a:=range results{if a!=nil{accepted++}};if accepted!=1{t.Fatal("admissions raced",accepted)}}
