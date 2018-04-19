/*

  Velociraptor's context extends the standard Go context to provide
  resource management capabilities.

*/
package context

import (
	"context"
	"www.velocidex.com/golang/velociraptor/config"
)

type Context struct {
	ctx context.Context
	Config config.Config
}


func (self *Context) Done() <- chan struct{} {
	return self.ctx.Done()
}



func Background() Context {
	return Context{ctx: context.Background()}
}
