package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Kazzer/zboard-merc/internal/keys"
	"github.com/Kazzer/zboard-merc/internal/merc"
	"github.com/Kazzer/zboard-merc/internal/profile"
	"github.com/Kazzer/zboard-merc/internal/version"
)

type control struct {
	sel    *selector
	rec    *recorder
	s      *session
	dir    string
	addr   string
	token  string
	reload func() ([]string, error)
}

func (c *control) uiLink() string {
	host, port, err := net.SplitHostPort(c.addr)
	if err != nil || host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port) + "/ui?token=" + c.token
}

type Status struct {
	Version  string          `json:"version"`
	Driver   string          `json:"driver,omitempty"`
	Active   string          `json:"active"`
	Pinned   string          `json:"pinned,omitempty"`
	Default  string          `json:"default,omitempty"`
	Process  string          `json:"process"`
	Table    string          `json:"table"`
	Mods     []string        `json:"mods"`
	Flags    map[string]bool `json:"flags"`
	LEDs     ledStatus       `json:"leds"`
	Attached bool            `json:"attached"`
}

type ledStatus struct {
	Enabled bool `json:"enabled"`
	Action  bool `json:"action"`
}

type configFile struct {
	Default string `json:"default" yaml:"default"`
	Device  string `json:"device" yaml:"device"`
	Control string `json:"control" yaml:"control"`
	TapHold string `json:"tap-hold,omitempty" yaml:"tap-hold,omitempty"`
	LEDs    struct {
		Enabled string `json:"enabled" yaml:"enabled"`
		Action  string `json:"action" yaml:"action"`
	} `json:"leds" yaml:"leds"`
	Dir string `json:"dir,omitempty" yaml:"-"`
}

type modFile struct {
	Document *profile.Document `json:"document"`
	YAML     string            `json:"yaml"`
	File     string            `json:"file"`
}

func newControl(dir string, sel *selector, rec *recorder, reload func() ([]string, error)) (*control, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return nil, err
	}
	c := &control{sel: sel, rec: rec, dir: dir, token: hex.EncodeToString(b[:]), reload: reload}
	if err := os.WriteFile(filepath.Join(dir, "control.token"), []byte(c.token), 0o600); err != nil {
		return nil, fmt.Errorf("control token: %w", err)
	}
	return c, nil
}

func (c *control) serve(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	c.addr = ln.Addr().String()
	srv := &http.Server{
		Handler:           c.handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	slog.Info("control", "listening", ln.Addr().String())
	go srv.Serve(ln)
	return nil
}

func (c *control) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", c.status)
	mux.HandleFunc("POST /mod", c.mod)
	mux.HandleFunc("POST /flag", c.flag)
	mux.HandleFunc("POST /reload", c.doReload)
	mux.HandleFunc("GET /keys", c.keyNames)
	mux.HandleFunc("GET /config", c.getConfig)
	mux.HandleFunc("PUT /config", c.putConfig)
	mux.HandleFunc("GET /mods", c.listMods)
	mux.HandleFunc("POST /mods", c.createMod)
	mux.HandleFunc("GET /mods/{name}", c.getMod)
	mux.HandleFunc("PUT /mods/{name}", c.putMod)
	mux.HandleFunc("DELETE /mods/{name}", c.deleteMod)
	mux.HandleFunc("GET /record", c.recordState)
	mux.HandleFunc("POST /record/start", c.recordStart)
	mux.HandleFunc("POST /record/stop", c.recordStop)
	mux.HandleFunc("GET /ui", c.login)
	mux.Handle("GET /ui/", http.StripPrefix("/ui/", http.FileServerFS(uiFiles)))
	return c.guard(mux)
}

func (c *control) tokenOK(s string) bool {
	return subtle.ConstantTimeCompare([]byte(s), []byte(c.token)) == 1
}

