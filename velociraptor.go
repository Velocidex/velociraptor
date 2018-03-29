/*

  Velociraptor is a tool for collecting host based state information
  using Velocidex Query Language (VQL) queries.

  The primary use case for Velociraptor is for incident
  response/detection and host based inventory management.

*/

package velociraptor

import (
	"www.velocidex.com/golang/vfilter"
)

func MakeScope() *vfilter.Scope {
	return vfilter.NewScope().AppendPlugins(
		MakePslistPlugin(),
		MakeUsersPlugin(),
		MakeInfoPlugin(),
		MakeConnectionsPlugin(),
		GlobPlugin{},
	).AddProtocolImpl(
		_ProcessFieldImpl{},
	)
}
