package lifecycle

import (
 "bytes"
 "encoding/xml"
 "fmt"
 "io"
 "net/url"
 "path"
 "strings"
)

type containerDoc struct { Roots []struct { Path string `xml:"full-path,attr"` } `xml:"rootfiles>rootfile"` }
type packageDoc struct {
 Title string `xml:"metadata>title"`; Language string `xml:"metadata>language"`
 Items []struct { ID string `xml:"id,attr"`; Href string `xml:"href,attr"`; Media string `xml:"media-type,attr"`; Properties string `xml:"properties,attr"` } `xml:"manifest>item"`
 Spine []struct { Ref string `xml:"idref,attr"`; Linear string `xml:"linear,attr"` } `xml:"spine>itemref"`
}
func parseEPUB(data []byte) (Book,error) {
 files,err:=ReadZIP(data); if err!=nil { return Book{},err }
 if string(files["mimetype"])!="application/epub+zip" || files["META-INF/encryption.xml"]!=nil { return Book{},ErrInvalid }
 var c containerDoc; if xml.Unmarshal(files["META-INF/container.xml"],&c)!=nil || len(c.Roots)!=1 || !SafeName(c.Roots[0].Path) { return Book{},ErrInvalid }
 opf:=c.Roots[0].Path; var p packageDoc
 if xml.Unmarshal(files[opf],&p)!=nil || len(p.Spine)==0 || len(p.Spine)>1000 { return Book{},ErrInvalid }
 b:=Book{Title:p.Title,Language:p.Language}; ids:=map[string]int{}
 for i,item:=range p.Items { if item.ID=="" { return b,ErrInvalid }; if _,ok:=ids[item.ID];ok { return b,ErrInvalid };ids[item.ID]=i }
 seen:=map[string]bool{}
 for _,ref:=range p.Spine {
  i,ok:=ids[ref.Ref]; if !ok || seen[ref.Ref] { return b,ErrInvalid };seen[ref.Ref]=true
  item:=p.Items[i]; if ref.Linear=="no" || strings.Contains(" "+item.Properties+" "," nav ") { continue }
  if item.Media!="application/xhtml+xml" && item.Media!="text/html" { return b,ErrInvalid }
  u,err:=url.Parse(item.Href); if err!=nil || u.IsAbs() || u.Host!="" || u.RawQuery!="" || strings.HasPrefix(u.Path,"/") { return b,ErrInvalid }
  name:=path.Join(path.Dir(opf),u.Path); if !SafeName(name) { return b,ErrInvalid }
  raw,ok:=files[name]; if !ok || len(raw)>MaxChapter*2 { return b,ErrInvalid }
  title,text,err:=xhtmlText(raw); if err!=nil { return b,err }
  if strings.TrimSpace(text)=="" { continue }
  if title=="" { title=fmt.Sprintf("第%d章",len(b.Chapters)+1) }
  b.Chapters=append(b.Chapters,Chapter{Title:title,Text:text})
 }
 return b,nil
}

// XML tokens become plain text; no resource is fetched or executed.
func xhtmlText(raw []byte) (string,string,error) {
 d:=xml.NewDecoder(bytes.NewReader(raw));d.Entity=map[string]string{"nbsp":" ","mdash":"—","ndash":"–","hellip":"…","lsquo":"‘","rsquo":"’","ldquo":"“","rdquo":"”"}
 body:=false; skip:=0; heading:=false; title:=""; depth:=0;var text,h strings.Builder
 for { tok,err:=d.Token();if err==io.EOF { break };if err!=nil { return "","",ErrInvalid }
  switch t:=tok.(type) {
  case xml.Directive: if strings.Contains(strings.ToUpper(string(t)),"ENTITY") { return "","",ErrInvalid }
  case xml.StartElement:
   depth++;if depth>128{return "","",ErrLimit}
   n:=strings.ToLower(t.Name.Local);if skip>0 {skip++;continue};if n=="script" || n=="style" || n=="head" || n=="svg" {skip=1;continue};if n=="body" {body=true};if !body {continue}
   if (n=="h1" || n=="h2") && title=="" { heading=true;h.Reset() }
   if n=="p" || n=="div" || n=="br" || n=="li" || n=="h1" || n=="h2" {text.WriteByte('\n')}
  case xml.EndElement:
   depth--;if skip>0 {skip--;continue};n:=strings.ToLower(t.Name.Local)
   if (n=="h1" || n=="h2") && heading {title=strings.TrimSpace(h.String());heading=false}
   if body && (n=="p" || n=="div" || n=="li" || n=="h1" || n=="h2") {text.WriteByte('\n')};if n=="body" {body=false}
  case xml.CharData: if body && skip==0 {if heading {h.Write(t)} else {text.Write(t)}}
  }
 }
 return title,strings.TrimSpace(text.String()),nil
}