func (c *control) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if host != "127.0.0.1" && host != "localhost" && host != "[::1]" && host != "::1" {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if o := r.Header.Get("Origin"); o != "" && !strings.HasPrefix(o, "http://127.0.0.1") && !strings.HasPrefix(o, "http://localhost") {
			http.Error(w, "forbidden origin", http.StatusForbidden)
			return
		}
		switch {
		case r.URL.Path == "/ui":
		case c.tokenOK(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")):
		default:
			ck, err := r.Cookie("zboard")
			if err != nil || !c.tokenOK(ck.Value) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if r.Method != http.MethodGet && r.Header.Get("X-Requested-With") != "zboard" {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (c *control) login(w http.ResponseWriter, r *http.Request) {
	if !c.tokenOK(r.URL.Query().Get("token")) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		fmt.Fprintln(w, "zboard: open the UI with `zboard ui`, which carries the token")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "zboard", Value: c.token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	http.Redirect(w, r, "/ui/", http.StatusSeeOther)
}

func (c *control) status(w http.ResponseWriter, r *http.Request) {
	c.sel.mu.Lock()
	st := Status{Version: version.String, Process: c.sel.process, Flags: c.sel.eng.Flags(), Mods: []string{}, Table: "legend"}
	if m := c.sel.eng.Active(); m != nil {
		st.Active = m.Name
		if m.Table == merc.StandIn {
			st.Table = "standin"
		}
	}
	if c.sel.pinned != nil {
		st.Pinned = c.sel.pinned.Name
	}
	if c.sel.def != nil {
		st.Default = c.sel.def.Name
	}
	for _, m := range c.sel.mods {
		st.Mods = append(st.Mods, m.Name)
	}
	c.sel.mu.Unlock()
	st.LEDs.Enabled, st.LEDs.Action = c.sel.eng.LEDState()
	st.Attached = c.s == nil || c.s.keyboardAttached()
	if c.s != nil {
		st.Driver = c.s.driverState()
	}
	writeJSON(w, st)
}

func (c *control) mod(w http.ResponseWriter, r *http.Request) {
	if err := c.sel.pin(r.FormValue("name")); err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	c.status(w, r)
}

func (c *control) flag(w http.ResponseWriter, r *http.Request) {
	name := strings.ToLower(r.FormValue("name"))
	if name == "" {
		http.Error(w, "name required", http.StatusBadRequest)
		return
	}
	on := r.FormValue("on") == "1" || strings.EqualFold(r.FormValue("on"), "true") || strings.EqualFold(r.FormValue("on"), "on")
	c.sel.eng.SetFlag(name, on)
	c.status(w, r)
}

func (c *control) doReload(w http.ResponseWriter, r *http.Request) {
	if !c.reloadOr(w) {
		return
	}
	c.status(w, r)
}

func (c *control) reloadOr(w http.ResponseWriter) bool {
	names, err := c.reload()
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return false
	}
	slog.Info("mods reloaded", "mods", names)
	return true
}

func (c *control) keyNames(w http.ResponseWriter, r *http.Request) {
	var sources []string
	for _, k := range merc.Keys {
		if s := k.Source(); s != "" {
			sources = append(sources, s)
		}
	}
	names := make([]string, 0, len(keys.All))
	for _, k := range keys.All {
		names = append(names, k.Name)
	}
	sort.Strings(names)
	writeJSON(w, map[string][]string{"sources": append(sources, names...), "keys": names})
}

func (c *control) readConfig() (configFile, error) {
	var cfg configFile
	b, err := os.ReadFile(filepath.Join(c.dir, "config.yaml"))
	if err != nil && !os.IsNotExist(err) {
		return cfg, err
	}
	if err == nil {
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, fmt.Errorf("config.yaml: %w", err)
		}
	}
	cfg.Dir = c.dir
	return cfg, nil
}

func (c *control) getConfig(w http.ResponseWriter, r *http.Request) {
	cfg, err := c.readConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, cfg)
}

func (c *control) putConfig(w http.ResponseWriter, r *http.Request) {
	var cfg configFile
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&cfg); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if _, err := profile.ParseLEDs(cfg.LEDs.Enabled, cfg.LEDs.Action); err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	if cfg.TapHold != "" {
		if d, err := time.ParseDuration(cfg.TapHold); err != nil || d < 0 {
			http.Error(w, fmt.Sprintf("tap-hold %q is not a duration", cfg.TapHold), http.StatusUnprocessableEntity)
			return
		}
	}
	if cfg.Default != "" {
		if _, _, err := c.findMod(cfg.Default); err != nil {
			http.Error(w, "default: "+err.Error(), http.StatusUnprocessableEntity)
			return
		}
	}
	cfg.Dir = ""
	body, err := yaml.Marshal(cfg)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	header := "# zboard agent configuration; the web UI and `zboard` write this file.\n# Mods live in mods/*.yaml next to it. LED sources: mod, on, off, flag:<name>.\n"
	if err := os.WriteFile(filepath.Join(c.dir, "config.yaml"), append([]byte(header), body...), 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !c.reloadOr(w) {
		return
	}
	c.status(w, r)
}

func (c *control) modFiles() ([]modFile, error) {
	paths, _ := filepath.Glob(filepath.Join(c.dir, "mods", "*.yaml"))
	sort.Strings(paths)
	var out []modFile
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		d, err := profile.DecodeDocument(b)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		d.Name = strings.TrimSuffix(filepath.Base(p), ".yaml")
		out = append(out, modFile{Document: d, YAML: string(b), File: filepath.Base(p)})
	}
	return out, nil
}

