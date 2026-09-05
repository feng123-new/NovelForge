package authoring

import (
 "context"
 "database/sql"
 "encoding/json"
 "errors"
 "fmt"
 "strings"
 "time"
 "unicode"
 "unicode/utf8"

 "github.com/voocel/ainovel-cli/internal/db/migrate"
)

type Store struct { DB *sql.DB }
func OpenExisting(path string) (*Store,error) { db,err:=migrate.Open(path,5*time.Second); if err!=nil { return nil,err }; return &Store{DB:db},nil }
func (s *Store) Close() error { return s.DB.Close() }
type reader interface { QueryContext(context.Context,string,...any)(*sql.Rows,error); QueryRowContext(context.Context,string,...any)*sql.Row }
func readState(ctx context.Context,q reader)(State,error) { var out State; var raw string; err:=q.QueryRowContext(ctx,"SELECT revision,rules_json FROM authoring_state WHERE id=1").Scan(&out.Revision,&raw); if err==nil { err=json.Unmarshal([]byte(raw),&out.Rules) }; out.Entries=[]Entry{}; out.Builtins=Builtins(); return out,err }
func decodeEntries(rows *sql.Rows)([]Entry,error) { defer rows.Close(); out:=[]Entry{}; for rows.Next(){ var raw string; if err:=rows.Scan(&raw);err!=nil{return nil,err};var e Entry;if err:=json.Unmarshal([]byte(raw),&e);err!=nil{return nil,err};out=append(out,e) };return out,rows.Err() }
func (s *Store) State(ctx context.Context,kind string,limit,offset int)(State,error) {
 if (kind!=""&&!ValidKind(kind))||limit<1||limit>100||offset<0{return State{},ErrValidation}
 tx,err:=s.DB.BeginTx(ctx,&sql.TxOptions{ReadOnly:true});if err!=nil{return State{},err};defer tx.Rollback()
 out,err:=readState(ctx,tx);if err!=nil{return out,err};out.Limit=limit;out.Offset=offset
 err=tx.QueryRowContext(ctx,"SELECT count(*) FROM authoring_entries WHERE (?='' OR kind=?)",kind,kind).Scan(&out.Total);if err!=nil{return out,err}
 rows,err:=tx.QueryContext(ctx,"SELECT payload_json FROM authoring_entries WHERE (?='' OR kind=?) ORDER BY kind,priority DESC,id LIMIT ? OFFSET ?",kind,kind,limit,offset);if err!=nil{return out,err};out.Entries,err=decodeEntries(rows);if err!=nil{return out,err};return out,tx.Commit()
}
func (s *Store) Mutate(ctx context.Context,key string,m Mutation)(Change,error) {
 if key==""||len(key)>256||m.ExpectedRevision<1{return Change{},ErrValidation}
 count:=0;if m.Entry!=nil{count++};if m.DeleteID!=""{count++};if m.Rules!=nil{count++};if count!=1{return Change{},ErrValidation}
 requestHash:=Digest(m)
 tx,err:=s.DB.BeginTx(ctx,nil);if err!=nil{return Change{},err};defer tx.Rollback()
 var oldHash,oldResult string
 err=tx.QueryRowContext(ctx,"SELECT request_hash,result_json FROM authoring_operations WHERE idempotency_key=?",key).Scan(&oldHash,&oldResult)
 if err==nil{if oldHash!=requestHash{return Change{},ErrConflict};var out Change;if err=json.Unmarshal([]byte(oldResult),&out);err!=nil{return out,err};out.Replayed=true;return out,nil};if !errors.Is(err,sql.ErrNoRows){return Change{},err}
 result,err:=tx.ExecContext(ctx,"UPDATE authoring_state SET revision=revision+1 WHERE id=1 AND revision=?",m.ExpectedRevision);if err!=nil{return Change{},err};n,err:=result.RowsAffected();if err!=nil{return Change{},err};if n!=1{return Change{},ErrConflict}
 out:=Change{Revision:m.ExpectedRevision+1}
 if m.Entry!=nil {
  e:=*m.Entry;create:=e.ID=="";if create{e.ID="entry_"+Digest(key)[:24]};if err=e.Validate();err!=nil{return out,err}
  var exists int;err=tx.QueryRowContext(ctx,"SELECT count(*) FROM authoring_entries WHERE id=?",e.ID).Scan(&exists);if err!=nil{return out,err};if !create&&exists==0{return out,ErrNotFound}
  var total int;err=tx.QueryRowContext(ctx,"SELECT count(*) FROM authoring_entries").Scan(&total);if err!=nil{return out,err};if create&&total>=MaxEntries{return out,ErrValidation}
  raw,_:=json.Marshal(e)
  _,err=tx.ExecContext(ctx,`INSERT INTO authoring_entries VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET kind=excluded.kind,role=excluded.role,enabled=excluded.enabled,pinned=excluded.pinned,priority=excluded.priority,from_chapter=excluded.from_chapter,pov=excluded.pov,payload_json=excluded.payload_json`,e.ID,e.Kind,e.Role,e.Enabled,e.Pinned,e.Priority,e.FromChapter,e.POV,string(raw));if err!=nil{return out,err}
  if _,err=tx.ExecContext(ctx,"DELETE FROM authoring_search WHERE id=?",e.ID);err!=nil{return out,err}
  _,err=tx.ExecContext(ctx,"INSERT INTO authoring_search(id,kind,text,characters) VALUES(?,?,?,?)",e.ID,e.Kind,e.Title+"\n"+e.Markdown,characterText(e.Title+"\n"+e.Markdown));if err!=nil{return out,err};out.EntryID=e.ID
 }
 if m.DeleteID!="" {
  if !identifier.MatchString(m.DeleteID){return out,ErrValidation}
  result,err=tx.ExecContext(ctx,"DELETE FROM authoring_entries WHERE id=?",m.DeleteID);if err!=nil{return out,err};n,err=result.RowsAffected();if err!=nil{return out,err};if n!=1{return out,ErrNotFound}
  if _,err=tx.ExecContext(ctx,"DELETE FROM authoring_search WHERE id=?",m.DeleteID);err!=nil{return out,err};out.EntryID=m.DeleteID
 }
 if m.Rules!=nil {if err=m.Rules.Validate();err!=nil{return out,err};raw,_:=json.Marshal(m.Rules);if _,err=tx.ExecContext(ctx,"UPDATE authoring_state SET rules_json=? WHERE id=1",string(raw));err!=nil{return out,err}}
 raw,_:=json.Marshal(out);mutation,_:=json.Marshal(m)
 if _,err=tx.ExecContext(ctx,"INSERT INTO authoring_operations VALUES(?,?,?,?)",key,requestHash,string(raw),string(mutation));err!=nil{return out,err}
 return out,tx.Commit()
}
// Encoded rune phrases support two-character names without pretending unicode61 segments Chinese.
func characterText(text string) string {var b strings.Builder;for _,r:=range strings.ToLower(text){if unicode.IsLetter(r)||unicode.IsDigit(r){fmt.Fprintf(&b,"u%x ",r)}else{b.WriteString("gap ")}};return strings.TrimSpace(b.String())}
func matchQuery(query string)(string,error) {
 if len(query)>512||!utf8.ValidString(query){return "",ErrValidation};parts:=strings.Fields(query);if len(parts)>8{parts=parts[:8]};terms:=[]string{}
 for _,p:=range parts { if utf8.RuneCountInString(p)>128{return "",ErrValidation};has:=false;for _,r:=range p{has=has||unicode.IsLetter(r)||unicode.IsDigit(r)};if !has{continue};literal:=strings.ReplaceAll(p,"\"","\"\"");terms=append(terms,`text : "`+literal+`"`, `characters : "`+characterText(p)+`"`) }
 return strings.Join(terms," OR "),nil
}
func search(ctx context.Context,q reader,kind,query string,chapter int,pov string,limit,offset int)([]Entry,error) {
 if !ValidKind(kind)||limit<1||limit>100||offset<0||chapter<0||chapter>1000||len(pov)>200{return nil,ErrValidation};match,err:=matchQuery(query);if err!=nil{return nil,err};if match==""{return []Entry{},nil}
 rows,err:=q.QueryContext(ctx,`SELECT e.payload_json FROM authoring_search JOIN authoring_entries e ON e.id=authoring_search.id WHERE authoring_search MATCH ? AND e.kind=? AND e.enabled=1 AND e.from_chapter<=? AND (e.pov='' OR e.pov=?) ORDER BY bm25(authoring_search),e.priority DESC,e.id LIMIT ? OFFSET ?`,match,kind,chapter,pov,limit,offset);if err!=nil{return nil,err};return decodeEntries(rows)
}
func (s *Store) Search(ctx context.Context,kind,query string,chapter int,pov string,limit,offset int)([]Entry,error){return search(ctx,s.DB,kind,query,chapter,pov,limit,offset)}

