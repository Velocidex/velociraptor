/*

  Velociraptor's context extends the standard Go context to provide
  resource management capabilities.

*/
package context

import (
	"context"
	"time"
	"www.velocidex.com/golang/velociraptor/config"
)

type Context struct {
	ctx    context.Context
	Config *config.Config
}

func (self *Context) Done() <-chan struct{} {
	return self.ctx.Done()
}

func (self *Context) Deadline() (deadline time.Time, ok bool) {
	t, ok := self.ctx.Deadline()
	return t, ok
}

func (self *Context) Err() error {
	return self.ctx.Err()
}

func (self *Context) Value(key interface{}) interface{} {
	return self.ctx.Value(key)
}

func (self *Context) WithCancel() (ctx Context, cancel context.CancelFunc) {
	new_ctx, cancel_func := context.WithCancel(self.ctx)
	return Context{new_ctx, self.Config}, cancel_func
}

func Background() Context {
	return Context{
		ctx:    context.Background(),
		Config: config.GetDefaultConfig(),
	}
}

func BackgroundFromConfig(config_obj *config.Config) *Context {
	return &Context{
		ctx:    context.Background(),
		Config: config_obj,
	}
}
