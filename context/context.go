/*

  Velociraptor's context extends the standard Go context to provide
  resource management capabilities.

*/
package context

import (
	"context"
)

type Context struct {
	ctx context.Context
}


func Background() Context {
	return Context{ctx: context.Background()}
}