// Select pins exact selected Markdown/rules to a durable operation scope before any LLM request.
// Replays read the immutable selection even if a user edits a library later.
func (s *Store) Select(ctx context.Context,scope,role,query string,chapter int,pov string)(Selection,error) {
 if scope==""||len(scope)>256||!ValidRole(role)||chapter<0||chapter>1000||len(pov)>200{return Selection{},ErrValidation}
 if _,err:=matchQuery(query);err!=nil{return Selection{},err}
 key:=Digest([]any{scope,role,chapter,pov});hash:=Digest([]any{role,query,chapter,pov})
 tx,err:=s.DB.BeginTx(ctx,nil);if err!=nil{return Selection{},err};defer tx.Rollback()
 var storedHash,raw string
 err=tx.QueryRowContext(ctx,"SELECT request_hash,payload_json FROM authoring_selections WHERE id=?",key).Scan(&storedHash,&raw)
 if err==nil {var out Selection;if storedHash!=hash{return out,ErrConflict};err=json.Unmarshal([]byte(raw),&out);return out,err};if !errors.Is(err,sql.ErrNoRows){return Selection{},err}
 state,err:=readState(ctx,tx);if err!=nil{return Selection{},err};out:=Selection{Revision:state.Revision,Role:role,Rules:state.Rules,Entries:[]Entry{}}
 for _,e:=range state.Builtins{if e.Role==role||(role=="polish"&&e.Role=="writing"){out.Entries=append(out.Entries,e)}}
 rows,err:=tx.QueryContext(ctx,`SELECT payload_json FROM authoring_entries WHERE enabled=1 AND kind='skill' AND (role=? OR (?='polish' AND role='writing')) AND from_chapter<=? AND (pov='' OR pov=?) ORDER BY priority DESC,id LIMIT 17`,role,role,chapter,pov);if err!=nil{return out,err};skills,err:=decodeEntries(rows);if err!=nil{return out,err};if len(skills)>16{return out,ErrValidation};out.Entries=append(out.Entries,skills...)
 for _,kind:=range []string{"style","knowledge"}{
  rows,err=tx.QueryContext(ctx,`SELECT payload_json FROM authoring_entries WHERE enabled=1 AND pinned=1 AND kind=? AND from_chapter<=? AND (pov='' OR pov=?) ORDER BY priority DESC,id LIMIT 6`,kind,chapter,pov);if err!=nil{return out,err};items,err:=decodeEntries(rows);if err!=nil{return out,err}
  matched,err:=search(ctx,tx,kind,query,chapter,pov,6,0);if err!=nil{return out,err};seen:=map[string]bool{};for _,e:=range append(items,matched...){if !seen[e.ID]{seen[e.ID]=true;out.Entries=append(out.Entries,e)}}
 }
 data,err:=json.Marshal(out);if err!=nil{return out,err};if len(data)>512*1024{return out,ErrValidation}
 if _,err=tx.ExecContext(ctx,"INSERT INTO authoring_selections VALUES(?,?,?)",key,hash,string(data));err!=nil{return out,err};return out,tx.Commit()
}
