package lifecycle

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLifecycleTextAndEPUBRoundTrip(t *testing.T) {
	for _, name := range []string{"story.txt", "story.md"} {
		b, err := Parse(name, []byte("\ufeff第1章风雪\r\n张三走进青云宗。\r\n\r\n第2章 归来\r\n他带回玄铁剑。"))
		if err != nil || len(b.Chapters) != 2 || b.Chapters[0].Title != "第1章风雪" {
			t.Fatalf("parse %s: %+v %v", name, b, err)
		}
		for _, format := range []string{"txt", "md", "epub"} {
			data, _, err := Export(b, format)
			if err != nil {
				t.Fatal(err)
			}
			got, err := Parse("story."+format, data)
			if err != nil || len(got.Chapters) != 2 {
				t.Fatalf("roundtrip %s: %v %+v", format, err, got)
			}
			for i := range b.Chapters {
				if strings.TrimSpace(got.Chapters[i].Text) != b.Chapters[i].Text {
					t.Fatalf("text changed: %q", got.Chapters[i].Text)
				}
			}
			if format == "epub" {
				z, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
				if err != nil {
					t.Fatal(err)
				}
				if z.File[0].Name != "mimetype" || z.File[0].Method != zip.Store || len(z.File[0].Extra) != 0 {
					t.Fatal("EPUB mimetype header contract")
				}
			}
		}
	}
	if _, err := Parse("legacy.txt", []byte{0xff, 0xfe}); !errors.Is(err, ErrInvalid) {
		t.Fatal("non UTF-8 accepted")
	}
	b, err := Parse("story.md", []byte("# 小说标题\n\n## 第1章 开始\n正文。\n\n```\n## 第9章 示例不是章节\n```\n\n## 第2章 继续\n后文。"))
	if err != nil || len(b.Chapters) != 2 {
		t.Fatalf("markdown fences/title: %+v %v", b, err)
	}
}
func makeZIP(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	z := zip.NewWriter(&b)
	for n, data := range files {
		w, err := z.Create(n)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = w.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}
func TestLifecycleEPUBSpineAndInactiveMarkup(t *testing.T) {
	original := Book{Title: "Test", Chapters: []Chapter{{1, "One", "First."}, {2, "Two", "Second."}}}
	data, _, err := Export(original, "epub")
	if err != nil {
		t.Fatal(err)
	}
	files, err := ReadZIP(data)
	if err != nil {
		t.Fatal(err)
	}
	files["OEBPS/package.opf"] = []byte(strings.Replace(string(files["OEBPS/package.opf"]), `<itemref idref="c1"/><itemref idref="c2"/>`, `<itemref idref="c2"/><itemref idref="c1"/>`, 1))
	files["OEBPS/chapter-0002.xhtml"] = []byte(strings.Replace(string(files["OEBPS/chapter-0002.xhtml"]), "</body>", "<script>never include this</script><style>hidden CSS</style></body>", 1))
	b, err := Parse("spine.epub", makeZIP(t, files))
	if err != nil || len(b.Chapters) != 2 || b.Chapters[0].Text != "Second." {
		t.Fatalf("spine/plain text: %+v %v", b, err)
	}
	var p packageDoc
	if err = xml.Unmarshal(files["OEBPS/package.opf"], &p); err != nil {
		t.Fatal(err)
	}
	files["META-INF/encryption.xml"] = []byte("encrypted")
	if _, err = Parse("encrypted.epub", makeZIP(t, files)); err == nil {
		t.Fatal("encrypted EPUB accepted")
	}
}
func TestLifecycleArchivePathsAndManifest(t *testing.T) {
	for _, name := range []string{"../escape", "/absolute", "C:/drive", "dir\\file", "CON.txt", "a/../b", "a. /b"} {
		if SafeName(name) {
			t.Fatalf("unsafe path: %s", name)
		}
	}
	for _, files := range []map[string][]byte{{"../escape": []byte("x")}, {"a.txt": []byte("1"), "A.txt": []byte("2")}} {
		if _, err := ReadZIP(makeZIP(t, files)); err == nil {
			t.Fatal("unsafe ZIP accepted")
		}
	}
	var raw bytes.Buffer
	z := zip.NewWriter(&raw)
	h := &zip.FileHeader{Name: "link", Method: zip.Store}
	h.SetMode(os.ModeSymlink | 0777)
	w, _ := z.CreateHeader(h)
	_, _ = w.Write([]byte("outside"))
	_ = z.Close()
	if _, err := ReadZIP(raw.Bytes()); err == nil {
		t.Fatal("symlink ZIP accepted")
	}
	files := map[string][]byte{".novelforge/project.db": []byte("snapshot"), ".novelforge/project.json": []byte(`{}`)}
	data, err := Pack(Manifest{ProjectID: "p", Title: "title", Schema: 10, Format: 2}, files)
	if err != nil {
		t.Fatal(err)
	}
	m, got, err := Unpack(data)
	if err != nil || len(got) != 2 || m.CredentialsIncluded || m.JobsIncluded {
		t.Fatal("manifest roundtrip", err)
	}
	entries, _ := ReadZIP(data)
	entries[".novelforge/project.db"] = []byte("changed")
	if _, _, err = Unpack(makeZIP(t, entries)); err == nil {
		t.Fatal("hash mismatch accepted")
	}
	files[".novelforge/config.json"] = []byte(`{"api_key":"test-only-secret"}`)
	if _, err = Pack(m, files); err == nil {
		t.Fatal("credential file accepted")
	}
}
