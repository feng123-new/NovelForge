package lifecycle

import (
	"archive/zip"
	"bytes"
	"fmt"
	"html"
	"io"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

func Export(book Book, format string) ([]byte, string, error) {
	if len(book.Chapters) == 0 || len(book.Chapters) > 1000 {
		return nil, "", ErrInvalid
	}
	total := 0
	for _, c := range book.Chapters {
		total += len(c.Text)
		if !utf8.ValidString(c.Text) || strings.ContainsRune(c.Text, 0) || c.Number < 1 || strings.TrimSpace(c.Text) == "" {
			return nil, "", ErrInvalid
		}
		if len(c.Text) > MaxChapter || total > MaxExpanded {
			return nil, "", ErrLimit
		}
	}
	switch format {
	case "txt", "md":
		var b strings.Builder
		for _, c := range book.Chapters {
			if format == "md" {
				b.WriteString("# ")
			}
			b.WriteString(c.Title)
			b.WriteString("\n\n")
			b.WriteString(c.Text)
			b.WriteString("\n\n")
		}
		media := "text/plain; charset=utf-8"
		if format == "md" {
			media = "text/markdown; charset=utf-8"
		}
		return []byte(b.String()), media, nil
	case "epub":
		return exportEPUB(book)
	default:
		return nil, "", ErrInvalid
	}
}
func exportEPUB(b Book) ([]byte, string, error) {
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	put := func(name, text string, method uint16) error {
		h := &zip.FileHeader{Name: name, Method: method}
		w, e := z.CreateHeader(h)
		if e != nil {
			return e
		}
		_, e = io.WriteString(w, text)
		return e
	}
	if err := put("mimetype", "application/epub+zip", zip.Store); err != nil {
		return nil, "", err
	}
	files := map[string]string{"META-INF/container.xml": `<?xml version="1.0"?><container version="1.0" xmlns="urn:oasis:names:tc:opendocument:xmlns:container"><rootfiles><rootfile full-path="OEBPS/package.opf" media-type="application/oebps-package+xml"/></rootfiles></container>`}
	esc := html.EscapeString
	lang := b.Language
	if lang == "" {
		lang = "zh"
	}
	var manifest, spine, nav strings.Builder
	for i, c := range b.Chapters {
		name := fmt.Sprintf("chapter-%04d.xhtml", i+1)
		id := fmt.Sprintf("c%d", i+1)
		fmt.Fprintf(&manifest, `<item id="%s" href="%s" media-type="application/xhtml+xml"/>`, id, name)
		fmt.Fprintf(&spine, `<itemref idref="%s"/>`, id)
		fmt.Fprintf(&nav, `<li><a href="%s">%s</a></li>`, name, esc(c.Title))
		var paragraphs strings.Builder
		for _, line := range strings.Split(strings.ReplaceAll(c.Text, "\r\n", "\n"), "\n") {
			if strings.TrimSpace(line) != "" {
				paragraphs.WriteString("<p>" + esc(line) + "</p>")
			}
		}
		files["OEBPS/"+name] = `<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" lang="` + esc(lang) + `"><head><title>` + esc(c.Title) + `</title></head><body><h1>` + esc(c.Title) + `</h1>` + paragraphs.String() + `</body></html>`
	}
	files["OEBPS/nav.xhtml"] = `<?xml version="1.0" encoding="UTF-8"?><html xmlns="http://www.w3.org/1999/xhtml" xmlns:epub="http://www.idpf.org/2007/ops"><head><title>目录</title></head><body><nav epub:type="toc" id="toc"><h1>目录</h1><ol>` + nav.String() + `</ol></nav></body></html>`
	files["OEBPS/package.opf"] = `<?xml version="1.0" encoding="UTF-8"?><package xmlns="http://www.idpf.org/2007/opf" version="3.0" unique-identifier="book-id"><metadata xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:identifier id="book-id">urn:sha256:` + SHA([]byte(fmt.Sprint(b))) + `</dc:identifier><dc:title>` + esc(b.Title) + `</dc:title><dc:language>` + esc(lang) + `</dc:language><meta property="dcterms:modified">` + time.Now().UTC().Format("2006-01-02T15:04:05Z") + `</meta></metadata><manifest><item id="nav" href="nav.xhtml" media-type="application/xhtml+xml" properties="nav"/>` + manifest.String() + `</manifest><spine>` + spine.String() + `</spine></package>`
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := put(name, files[name], zip.Deflate); err != nil {
			return nil, "", err
		}
	}
	if err := z.Close(); err != nil {
		return nil, "", err
	}
	return out.Bytes(), "application/epub+zip", nil
}
