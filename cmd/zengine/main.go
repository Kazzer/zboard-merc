package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/systray"
	"gopkg.in/yaml.v3"

	"github.com/Kazzer/zboard-merc/internal/engine"
	"github.com/Kazzer/zboard-merc/internal/profile"
	"github.com/Kazzer/zboard-merc/internal/version"
)

type Config struct {
	Default string `yaml:"default"`
	Device  string `yaml:"device"`
	Control string `yaml:"control"`
	Tray    string `yaml:"tray"`
	TapHold string `yaml:"tap-hold"`
	LEDs    struct {
		Enabled string `yaml:"enabled"`
		Action  string `yaml:"action"`
	} `yaml:"leds"`
	pin string
	dir string
}

func configDir() string {
	if d := os.Getenv("ZBOARD_CONFIG"); d != "" {
		return d
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "zboard")
	}
	if d := os.Getenv("APPDATA"); d != "" {
		return filepath.Join(d, "zboard")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "zboard")
}

func loadConfig(dir string) (Config, []*profile.Mod, error) {
	var cfg Config
	if b, err := os.ReadFile(filepath.Join(dir, "config.yaml")); err == nil {
		if err := yaml.Unmarshal(b, &cfg); err != nil {
			return cfg, nil, fmt.Errorf("config.yaml: %w", err)
		}
	}
	if cfg.TapHold != "" {
		d, err := time.ParseDuration(cfg.TapHold)
		if err != nil || d < 0 {
			return cfg, nil, fmt.Errorf("config.yaml: tap-hold %q is not a duration", cfg.TapHold)
		}
		profile.DefaultTapHold = d
	}
	paths, _ := filepath.Glob(filepath.Join(dir, "mods", "*.yaml"))
	sort.Strings(paths)
	var mods []*profile.Mod
	files := map[string]string{}
	for _, p := range paths {
		m, err := profile.Load(p)
		if err != nil {
			return cfg, nil, fmt.Errorf("%s: %w", filepath.Base(p), err)
		}
		if other, dup := files[strings.ToLower(m.Name)]; dup {
			return cfg, nil, fmt.Errorf("%s and %s both define a Mod named %q", other, filepath.Base(p), m.Name)
		}
		files[strings.ToLower(m.Name)] = filepath.Base(p)
		mods = append(mods, m)
	}
	return cfg, mods, nil
}

type scheduler struct{}

func (scheduler) After(d time.Duration, fn func()) func() {
	t := time.AfterFunc(d, fn)
	return func() { t.Stop() }
}

type selector struct {
	mu      sync.Mutex
	eng     *engine.Engine
	mods    []*profile.Mod
	def     *profile.Mod
	pinned  *profile.Mod
	process string
}

func (s *selector) replace(mods []*profile.Mod, def string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pinned := ""
	if s.pinned != nil {
		pinned = s.pinned.Name
	}
	s.mods = mods
	s.def, s.pinned = s.byName(def), s.byName(pinned)
	s.apply()
}

type session struct {
	eng      *engine.Engine
	sel      *selector
	ctl      *control
	rec      *recorder
	reload   func() ([]string, error)
	attached func() bool
	driver   func() bool
}

func (s *session) keyboardAttached() bool { return s.attached == nil || s.attached() }

func (s *session) driverState() string {
	switch {
	case s.driver == nil:
		return ""
	case s.driver():
		return "connected"
	}
	return "absent"
}

func (s *session) handle(ev engine.Event) {
	s.rec.observe(ev)
	s.eng.Handle(ev)
}

func (s *session) uiLink() string {
	if s.ctl == nil {
		return ""
	}
	return s.ctl.uiLink()
}

func (s *selector) pin(name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if name == "" {
		s.pinned = nil
	} else if m := s.byName(name); m != nil {
		s.pinned = m
	} else {
		return fmt.Errorf("no Mod named %q", name)
	}
	s.apply()
	return nil
}

func startControl(cfg Config, sel *selector, rec *recorder) (*control, func() ([]string, error), error) {
	reload := func() ([]string, error) {
		c, mods, err := loadConfig(cfg.dir)
		if err != nil {
			return nil, err
		}
		leds, err := profile.ParseLEDs(c.LEDs.Enabled, c.LEDs.Action)
		if err != nil {
			return nil, fmt.Errorf("config.yaml leds: %w", err)
		}
		sel.replace(mods, c.Default)
		sel.eng.SetLEDDefaults(leds)
		declareFlags(sel.eng, c, mods)
		var names []string
		for _, m := range mods {
			names = append(names, m.Name)
		}
		return names, nil
	}
	ctl, err := newControl(cfg.dir, sel, rec, reload)
	if err != nil {
		slog.Warn("control API off", "error", err)
		return nil, reload, nil
	}
	if err := ctl.serve(cfg.Control); err != nil {
		return nil, reload, fmt.Errorf("control address %s: %w (is another zengine running?)", cfg.Control, err)
	}
	return ctl, reload, nil
}

func trayEnabled(cfg Config) bool {
	switch strings.ToLower(cfg.Tray) {
	case "off", "no", "false":
		return false
	case "on", "yes", "true":
		return true
	}
	return trayAvailable()
}

func (s *selector) byName(name string) *profile.Mod {
	for _, m := range s.mods {
		if strings.EqualFold(m.Name, name) {
			return m
		}
	}
	return nil
}

