package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kazzer/zboard-merc/internal/engine"
	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
	"github.com/Kazzer/zboard-merc/internal/version"
)

func init() { slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil))) }

type nopOut struct{}

func (nopOut) Key(keys.Key, bool) {}
func (nopOut) Type(string)        {}
func (nopOut) Launch(string)      {}
func (nopOut) LEDs(bool, bool)    {}
func (nopOut) Table(merc.Table)   {}
func (nopOut) ModRequest(string)  {}
func (nopOut) OpenUI()            {}

func testControl(t *testing.T) (*control, *selector, string) {
	t.Helper()
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "mods"), 0o755)
	os.WriteFile(filepath.Join(dir, "mods", "a.yaml"), []byte("name: a\ntriggers: [game.exe]\nkeys:\n  pad.1: f1\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "mods", "b.yaml"), []byte("# b\nname: b\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "config.yaml"), []byte("default: b\nleds:\n  enabled: mod\n  action: mod\n"), 0o644)
	cfg, mods, err := loadConfig(dir)
	if err != nil {
		t.Fatal(err)
	}
	sel := &selector{eng: engine.New(nopOut{}, scheduler{}), mods: mods}
	sel.def = sel.byName(cfg.Default)
	sel.foreground("")
	cfg.dir = dir
	ctl, err := newControl(dir, sel, newRecorder(sel.eng), func() ([]string, error) {
		c, mods, err := loadConfig(dir)
		if err != nil {
			return nil, err
		}
		sel.replace(mods, c.Default)
		var names []string
		for _, m := range mods {
			names = append(names, m.Name)
		}
		return names, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return ctl, sel, dir
}

type caller struct {
	t   *testing.T
	url string
}

type reply struct {
	code    int
	body    string
	headers http.Header
}

func (c caller) do(method, path, body string, headers map[string]string) reply {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.url+path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if strings.HasPrefix(body, "{") {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		if k == "Host" {
			req.Host = v
		} else {
			req.Header.Set(k, v)
		}
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return reply{resp.StatusCode, string(b), resp.Header}
}

func TestControl(t *testing.T) {
	ctl, _, _ := testControl(t)
	srv := httptest.NewServer(ctl.handler())
	defer srv.Close()
	c := caller{t, srv.URL}
	auth := map[string]string{"Authorization": "Bearer " + ctl.token}
	if r := c.do("GET", "/status", "", nil); r.code != 401 {
		t.Errorf("no token: %d", r.code)
	}
	if r := c.do("GET", "/status", "", map[string]string{"Authorization": "Bearer " + ctl.token, "Host": "evil.example"}); r.code != 403 {
		t.Errorf("foreign host: %d", r.code)
	}
	if r := c.do("GET", "/status", "", auth); r.code != 200 || !strings.Contains(r.body, `"active":"b"`) || !strings.Contains(r.body, `"default":"b"`) || !strings.Contains(r.body, `"leds":{"enabled":true,"action":false}`) || !strings.Contains(r.body, `"version":"`+version.String+`"`) {
		t.Errorf("status: %d %s", r.code, r.body)
	}
	if r := c.do("POST", "/mod", "name=a", auth); r.code != 200 || !strings.Contains(r.body, `"active":"a"`) || !strings.Contains(r.body, `"pinned":"a"`) {
		t.Errorf("pin: %d %s", r.code, r.body)
	}
	if r := c.do("POST", "/mod", "name=zzz", auth); r.code != 404 {
		t.Errorf("unknown mod: %d", r.code)
	}
	if r := c.do("POST", "/flag", "name=Streaming&on=1", auth); r.code != 200 || !strings.Contains(r.body, `"streaming":true`) {
		t.Errorf("flag: %d %s", r.code, r.body)
	}
	if r := c.do("POST", "/mod", "", auth); r.code != 200 || !strings.Contains(r.body, `"active":"b"`) {
		t.Errorf("unpin: %d %s", r.code, r.body)
	}
	r := c.do("GET", "/keys", "", auth)
	for _, want := range []string{`"pad.w"`, `"num./"`, `"hotkey.1"`, `"backlight.colour"`, `"left-ctrl"`} {
		if !strings.Contains(r.body, want) {
			t.Errorf("keys lack %s", want)
		}
	}
}

func TestControlConfig(t *testing.T) {
	ctl, _, dir := testControl(t)
	srv := httptest.NewServer(ctl.handler())
	defer srv.Close()
	c := caller{t, srv.URL}
	auth := map[string]string{"Authorization": "Bearer " + ctl.token}
	if r := c.do("GET", "/config", "", auth); r.code != 200 || !strings.Contains(r.body, `"default":"b"`) || !strings.Contains(r.body, `"dir":`) {
		t.Errorf("get config: %d %s", r.code, r.body)
	}
	if r := c.do("PUT", "/config", `{"default":"a","leds":{"enabled":"flag:x","action":"mod"}}`, auth); r.code != 200 || !strings.Contains(r.body, `"default":"a"`) {
		t.Errorf("put config: %d %s", r.code, r.body)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "config.yaml"))
	if !strings.Contains(string(b), "default: a\n") || !strings.Contains(string(b), "enabled: flag:x") || strings.Contains(string(b), "dir:") {
		t.Errorf("config.yaml written:\n%s", b)
	}
	if r := c.do("PUT", "/config", `{"leds":{"enabled":"purple","action":"mod"}}`, auth); r.code != 422 {
		t.Errorf("bad led source: %d %s", r.code, r.body)
	}
	if r := c.do("PUT", "/config", `{"default":"nope"}`, auth); r.code != 422 {
		t.Errorf("unknown default: %d %s", r.code, r.body)
	}
}

func TestControlMods(t *testing.T) {
	ctl, sel, dir := testControl(t)
	srv := httptest.NewServer(ctl.handler())
	defer srv.Close()
	c := caller{t, srv.URL}
	auth := map[string]string{"Authorization": "Bearer " + ctl.token}
	if r := c.do("GET", "/mods", "", auth); r.code != 200 || !strings.Contains(r.body, `"name":"a","file":"a.yaml"`) || !strings.Contains(r.body, `"triggers":["game.exe"]`) {
		t.Errorf("list: %d %s", r.code, r.body)
	}
	if r := c.do("GET", "/mods/b", "", auth); r.code != 200 || !strings.Contains(r.body, `"yaml":"# b\nname: b\n"`) {
		t.Errorf("get: %d %s", r.code, r.body)
	}
	r := c.do("POST", "/mods", `{"name":"C d","document":{"table":"legend","keys":[{"source":"pad.1","action":"f1"},{"source":"pad.2","action":{"chord":["left-ctrl","s"]}}],"layers":[]}}`, auth)
	if r.code != 201 || !strings.Contains(r.body, `"file":"c-d.yaml"`) || !strings.Contains(r.body, `"name":"c-d"`) {
		t.Fatalf("create: %d %s", r.code, r.body)
	}
	if b, err := os.ReadFile(filepath.Join(dir, "mods", "c-d.yaml")); err != nil || !strings.Contains(string(b), "pad.2:\n    chord:\n      - left-ctrl\n      - s\n") {
		t.Errorf("created file: %v\n%s", err, b)
	}
	if sel.byName("c-d") == nil {
		t.Error("created Mod not loaded")
	}
	if r := c.do("POST", "/mods", `{"name":"c d","document":{}}`, auth); r.code != 409 {
		t.Errorf("duplicate create: %d", r.code)
	}
	if r := c.do("POST", "/mods", `{"document":{}}`, auth); r.code != 400 {
		t.Errorf("create without a name: %d", r.code)
	}
	before, _ := os.ReadFile(filepath.Join(dir, "mods", "c-d.yaml"))
	if r := c.do("PUT", "/mods/C-d", `{"document":{"keys":[{"source":"pad.1","action":{"tap":"no-such-key"}}]}}`, auth); r.code != 422 || !strings.Contains(r.body, "no-such-key") {
		t.Errorf("invalid put: %d %s", r.code, r.body)
	}
	if after, _ := os.ReadFile(filepath.Join(dir, "mods", "c-d.yaml")); string(after) != string(before) {
		t.Error("an invalid put changed the file")
	}
	if r := c.do("PUT", "/mods/c-d", `{"name":"c","yaml":"keys:\n  pad.2: f2  # kept\n"}`, auth); r.code != 200 || !strings.Contains(r.body, `"name":"c"`) || !strings.Contains(r.body, `"file":"c.yaml"`) || !strings.Contains(r.body, "# kept") {
		t.Errorf("raw put with rename: %d %s", r.code, r.body)
	}
	if r := c.do("GET", "/mods/c-d", "", auth); r.code != 404 {
		t.Errorf("old name still answers: %d", r.code)
	}
	if r := c.do("PUT", "/mods/c", `{"name":"a","document":{}}`, auth); r.code != 409 {
		t.Errorf("rename onto another Mod: %d %s", r.code, r.body)
	}
	if r := c.do("DELETE", "/mods/c", "", auth); r.code != 204 {
		t.Errorf("delete: %d %s", r.code, r.body)
	}
	if _, err := os.Stat(filepath.Join(dir, "mods", "c.yaml")); !os.IsNotExist(err) {
		t.Error("file not removed")
	}
	if sel.byName("c") != nil {
		t.Error("deleted Mod still loaded")
	}
}

func TestControlUILogin(t *testing.T) {
	ctl, _, _ := testControl(t)
	srv := httptest.NewServer(ctl.handler())
	defer srv.Close()
	c := caller{t, srv.URL}
	if r := c.do("GET", "/ui?token=bad", "", nil); r.code != 401 {
		t.Errorf("bad token: %d", r.code)
	}
	r := c.do("GET", "/ui?token="+ctl.token, "", nil)
	if r.code != 303 || r.headers.Get("Location") != "/ui/" || !strings.Contains(r.headers.Get("Set-Cookie"), "zboard="+ctl.token) || !strings.Contains(r.headers.Get("Set-Cookie"), "HttpOnly") {
		t.Fatalf("login: %d %v", r.code, r.headers)
	}
	cookie := map[string]string{"Cookie": "zboard=" + ctl.token}
	if r := c.do("GET", "/status", "", cookie); r.code != 200 {
		t.Errorf("status with cookie: %d", r.code)
	}
	if r := c.do("GET", "/ui/", "", cookie); r.code != 200 || !strings.Contains(r.body, "<title>zboard</title>") {
		t.Errorf("ui page: %d", r.code)
	}
	if r := c.do("GET", "/ui/app.js", "", cookie); r.code != 200 || !strings.Contains(r.body, "openRoute") {
		t.Errorf("ui script: %d", r.code)
	}
	if r := c.do("GET", "/ui/", "", nil); r.code != 401 {
		t.Errorf("ui page without cookie: %d", r.code)
	}
	if r := c.do("POST", "/flag", "name=x&on=1", cookie); r.code != 403 {
		t.Errorf("cookie mutation without the header: %d", r.code)
	}
	if r := c.do("POST", "/flag", "name=x&on=1", map[string]string{"Cookie": "zboard=" + ctl.token, "X-Requested-With": "zboard"}); r.code != 200 {
		t.Errorf("cookie mutation with the header: %d %s", r.code, r.body)
	}
}

func TestControlRecord(t *testing.T) {
	ctl, sel, _ := testControl(t)
	srv := httptest.NewServer(ctl.handler())
	defer srv.Close()
	c := caller{t, srv.URL}
	auth := map[string]string{"Authorization": "Bearer " + ctl.token}
	if r := c.do("GET", "/record", "", auth); r.code != 200 || !strings.Contains(r.body, `"recording":false`) {
		t.Errorf("idle: %d %s", r.code, r.body)
	}
	if r := c.do("POST", "/record/start", "", auth); r.code != 200 || !strings.Contains(r.body, `"recording":true`) {
		t.Errorf("start: %d %s", r.code, r.body)
	}
	k, _ := keys.ByName("x")
	ctl.rec.observe(engine.Event{Usage: k.Usage, Down: true})
	ctl.rec.observe(engine.Event{Usage: k.Usage, Down: false})
	if r := c.do("GET", "/record", "", auth); !strings.Contains(r.body, `"steps":[{"tap":"x"}]`) {
		t.Errorf("so far: %s", r.body)
	}
	if r := c.do("POST", "/record/stop", "", auth); r.code != 200 || !strings.Contains(r.body, `"recording":false`) || !strings.Contains(r.body, `{"tap":"x"}`) {
		t.Errorf("stop: %d %s", r.code, r.body)
	}
	if on, _ := ctl.rec.state(); on {
		t.Error("still recording")
	}
	_ = sel
}
