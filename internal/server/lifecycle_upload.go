package server

import (
 "encoding/json"
 "errors"
 "io"
 "net/http"
 "path"
 "strconv"
 "strings"

 "github.com/voocel/ainovel-cli/internal/lifecycle"
)

type lifecycleUpload struct { Filename string `json:"filename"`; SHA string `json:"sha256"`; Start int `json:"start_chapter"`; Data []byte `json:"-"` }
func readLifecycleUpload(w http.ResponseWriter,r *http.Request,restore bool) (lifecycleUpload,error) {
 var out lifecycleUpload;out.Start=1
 if !idempotencyKeyPattern.MatchString(strings.TrimSpace(r.Header.Get("Idempotency-Key"))){return out,lifecycle.ErrInvalid}
 limit:=lifecycle.MaxManuscript;if restore{limit=lifecycle.MaxArchive}
 r.Body=http.MaxBytesReader(w,r.Body,int64(limit+65536))
 reader,err:=r.MultipartReader();if err!=nil{return out,lifecycle.ErrInvalid}
 seen:=map[string]bool{}
 for {p,err:=reader.NextPart();if err==io.EOF{break};if err!=nil{return out,lifecycle.ErrInvalid}
  name:=p.FormName();if seen[name]{p.Close();return out,lifecycle.ErrInvalid};seen[name]=true
  switch name {
  case "file":
   out.Filename=p.FileName();if out.Filename=="" || len(out.Filename)>200 || strings.ContainsAny(out.Filename,"\\\x00") || path.Base(out.Filename)!=out.Filename{p.Close();return out,lifecycle.ErrInvalid}
   out.Data,err=io.ReadAll(io.LimitReader(p,int64(limit)+1));p.Close();if err!=nil{return out,lifecycle.ErrInvalid};if len(out.Data)==0 || len(out.Data)>limit{return out,lifecycle.ErrLimit}
  case "start_chapter":
   if restore{p.Close();return out,lifecycle.ErrInvalid};v,err:=io.ReadAll(io.LimitReader(p,16));p.Close();if err!=nil{return out,lifecycle.ErrInvalid};out.Start,err=strconv.Atoi(string(v));if err!=nil || out.Start<1 || out.Start>1000{return out,lifecycle.ErrInvalid}
  default:p.Close();return out,lifecycle.ErrInvalid
  }
 }
 if len(out.Data)==0{return out,lifecycle.ErrInvalid};out.SHA=lifecycle.SHA(out.Data);return out,nil
}
func (s *Server) uploadManuscript(w http.ResponseWriter,r *http.Request,id string) {
 upload,err:=readLifecycleUpload(w,r,false);if err!=nil{writeFailure(w,r,*lifecycleFailure(err));return};body,_:=json.Marshal(upload)
 s.executeIdempotentBody(w,r,"lifecycle.import",id,body,func([]byte)(int,any,*apiFailure){
  v,err:=s.projects.StageManuscript(r.Context(),id,upload.Filename,upload.Data,upload.Start)
  if err!=nil{f:=lifecycleFailure(err);return f.Status,nil,f};return 201,map[string]any{"import":v,"model_called":false,"finalized":false},nil
 })
}
func (s *Server) handleLifecycleRestore(w http.ResponseWriter,r *http.Request) {
 if r.Method!="POST"{writeMethodNotAllowed(w,r,"POST");return}
 upload,err:=readLifecycleUpload(w,r,true);if err!=nil{writeFailure(w,r,*lifecycleFailure(err));return}
 if !strings.EqualFold(path.Ext(upload.Filename),".zip"){writeFailure(w,r,*lifecycleFailure(lifecycle.ErrInvalid));return}
 body,_:=json.Marshal(upload)
 s.executeIdempotentBody(w,r,"lifecycle.restore","",body,func([]byte)(int,any,*apiFailure){
  manifest,_,err:=lifecycle.Unpack(upload.Data);if err!=nil{f:=lifecycleFailure(err);return f.Status,nil,f}
  // An orphaned unfinished workspace job must not adopt restored data.
  pending,err:=s.jobs.Unfinished(r.Context(),manifest.ProjectID);if err!=nil{f:=jobFailure(err);return f.Status,nil,f};if pending{f:=lifecycleFailure(lifecycle.ErrConflict);return f.Status,nil,f}
  result,err:=s.projects.RestoreLifecycle(r.Context(),upload.Data);if err!=nil{f:=lifecycleFailure(err);return f.Status,nil,f};status:=201;if result.Replayed{status=200};return status,result,nil
 })
}

var _ = errors.Is
