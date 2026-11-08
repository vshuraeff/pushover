package listen

import (
	"bytes"
	"context"
	"testing"

	"github.com/vshuraeff/pushover/internal/api"
	"github.com/vshuraeff/pushover/internal/config"
	"github.com/vshuraeff/pushover/internal/crypto"
	"github.com/vshuraeff/pushover/internal/notify"
	"github.com/vshuraeff/pushover/internal/openclient"
)

func TestRenderHTML(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantText string
		wantLink string
	}{
		{"formatting tags", `<b>bold</b>, <i>italic</i>, <u>under</u>`, "bold, italic, under", ""},
		{"font and link", `<font color="#ff0000">red</font> <a href="https://example.invalid">link</a>`, "red link", "https://example.invalid"},
		{"line breaks", "one<br>two<BR/>three<br />four", "one\ntwo\nthree\nfour", ""},
		{"entities decoded after stripping", `&lt;b&gt; is &amp; stays &quot;text&quot;`, `<b> is & stays "text"`, ""},
		{"newlines kept", "line 1\n<b>line 2</b>", "line 1\nline 2", ""},
		{"surrounding space trimmed", "  <b>x</b><br>", "x", ""},
		{"literal less-than kept", "free < 10% on <b>host</b>", "free < 10% on host", ""},
		{"literal comparison kept", "Temperature < 5; pressure > 10: <b>alarm</b>", "Temperature < 5; pressure > 10: alarm", ""},
		{"quoted attribute with greater-than", `<a href="https://h/?q=a>b">x</a>`, "x", "https://h/?q=a>b"},
		{"single-quoted attribute", `<font color='red'>hot</font>`, "hot", ""},
		{"comment dropped", "<!-- tpl v2 --><b>disk</b> full", "disk full", ""},
		{"first link wins", `<a href="https://one.test">1</a> <a href="https://two.test">2</a>`, "1 2", "https://one.test"},
		{"link entities decoded", `<A class="x" HREF='https://h.test/?a=1&amp;b=2'>go</A>`, "go", "https://h.test/?a=1&b=2"},
		{"unquoted href", `<a href=https://h.test/x>go</a>`, "go", "https://h.test/x"},
		{"commented link ignored", `<!-- <a href="https://old.test">old</a> --><a href="https://new.test">new</a>`, "new", "https://new.test"},
		{"href-like attribute ignored", `<a data-href="https://bad.test" href="https://good.test">x</a>`, "x", "https://good.test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text, link := renderHTML(tt.in)
			if text != tt.wantText || link != tt.wantLink {
				t.Fatalf("renderHTML(%q) = (%q, %q), want (%q, %q)", tt.in, text, link, tt.wantText, tt.wantLink)
			}
		})
	}
}

func TestSyncerRendersHTMLBodyAsPlainText(t *testing.T) {
	key := bytes.Repeat([]byte{0x42}, 32)
	encryptedBody, err := crypto.EncryptField(`<b>encrypted</b> &amp; html`, key)
	if err != nil {
		t.Fatalf("EncryptField(body) error = %v", err)
	}
	messages := []openclient.Message{
		{ID: 1, IDStr: "1", Title: "<b>title</b>", Message: `<b>disk</b> at <font color="red">95%</font>`, HTML: 1},
		{ID: 2, IDStr: "2", Title: "plain", Message: "<b>not html</b>"},
		{ID: 3, IDStr: "3", Title: "encrypted", Message: encryptedBody, HTML: 1},
		{ID: 4, IDStr: "4", Title: "link", Message: `see <a href="https://body.test/x">this</a>`, HTML: 1},
		{ID: 5, IDStr: "5", Title: "explicit url", Message: `see <a href="https://body.test/x">this</a>`, URL: "https://explicit.test", HTML: 1},
		{ID: 6, IDStr: "6", Title: "plain link", Message: `see <a href="https://body.test/x">this</a>`},
	}
	store := &memoryStore{state: &config.State{}}
	notifier := &recordingNotifier{}
	syncer := newSyncer(syncerConfig{
		ctx:           context.Background(),
		store:         store,
		state:         store.state,
		notifier:      notifier,
		logger:        testLogger(),
		encryptionKey: key,
		fetch: func(context.Context, *api.Client, string, string) ([]openclient.Message, error) {
			return append([]openclient.Message(nil), messages...), nil
		},
		updateHighest: func(context.Context, string, string, int64) error { return nil },
	})

	if err := syncer.runOnce(context.Background()); err != nil {
		t.Fatalf("runOnce() error = %v", err)
	}
	notifier.mu.Lock()
	got := append([]notify.Notification(nil), notifier.notifications...)
	notifier.mu.Unlock()
	want := []struct{ title, body, url string }{
		{"<b>title</b>", "disk at 95%", ""},
		{"plain", "<b>not html</b>", ""},
		{"encrypted", "encrypted & html", ""},
		{"link", "see this", "https://body.test/x"},
		{"explicit url", "see this", "https://explicit.test"},
		{"plain link", `see <a href="https://body.test/x">this</a>`, ""},
	}
	if len(got) != len(want) {
		t.Fatalf("notifications = %d, want %d", len(got), len(want))
	}
	for i, w := range want {
		if got[i].Title != w.title || got[i].Body != w.body || got[i].URL != w.url {
			t.Fatalf("notification %d = title %q body %q url %q, want title %q body %q url %q", i, got[i].Title, got[i].Body, got[i].URL, w.title, w.body, w.url)
		}
	}
}
