package config

import (
	"bytes"
	"fmt"
	"strings"
)

type (
	Virtual string

	Directive struct {
		Line    int        `json:"line"`
		Virtual Virtual    `json:"virtual,omitempty"`
		Name    string     `json:"name"`
		Args    []string   `json:"args,omitempty"`
		Body    Directives `json:"body,omitempty"`
	}
	Directives        []*Directive
	DirectiveIterator struct {
		items   *Directives
		current int
	}
	Configuration = Directive
)

func NewDirective(name string, args ...string) *Directive {
	return &Directive{Name: name, Args: args}
}

func (d *Directive) String() string {
	return d.Pretty(0)
}

func (d *Directive) HasArgs() bool {
	return len(d.Args) > 0
}

func (d *Directive) BodyBytes() []byte {
	out := bytes.NewBufferString("")
	for _, body := range d.Body {
		out.WriteString(body.Pretty(0))
		out.WriteString("\n")
	}
	return out.Bytes()
}

func (d *Directive) noBody() bool {
	if len(d.Body) == 0 {
		return true
	} else {
		for _, body := range d.Body {
			if body.Virtual == "" {
				return false
			}
		}
		return true
	}
}

func (d *Directive) AddBody(name string, args ...string) *Directive {
	body := NewDirective(name, args...)
	d.AddBodyDirective(body)
	return body
}

func (d *Directive) AddBodyDirective(directive ...*Directive) {
	if d.Body == nil {
		d.Body = make([]*Directive, 0)
	}
	d.Body = append(d.Body, directive...)
}

func (d *Directive) Pretty(prefix int) string {
	prefixString := strings.Repeat(" ", prefix*4)
	if d.Name == "#" {
		return fmt.Sprintf("%s%s", prefixString, d.Args[0])
	} else if d.Virtual != "" {
		return ""
	} else {
		out := bytes.NewBufferString(prefixString)
		out.WriteString(d.Name)
		out.WriteString(" ")
		if len(d.Args) > 0 {
			out.WriteString(strings.Join(d.Args, " "))
		}

		if d.noBody() {
			out.WriteString(";")
		} else {
			out.WriteString(" {")
			for _, body := range d.Body {
				out.WriteString("\n")
				out.WriteString(body.Pretty(prefix + 1))
			}
			out.WriteString(fmt.Sprintf("\n%s}", prefixString))
		}
		return out.String()
	}
}

func (ds *Directives) Get(name string) *Directive {
	for _, d := range *ds {
		if d.Name == name {
			return d
		}
	}
	return nil
}

func (ds *Directives) Remove(name string) *Directive {
	for i, d := range *ds {
		if d.Name == name {
			*ds = append((*ds)[0:i], (*ds)[i+1:]...)
			return d
		}
	}
	return nil
}

func (ds *Directives) Iterator() *DirectiveIterator {
	return &DirectiveIterator{
		current: -1, items: ds,
	}
}

func (ds *DirectiveIterator) Next() *Directive {
	ds.current++
	return (*ds.items)[ds.current]
}

func (ds *DirectiveIterator) HasNext() bool {
	return len(*ds.items) > ds.current+1
}

func (ds *DirectiveIterator) Remove() {
	*ds.items = append((*ds.items)[0:ds.current], (*ds.items)[ds.current+1:]...)
	ds.current--
}