func (s *selector) foreground(process string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.process = process
	s.apply()
}

func (s *selector) request(which string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur := s.eng.Active()
	idx := -1
	for i, m := range s.mods {
		if m == cur {
			idx = i
		}
	}
	switch strings.ToLower(which) {
	case "next":
		if len(s.mods) > 0 {
			s.pinned = s.mods[(idx+1)%len(s.mods)]
		}
	case "previous":
		if len(s.mods) > 0 {
			s.pinned = s.mods[(idx-1+len(s.mods))%len(s.mods)]
		}
	default:
		s.pinned = s.byName(which)
	}
	s.apply()
}

func (s *selector) apply() {
	if m := engine.Select(s.mods, s.process); m != nil {
		s.pinned = nil
		s.activate(m, true)
		return
	}
	if s.pinned != nil {
		s.activate(s.pinned, false)
		return
	}
	s.activate(s.def, false)
}

func (s *selector) activate(m *profile.Mod, byTrigger bool) {
	if m != s.eng.Active() {
		name := "bare keyboard"
		if m != nil {
			name = m.Name
		}
		slog.Info("mod", "active", name, "trigger", byTrigger, "process", s.process)
	}
	s.eng.Activate(m, byTrigger)
}

func orMod(s string) string {
	if s == "" {
		return "mod"
	}
	return s
}

func declareFlags(eng *engine.Engine, cfg Config, mods []*profile.Mod) {
	var names []string
	ledFlags := func(l profile.LEDs) {
		for _, s := range []profile.LEDSource{l.Enabled, l.Action} {
			if s.Kind == profile.LEDFlag {
				names = append(names, s.Flag)
			}
		}
	}
	ledFlags(ledDefaults(cfg))
	for _, m := range mods {
		if m.LEDs != nil {
			ledFlags(*m.LEDs)
		}
		layers := []profile.Layer{m.Base}
		for _, l := range m.Layers {
			layers = append(layers, l)
		}
		for _, l := range layers {
			for _, b := range l {
				if b.Toggle != "" {
					names = append(names, b.Toggle)
				}
			}
		}
	}
	eng.Declare(names...)
}

func ledDefaults(cfg Config) profile.LEDs {
	l, _ := profile.ParseLEDs(cfg.LEDs.Enabled, cfg.LEDs.Action)
	return l
}

func main() {
	check := flag.Bool("check", false, "load the configuration and the Mods, report, and exit")
	dir := flag.String("config", configDir(), "configuration directory")
	pin := flag.String("mod", "", "activate this Mod at start as if a key had selected it (triggers still win)")
	debug := flag.Bool("debug", os.Getenv("ZBOARD_DEBUG") != "", "log every key event and LED write (ZBOARD_DEBUG=1 does the same)")
	logPath := flag.String("log", "", "write the log to this file as well as to stderr (truncated at start)")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println("zengine", version.String)
		return
	}
	level := slog.LevelInfo
	if *debug {
		level = slog.LevelDebug
	}
	var logOut io.Writer = os.Stderr
	if *logPath != "" {
		f, err := os.Create(*logPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "log file:", err)
			os.Exit(1)
		}
		defer f.Close()
		logOut = io.MultiWriter(os.Stderr, f)
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(logOut, &slog.HandlerOptions{Level: level})))
	cfg, mods, err := loadConfig(*dir)
	if err != nil {
		slog.Error("configuration", "error", err)
		os.Exit(1)
	}
	for _, m := range mods {
		slog.Info("mod loaded", "name", m.Name, "table", m.Table, "triggers", m.Triggers, "keys", len(m.Base), "layers", len(m.Layers))
	}
	leds, err := profile.ParseLEDs(cfg.LEDs.Enabled, cfg.LEDs.Action)
	if err != nil {
		slog.Error("config.yaml leds", "error", err)
		os.Exit(1)
	}
	if *check {
		fmt.Printf("%d Mod(s) in %s; default %q; device %q; leds enabled=%s action=%s; tray %v\n", len(mods), *dir, cfg.Default, cfg.Device,
			orMod(cfg.LEDs.Enabled), orMod(cfg.LEDs.Action), trayEnabled(cfg))
		return
	}
	_ = leds
	if *pin != "" {
		found := false
		for _, m := range mods {
			found = found || strings.EqualFold(m.Name, *pin)
		}
		if !found {
			slog.Error("-mod names a Mod that is not loaded", "name", *pin)
			os.Exit(1)
		}
	}
	cfg.pin = *pin
	cfg.dir = *dir
	if cfg.Control == "" {
		cfg.Control = "127.0.0.1:47319"
	}
	stop := make(chan struct{})
	var once sync.Once
	quit := func() { once.Do(func() { close(stop) }) }
	if !trayEnabled(cfg) {
		if err := run(cfg, mods, stop, nil); err != nil {
			slog.Error("agent", "error", err)
			os.Exit(1)
		}
		return
	}
	ui := newTrayUI(stop, quit)
	done := make(chan error, 1)
	go func() {
		err := run(cfg, mods, stop, ui.attach)
		select {
		case <-ui.readyCh:
		case <-time.After(3 * time.Second):
		}
		done <- err
		systray.Quit()
	}()
	systray.Run(ui.onReady, nil)
	if err := <-done; err != nil {
		slog.Error("agent", "error", err)
		os.Exit(1)
	}
}
