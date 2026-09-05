package project

import (
 "context"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"

 "github.com/voocel/ainovel-cli/internal/autopilot"
)

type foundationOutput struct {RequestID string `json:"request_id"`;Foundation autopilot.Foundation `json:"foundation"`}
// Foundation output is a project-local planning artifact, not accepted Truth.
// Holding AcquireExecution is the caller's responsibility for all writes.
func(r *Repository) LoadAutopilotFoundation(ctx context.Context,id,requestID string)(*autopilot.Foundation,error){
 if err:=ctx.Err();err!=nil{return nil,err};entry,err:=r.find(id);if err!=nil{return nil,err}
 path:=filepath.Join(entry.Root,".novelforge","foundation-output.json")
 info,err:=os.Lstat(path);if err!=nil{return nil,err};if info.Mode()&os.ModeSymlink!=0||!info.Mode().IsRegular()||info.Size()>512*1024{return nil,ErrUnsafePath}
 var out foundationOutput;if err=readJSONFile(path,&out);err!=nil{return nil,err};if out.RequestID!=requestID{return nil,os.ErrNotExist};return &out.Foundation,nil
}
func(r *Repository) SaveAutopilotFoundation(ctx context.Context,id,requestID string,f autopilot.Foundation)error{
 old,err:=r.LoadAutopilotFoundation(ctx,id,requestID)
 if err==nil {a,_:=json.Marshal(old);b,_:=json.Marshal(f);if string(a)!=string(b){return ErrConflict};return nil}
 if !errors.Is(err,os.ErrNotExist){return err}
 entry,err:=r.find(id);if err!=nil{return err}
 return writeJSONAtomic(filepath.Join(entry.Root,".novelforge","foundation-output.json"),foundationOutput{RequestID:requestID,Foundation:f},0600)
}