func (c *control) findMod(name string) (modFile, []modFile, error) {
	all, err := c.modFiles()
	if err != nil {
		return modFile{}, nil, err
	}
	for _, m := range all {
		if strings.EqualFold(m.Document.Name, name) {
			return m, all, nil
		}
	}
	return modFile{}, all, fmt.Errorf("no Mod named %q", name)
}

func (c *control) listMods(w http.ResponseWriter, r *http.Request) {
	all, err := c.modFiles()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type entry struct {
		Name     string   `json:"name"`
		File     string   `json:"file"`
		Table    string   `json:"table"`
		Triggers []string `json:"triggers"`
	}
	out := []entry{}
	for _, m := range all {
		out = append(out, entry{m.Document.Name, m.File, m.Document.Table, m.Document.Triggers})
	}
	writeJSON(w, out)
}

func (c *control) getMod(w http.ResponseWriter, r *http.Request) {
	m, _, err := c.findMod(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, m)
}

type modBody struct {
	Name     string            `json:"name"`
	Document *profile.Document `json:"document"`
	YAML     *string           `json:"yaml"`
}

func (b modBody) name() string {
	n := b.Name
	if n == "" && b.Document != nil {
		n = b.Document.Name
	}
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(strings.TrimSpace(n)), "-"), "-.")
}

func (b modBody) validated() ([]byte, *profile.Mod, error) {
	var text []byte
	switch {
	case b.YAML != nil:
		text = []byte(*b.YAML)
	case b.Document != nil:
		var err error
		if text, err = b.Document.Encode(); err != nil {
			return nil, nil, err
		}
	default:
		return nil, nil, fmt.Errorf("document or yaml required")
	}
	m, err := profile.Parse(text)
	if err != nil {
		return nil, nil, err
	}
	return text, m, nil
}

func readModBody(r *http.Request) (modBody, error) {
	var b modBody
	err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(&b)
	return b, err
}

func (c *control) putMod(w http.ResponseWriter, r *http.Request) {
	cur, all, err := c.findMod(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	body, err := readModBody(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	text, _, err := body.validated()
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	name, file := cur.Document.Name, cur.File
	if n := body.name(); n != "" && !strings.EqualFold(n, name) {
		for _, other := range all {
			if strings.EqualFold(other.Document.Name, n) {
				http.Error(w, fmt.Sprintf("a Mod named %q exists", n), http.StatusConflict)
				return
			}
		}
		if err := os.Rename(filepath.Join(c.dir, "mods", cur.File), filepath.Join(c.dir, "mods", n+".yaml")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		name, file = n, n+".yaml"
	}
	c.writeMod(w, r, file, text, name)
}

var slugRe = regexp.MustCompile(`[^a-z0-9._-]+`)

func (c *control) createMod(w http.ResponseWriter, r *http.Request) {
	body, err := readModBody(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	text, _, err := body.validated()
	if err != nil {
		http.Error(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}
	name := body.name()
	if name == "" {
		http.Error(w, "a name is required: it becomes the file name", http.StatusBadRequest)
		return
	}
	if _, _, err := c.findMod(name); err == nil {
		http.Error(w, fmt.Sprintf("a Mod named %q exists", name), http.StatusConflict)
		return
	}
	w.WriteHeader(http.StatusCreated)
	c.writeMod(w, r, name+".yaml", text, name)
}

func (c *control) writeMod(w http.ResponseWriter, r *http.Request, file string, text []byte, name string) {
	dir := filepath.Join(c.dir, "mods")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := os.WriteFile(filepath.Join(dir, file), text, 0o644); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !c.reloadOr(w) {
		return
	}
	m, _, err := c.findMod(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, m)
}

func (c *control) deleteMod(w http.ResponseWriter, r *http.Request) {
	m, _, err := c.findMod(r.PathValue("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if err := os.Remove(filepath.Join(c.dir, "mods", m.File)); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if !c.reloadOr(w) {
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *control) recordState(w http.ResponseWriter, r *http.Request) {
	on, steps := c.rec.state()
	writeJSON(w, map[string]any{"recording": on, "steps": steps})
}

func (c *control) recordStart(w http.ResponseWriter, r *http.Request) {
	c.rec.start()
	c.recordState(w, r)
}

func (c *control) recordStop(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"recording": false, "steps": c.rec.stop()})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
