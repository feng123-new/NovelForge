package project

import (
 "bytes"
 "context"
 "encoding/hex"
 "encoding/json"
 "errors"
 "os"
 "path/filepath"
 "strings"

 "github.com/gofrs/flock"
 "github.com/voocel/ainovel-cli/internal/lifecycle"
)

type LifecycleRestoreResult struct { Project Project `json:"project"`; Replayed bool `json:"replayed"`; RequiresConfiguration bool `json:"requires_configuration"`; JobsResumed bool `json:"jobs_resumed"` }

// RestoreLifecycle only publishes a new directory, preserving all project IDs
// and version/event references. An existing ID or destination is never replaced.
func (r *Repository) RestoreLifecycle(ctx context.Context,data []byte) (LifecycleRestoreResult,error) {
 result:=LifecycleRestoreResult{RequiresConfiguration:true}
 m,files,err:=lifecycle.Unpack(data);if err!=nil{return result,err}
 idBytes,err:=hex.DecodeString(m.ProjectID);if err!=nil || len(idBytes)!=16{return result,lifecycle.ErrInvalid}
 var meta Metadata;d:=json.NewDecoder(bytes.NewReader(files[projectMetadataRelative]));d.DisallowUnknownFields()
 if d.Decode(&meta)!=nil || meta.ID!=m.ProjectID || meta.Title!=m.Title || meta.FormatVersion!=m.Format || m.Format<1 || m.Format>CurrentFormatVersion{return result,lifecycle.ErrInvalid}
 if validateCreateInput(CreateInput{Title:meta.Title,TargetWords:meta.TargetWords,TargetChapters:meta.TargetChapters,WordsPerChapter:meta.WordsPerChapter})!=nil{return result,lifecycle.ErrInvalid}
 if meta.Status!=StatusActive && meta.Status!=StatusArchived{return result,lifecycle.ErrInvalid}
 resolved,err:=filepath.EvalSymlinks(r.workspace);if err!=nil || resolved!=r.resolvedWorkspace{return result,ErrUnsafePath}
 private,err:=safeLifecycleDirectory(r.workspace,".novelforge");if err!=nil{return result,err}
 lockPath:=filepath.Join(private,"lifecycle-restore.lock")
 if st,e:=os.Lstat(lockPath);e==nil && !st.Mode().IsRegular(){return result,ErrUnsafePath}else if e!=nil && !errors.Is(e,os.ErrNotExist){return result,e}
 lock:=flock.New(lockPath,flock.SetPermissions(0600));ok,err:=lock.TryLock();if err!=nil || !ok{lock.Close();return result,lifecycle.ErrConflict};defer lock.Close()
 digest:=lifecycle.SHA(data)
 if e,err:=r.find(meta.ID);err==nil {
  marker,readErr:=readLifecycleFile(filepath.Join(e.Root,".novelforge","restore-origin.sha256"),128)
  if readErr==nil && string(marker)==digest {result.Project=e.Project;result.Replayed=true;return result,nil}
  return result,lifecycle.ErrConflict
 } else if !errors.Is(err,ErrNotFound){return result,err}
 staging,err:=os.MkdirTemp(private,"restore-");if err!=nil{return result,err};defer os.RemoveAll(staging)
 for name,b:=range files {
  if ctx.Err()!=nil{return result,ctx.Err()};p:=filepath.Join(staging,filepath.FromSlash(name))
  if err=ensureChildPath(staging,p);err!=nil{return result,err};if err=os.MkdirAll(filepath.Dir(p),0700);err!=nil{return result,err}
  f,err:=os.OpenFile(p,os.O_WRONLY|os.O_CREATE|os.O_EXCL,0600);if err!=nil{return result,err};_,err=f.Write(b);if err==nil{err=f.Sync()};ce:=f.Close();if err==nil{err=ce};if err!=nil{return result,err}
 }
 snapshot:=filepath.Join(staging,projectDatabaseRelative)
 if err=verifyLifecycleDB(ctx,snapshot,m);err!=nil{return result,err}
 if err=verifyLifecycleFiles(ctx,snapshot,m,files);err!=nil{return result,err}
 if err=initializeLayout(staging);err!=nil{return result,err}
 // Updates use the normal checksum/backup runner, after schema verification.
 if err=r.initializeProjectDatabase(ctx,staging);err!=nil{return result,err}
 if err=writeJSONAtomic(filepath.Join(staging,projectConfigRelative),defaultProjectConfig(),0600);err!=nil{return result,err}
 if err=os.WriteFile(filepath.Join(staging,".novelforge","restore-origin.sha256"),[]byte(digest),0600);err!=nil{return result,err}
 if err=checkpointProjectDatabase(ctx,staging);err!=nil{return result,err}
 destination:=filepath.Join(r.workspace,"restored-"+strings.ToLower(meta.ID))
 if _,err=os.Lstat(destination);!errors.Is(err,os.ErrNotExist){return result,lifecycle.ErrConflict}
 if err=os.Rename(staging,destination);err!=nil{return result,err}
 e,err:=r.read(destination);if err!=nil{return result,err};result.Project=e.Project
 return result,nil
}
