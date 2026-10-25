package profile

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

type Document struct {
	Name     string       `json:"name"`
	Table    string       `json:"table"`
	Triggers []string     `json:"triggers"`
	Menu     *bool        `json:"menu,omitempty"`
	LEDs     *DocLEDs     `json:"leds,omitempty"`
	Keys     []DocBinding `json:"keys"`
	Layers   []DocLayer   `json:"layers"`
}

type DocLEDs struct {
	Enabled string `json:"enabled" yaml:"enabled"`
	Action  string `json:"action" yaml:"action"`
}

type DocBinding struct {
	Source string `json:"source"`
	Action any    `json:"action"`
}

type DocLayer struct {
	Name string       `json:"name"`
	Keys []DocBinding `json:"keys"`
}

func DecodeDocument(b []byte) (*Document, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(b, &root); err != nil {
		return nil, err
	}
	d := &Document{Triggers: []string{}, Keys: []DocBinding{}, Layers: []DocLayer{}}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return d, nil
	}
	top := root.Content[0]
	if top.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("profile: the file is not a mapping")
	}
	for i := 0; i+1 < len(top.Content); i += 2 {
		key, val := top.Content[i], top.Content[i+1]
		var err error
		switch strings.ToLower(key.Value) {
		case "name":
		case "table":
			d.Table = val.Value
		case "triggers":
			err = val.Decode(&d.Triggers)
		case "menu":
			err = val.Decode(&d.Menu)
		case "leds":
			d.LEDs = &DocLEDs{}
			err = val.Decode(d.LEDs)
		case "keys":
			d.Keys, err = decodeBindings(val)
		case "layers":
			if val.Kind == yaml.MappingNode {
				for j := 0; j+1 < len(val.Content) && err == nil; j += 2 {
					var keys []DocBinding
					keys, err = decodeBindings(val.Content[j+1])
					d.Layers = append(d.Layers, DocLayer{Name: val.Content[j].Value, Keys: keys})
				}
			}
		}
		if err != nil {
			return nil, fmt.Errorf("profile: %s: %w", key.Value, err)
		}
	}
	if d.Triggers == nil {
		d.Triggers = []string{}
	}
	return d, nil
}

func decodeBindings(n *yaml.Node) ([]DocBinding, error) {
	out := []DocBinding{}
	if n.Kind != yaml.MappingNode {
		if n.Kind == yaml.ScalarNode && n.Tag == "!!null" {
			return out, nil
		}
		return nil, fmt.Errorf("not a mapping of keys")
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		var v any
		if err := n.Content[i+1].Decode(&v); err != nil {
			return nil, err
		}
		out = append(out, DocBinding{Source: n.Content[i].Value, Action: v})
	}
	return out, nil
}

func (d *Document) Encode() ([]byte, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	add := func(k string, v *yaml.Node) { root.Content = append(root.Content, str(k), v) }
	if d.Table != "" {
		add("table", str(d.Table))
	}
	if len(d.Triggers) > 0 {
		n := &yaml.Node{Kind: yaml.SequenceNode}
		for _, t := range d.Triggers {
			n.Content = append(n.Content, str(t))
		}
		add("triggers", n)
	}
	if d.Menu != nil {
		add("menu", &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: fmt.Sprint(*d.Menu)})
	}
	if d.LEDs != nil {
		add("leds", &yaml.Node{Kind: yaml.MappingNode,
			Content: []*yaml.Node{str("enabled"), str(d.LEDs.Enabled), str("action"), str(d.LEDs.Action)}})
	}
	keys, err := encodeBindings(d.Keys)
	if err != nil {
		return nil, err
	}
	add("keys", keys)
	if len(d.Layers) > 0 {
		layers := &yaml.Node{Kind: yaml.MappingNode}
		for _, l := range d.Layers {
			n, err := encodeBindings(l.Keys)
			if err != nil {
				return nil, fmt.Errorf("layer %s: %w", l.Name, err)
			}
			layers.Content = append(layers.Content, str(l.Name), n)
		}
		add("layers", layers)
	}
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return nil, err
	}
	return out.Bytes(), enc.Close()
}

func encodeBindings(bs []DocBinding) (*yaml.Node, error) {
	n := &yaml.Node{Kind: yaml.MappingNode}
	for _, b := range bs {
		var v yaml.Node
		if err := v.Encode(b.Action); err != nil {
			return nil, fmt.Errorf("%s: %w", b.Source, err)
		}
		n.Content = append(n.Content, str(b.Source), &v)
	}
	return n, nil
}

func str(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}
