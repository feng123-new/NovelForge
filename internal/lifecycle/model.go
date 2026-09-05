// Package lifecycle implements bounded, local manuscript interchange. It does
// not execute imported markup or promote prose to canonical facts.
package lifecycle

import (
 "crypto/sha256"
 "encoding/hex"
 "errors"
)

const MaxManuscript = 32 << 20
const MaxChapter = 1 << 20
const MaxArchive = 64 << 20
const MaxExpanded = 256 << 20
const MaxFiles = 4096

var ErrInvalid = errors.New("invalid lifecycle input")
var ErrConflict = errors.New("lifecycle state conflict")
var ErrNotFound = errors.New("lifecycle item not found")
var ErrLimit = errors.New("lifecycle size limit exceeded")

type Chapter struct {
 Number int `json:"number"`
 Title string `json:"title"`
 Text string `json:"text"`
}
type Book struct {
 Title string `json:"title"`
 Language string `json:"language"`
 Chapters []Chapter `json:"chapters"`
}
func SHA(b []byte) string {
 v := sha256.Sum256(b)
 return hex.EncodeToString(v[:])
}
