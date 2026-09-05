package lifecycle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"path"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func SafeName(name string) bool {
	if name == "" || len(name) > 240 || path.Clean(name) != name || strings.HasPrefix(name, "/") || strings.ContainsAny(name, "\\:\x00") {
		return false
	}
	for _, p := range strings.Split(name, "/") {
		if p == "." || p == ".." || strings.TrimRight(p, " .") != p {
			return false
		}
		for _, r := range p {
			if r < 32 || strings.ContainsRune("<>\"|?*", r) {
				return false
			}
		}
		stem := strings.ToUpper(strings.SplitN(p, ".", 2)[0])
		if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" || (len(stem) == 4 && (strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")) && stem[3] >= '0' && stem[3] <= '9') {
			return false
		}
	}
	return utf8.ValidString(name)
}

// ReadZIP verifies names, compression, sizes and CRC before extraction.
func ReadZIP(data []byte) (map[string][]byte, error) {
	if len(data) == 0 || len(data) > MaxArchive {
		return nil, ErrLimit
	}
	z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, ErrInvalid
	}
	if len(z.File) > MaxFiles {
		return nil, ErrLimit
	}
	out := map[string][]byte{}
	seen := map[string]bool{}
	var total uint64
	for _, f := range z.File {
		n := strings.TrimSuffix(f.Name, "/")
		if !SafeName(n) || seen[strings.ToLower(n)] || f.Flags&1 != 0 || (f.Method != zip.Store && f.Method != zip.Deflate) {
			return nil, ErrInvalid
		}
		seen[strings.ToLower(n)] = true
		if f.FileInfo().IsDir() {
			continue
		}
		if !f.Mode().IsRegular() {
			return nil, ErrInvalid
		}
		if f.UncompressedSize64 > MaxExpanded || total > MaxExpanded-f.UncompressedSize64 {
			return nil, ErrLimit
		}
		total += f.UncompressedSize64
		r, err := f.Open()
		if err != nil {
			return nil, ErrInvalid
		}
		b, err := io.ReadAll(io.LimitReader(r, int64(f.UncompressedSize64)+1))
		closeErr := r.Close()
		if err != nil || closeErr != nil || uint64(len(b)) != f.UncompressedSize64 {
			return nil, ErrInvalid
		}
		out[f.Name] = b
	}
	return out, nil
}

type FileRecord struct {
	SHA  string `json:"sha256"`
	Size int    `json:"size"`
}
type Manifest struct {
	Version             int                   `json:"version"`
	ProjectID           string                `json:"project_id"`
	Title               string                `json:"title"`
	Format              int                   `json:"project_format"`
	Schema              int                   `json:"schema_version"`
	Created             time.Time             `json:"created_at"`
	Files               map[string]FileRecord `json:"files"`
	CredentialsIncluded bool                  `json:"credentials_included"`
	JobsIncluded        bool                  `json:"jobs_included"`
}

// Allowlist product data, never configs, workspace jobs, logs, backups, trash
// or executables. Prose and audit history are not automatically redacted.
func BackupPath(name string) bool {
	if !SafeName(name) {
		return false
	}
	switch name {
	case ".novelforge/project.db", ".novelforge/project.json", ".novelforge/foundation-request.json", ".novelforge/foundation-output.json":
		return true
	}
	ext := strings.ToLower(path.Ext(name))
	if ext != ".md" && ext != ".txt" && ext != ".markdown" {
		return false
	}
	for _, prefix := range []string{"chapters/", "references/", ".novelforge/skills/", ".novelforge/style/", ".novelforge/rules/"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}
func Pack(m Manifest, files map[string][]byte) ([]byte, error) {
	if len(files) == 0 || len(files) >= MaxFiles {
		return nil, ErrLimit
	}
	m.Version = 1
	m.Files = map[string]FileRecord{}
	m.CredentialsIncluded = false
	m.JobsIncluded = false
	total := 0
	names := make([]string, 0, len(files))
	for name, b := range files {
		if !BackupPath(name) {
			return nil, ErrInvalid
		}
		total += len(b)
		if total > MaxExpanded {
			return nil, ErrLimit
		}
		names = append(names, name)
		m.Files[name] = FileRecord{SHA(b), len(b)}
	}
	sort.Strings(names)
	raw, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	put := func(name string, b []byte) error {
		w, e := z.Create(name)
		if e != nil {
			return e
		}
		_, e = w.Write(b)
		return e
	}
	if err = put("manifest.json", raw); err != nil {
		return nil, err
	}
	for _, name := range names {
		if err = put(name, files[name]); err != nil {
			return nil, err
		}
	}
	if err = z.Close(); err != nil {
		return nil, err
	}
	if out.Len() > MaxArchive {
		return nil, ErrLimit
	}
	return out.Bytes(), nil
}
func Unpack(data []byte) (Manifest, map[string][]byte, error) {
	var m Manifest
	files, err := ReadZIP(data)
	if err != nil {
		return m, nil, err
	}
	raw := files["manifest.json"]
	if len(raw) == 0 || len(raw) > 2<<20 {
		return m, nil, ErrInvalid
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if d.Decode(&m) != nil {
		return m, nil, ErrInvalid
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return m, nil, ErrInvalid
	}
	delete(files, "manifest.json")
	if m.Version != 1 || m.ProjectID == "" || m.Title == "" || m.Schema < 1 || m.CredentialsIncluded || m.JobsIncluded || len(m.Files) != len(files) {
		return m, nil, ErrInvalid
	}
	for name, b := range files {
		r, ok := m.Files[name]
		if !ok || !BackupPath(name) || len(b) != r.Size || SHA(b) != r.SHA {
			return m, nil, ErrInvalid
		}
	}
	if len(files[".novelforge/project.db"]) == 0 || len(files[".novelforge/project.json"]) == 0 {
		return m, nil, ErrInvalid
	}
	return m, files, nil
}
