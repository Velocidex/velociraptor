package utils

import (
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Velocidex/ordereddict"
)

type Stringer interface {
	String() string
}

type ServeMux struct {
	*http.ServeMux

	Handlers map[string]http.Handler
}

func (self *ServeMux) Handle(pattern string, handler http.Handler) {
	self.Handlers[pattern] = handler
	self.ServeMux.Handle(pattern, handler)
}

func (self *ServeMux) Debug() *ordereddict.Dict {
	res := ordereddict.NewDict()
	var keys []string
	for k := range self.Handlers {
		keys = append(keys, k)
	}

	sort.Strings(keys)

	for _, k := range keys {
		v, ok := self.Handlers[k]
		if !ok {
			continue
		}

		name := fmt.Sprintf("%T", v)
		stringer, ok := v.(Stringer)
		if ok {
			name = stringer.String()
		}

		parts := strings.Split(name, ":")
		res.Set(k, parts)
	}
	return res
}

func NewServeMux() *ServeMux {
	return &ServeMux{
		ServeMux: http.NewServeMux(),
		Handlers: make(map[string]http.Handler),
	}
}

type HandlerFuncContainer struct {
	http.HandlerFunc
	callSite string
	parent   *HandlerFuncContainer
}

func (self *HandlerFuncContainer) String() string {
	res := self.callSite
	if self.parent != nil {
		res += ": " + self.parent.String()
	}

	return res
}

func (self *HandlerFuncContainer) AddChild(note string) *HandlerFuncContainer {
	if self.callSite != "" {
		child := &HandlerFuncContainer{
			HandlerFunc: self.HandlerFunc,
			parent:      self,
			callSite:    note,
		}
		return child
	}
	self.callSite = note
	return self
}

func HandlerFunc(parent http.Handler, f http.HandlerFunc) *HandlerFuncContainer {
	res := &HandlerFuncContainer{
		HandlerFunc: http.HandlerFunc(f),
	}

	if parent != nil {
		parent_handler, ok := parent.(*HandlerFuncContainer)
		if ok {
			res.parent = parent_handler
		} else {
			res.parent = &HandlerFuncContainer{
				callSite: fmt.Sprintf("%T", parent),
			}
		}
	}

	pc, _, _, ok := runtime.Caller(1)
	if ok {
		details := runtime.FuncForPC(pc)
		if details != nil {
			res.callSite = filepath.Base(details.Name())
		}
	}

	return res
}

func StripPrefix(prefix string, h http.Handler) http.Handler {
	handler := http.StripPrefix(prefix, h)

	return HandlerFunc(h, handler.ServeHTTP)
}
