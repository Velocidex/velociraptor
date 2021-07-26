//+build extras

package tools

import (
	"fmt"
	"math/rand"

	"github.com/Velocidex/json"

	"cloud.google.com/go/pubsub"
	"github.com/Velocidex/ordereddict"
	"golang.org/x/net/context"
	"google.golang.org/api/option"

	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/arg_parser"
)

type GCSPubsubPublishArgs struct {
	Topic       string      `vfilter:"required,field=topic,doc=The topic to publish to"`
	ProjectId   string      `vfilter:"required,field=project_id,doc=The project id to publish to"`
	Msg         vfilter.Any `vfilter:"required,field=msg,doc=Message to publish to Pubsub"`
	Credentials string      `vfilter:"required,field=credentials,doc=The credentials to use"`
}

type GCSPubsubPublishFunction struct{}

func (self *GCSPubsubPublishFunction) Call(ctx context.Context,
	scope vfilter.Scope,
	args *ordereddict.Dict) vfilter.Any {

	arg := &GCSPubsubPublishArgs{}
	err := arg_parser.ExtractArgsWithContext(ctx, scope, args, arg)
	if err != nil {
		scope.Log("gcs_pubsub_publish: %s", err.Error())
		return vfilter.Null{}
	}

	client, err := pubsub.NewClient(
		ctx,
		arg.ProjectId,
		option.WithCredentialsJSON([]byte(arg.Credentials)),
	)

	if err != nil {
		return fmt.Errorf("gcs_pubsub_publish: %v", err)
	}

	defer client.Close()

	t := client.Topic(arg.Topic)

	serialized, err := json.Marshal(arg.Msg)
	if err != nil {
		return fmt.Errorf("gcs_pubsub_publish: %v", err)
	}

	result := t.Publish(ctx, &pubsub.Message{
		Data: []byte(serialized),
		Attributes: map[string]string{
			"messageId": fmt.Sprintf("VRAPTOR-%d", rand.Intn(100000)),
		},
	})

	id, err := result.Get(ctx)
	if err != nil {
		return fmt.Errorf("Get: %v", err)
	}
	return id
}

func (self GCSPubsubPublishFunction) Info(
	scope vfilter.Scope, type_map *vfilter.TypeMap) *vfilter.FunctionInfo {
	return &vfilter.FunctionInfo{
		Name:    "gcs_pubsub_publish",
		Doc:     "Publish a message to Google PubSub.",
		ArgType: type_map.AddType(scope, &GCSPubsubPublishArgs{}),
	}
}

func init() {
	vql_subsystem.RegisterFunction(&GCSPubsubPublishFunction{})
}
