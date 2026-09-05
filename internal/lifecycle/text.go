package lifecycle

import (
 "bytes"
 "path"
 "regexp"
 "strings"
 "unicode/utf8"
)

var chapterHeading = regexp.MustCompile(`(?i)^(?:第[0-9零〇一二三四五六七八九十百千万两]+[章回节]|chapter[ \t]+[0-9ivxlcdm]+)(?:\s|[：:、.．—-]|$)`)
var markdownHeading = regexp.MustCompile(`^(#{1,2})[ \t]+(.+)$`)

func Parse(filename string, data []byte) (Book,error) {
 if len(data)==0 || len(data)>MaxManuscript { return Book{},ErrLimit }
 var b Book; var err error
 switch strings.ToLower(path.Ext(filename)) {
 case ".epub": b,err=parseEPUB(data)
 case ".txt", ".md", ".markdown": b,err=parseText(filename,data)
 default: return b,ErrInvalid
 }
 if err!=nil { return b,err }
 if len(b.Chapters)==0 || len(b.Chapters)>1000 { return b,ErrLimit }
 total:=0
 for i:=range b.Chapters {
  c:=&b.Chapters[i]; c.Number=i+1; c.Text=strings.TrimSpace(c.Text)
  if c.Text=="" || !utf8.ValidString(c.Text) || strings.ContainsRune(c.Text,0) { return Book{},ErrInvalid }
  if len(c.Text)>MaxChapter || utf8.RuneCountInString(c.Title)>200 { return Book{},ErrLimit }
  total+=len(c.Text); if total>MaxManuscript { return Book{},ErrLimit }
 }
 return b,nil
}

func parseText(filename string,data []byte) (Book,error) {
 b:=Book{Title:strings.TrimSuffix(path.Base(filename),path.Ext(filename)),Language:"zh"}
 if !utf8.Valid(data) || bytes.ContainsRune(data,0) { return b,ErrInvalid }
 s:=strings.TrimPrefix(string(data),"\ufeff"); s=strings.ReplaceAll(strings.ReplaceAll(s,"\r\n","\n"),"\r","\n")
 lines:=strings.Split(s,"\n"); numbered:=false; level:=3; fenced:=false
 isMD:=strings.ToLower(path.Ext(filename))!=".txt"
 for _,line:=range lines {
  t:=strings.TrimSpace(line); if isMD && (strings.HasPrefix(t,"```") || strings.HasPrefix(t,"~~~")) { fenced=!fenced;continue }; if fenced { continue }
  title:=t; if m:=markdownHeading.FindStringSubmatch(t); isMD && m!=nil { title=m[2]; if len(m[1])<level { level=len(m[1]) } }
  if utf8.RuneCountInString(title)<=200 && chapterHeading.MatchString(title) { numbered=true }
 }
 title:="前言"; buf:=[]string{}; fenced=false
 flush:=func(){ text:=strings.TrimSpace(strings.Join(buf,"\n")); if text!="" { b.Chapters=append(b.Chapters,Chapter{Title:title,Text:text}) };buf=nil }
 for _,line:=range lines {
  t:=strings.TrimSpace(line); if isMD && (strings.HasPrefix(t,"```") || strings.HasPrefix(t,"~~~")) { fenced=!fenced;buf=append(buf,line);continue }
  h:=t; m:=markdownHeading.FindStringSubmatch(t); if isMD && m!=nil { h=m[2] }
  split:=!fenced && utf8.RuneCountInString(h)<=200 && ((numbered && chapterHeading.MatchString(h)) || (!numbered && isMD && m!=nil && len(m[1])==level))
  if split { flush();title=h } else { buf=append(buf,line) }
 }
 flush(); if len(b.Chapters)==1 && b.Chapters[0].Title=="前言" { b.Chapters[0].Title=b.Title }
 return b,nil
}
