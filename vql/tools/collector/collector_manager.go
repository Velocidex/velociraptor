package collector

import (
	"context"
	"fmt"
	"io"
	"log"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql/acl_managers"
	"www.velocidex.com/golang/vfilter"
	"www.velocidex.com/golang/vfilter/types"
)

type collectionManager struct {
	mu sync.Mutex

	ctx    context.Context
	cancel func()

	config_obj *config_proto.Config

	repository       services.Repository
	custom_artifacts []*artifacts_proto.Artifact
	container        *reporting.Container
	Output           string
	log_file         io.WriteCloser

	start_time         time.Time
	collection_context *flows.CollectionContext
	logger             *logWriter

	output_chan chan vfilter.Row

	metadata []vfilter.Row

	format string

	scope vfilter.Scope
}

func (self *collectionManager) getRepository(extra_artifacts vfilter.Any) (err error) {
	manager, err := services.GetRepositoryManager(self.config_obj)
	if err != nil {
		return err
	}

	self.repository, err = manager.GetGlobalRepository(self.config_obj)
	if err != nil {
		return err
	}

	if utils.IsNil(extra_artifacts) {
		return nil
	}

	// Private copy of the repository.
	self.repository = self.repository.Copy()

	loader := func(item *ordereddict.Dict) error {
		serialized, err := json.Marshal(item)
		if err != nil {
			return err
		}

		artifact, err := self.repository.LoadYaml(
			string(serialized), services.ValidateArtifact,
			!services.ArtifactIsBuiltIn)
		if err != nil {
			return err
		}

		self.custom_artifacts = append(self.custom_artifacts, artifact)

		// Check if we are allows to add these artifacts
		err = CheckArtifactModification(self.scope, artifact)
		if err != nil {
			return err
		}

		return nil
	}

	switch t := extra_artifacts.(type) {
	case []*ordereddict.Dict:
		for _, item := range t {
			err := loader(item)
			if err != nil {
				return err
			}
		}

	case *ordereddict.Dict:
		err := loader(t)
		if err != nil {
			return err
		}

	case []string:
		for _, item := range t {
			artifact, err := self.repository.LoadYaml(item,
				services.ValidateArtifact, !services.ArtifactIsBuiltIn)
			if err != nil {
				return err
			}

			err = CheckArtifactModification(self.scope, artifact)
			if err != nil {
				return err
			}
		}

	case string:
		artifact, err := self.repository.LoadYaml(t,
			services.ValidateArtifact, !services.ArtifactIsBuiltIn)
		if err != nil {
			return err
		}

		err = CheckArtifactModification(self.scope, artifact)
		if err != nil {
			return err
		}
	}

	return nil
}

// Install throttler into the scope.
func (self *collectionManager) addThrottler(
	ops_per_sec float64, cpu_limit float64, iops_limit float64,
	progress_timeout float64) {

	throttler := actions.NewThrottler(self.ctx, self.scope,
		ops_per_sec, cpu_limit, iops_limit)

	if progress_timeout > 0 {
		throttler = actions.NewProgressThrottler(
			self.ctx, self.scope, self.cancel, throttler,
			time.Duration(progress_timeout*1e9)*time.Nanosecond)
	}

	self.scope.SetThrottler(throttler)
}

func (self *collectionManager) setMetadata(metadata vfilter.StoredQuery) {
	self.metadata = types.Materialize(self.ctx, self.scope, metadata)
}

func (self *collectionManager) setFormat(format string) error {
	switch format {
	case "jsonl", "csv", "json":
	case "":
		format = "jsonl"
	default:
		return fmt.Errorf("format %v not supported", format)
	}

	self.format = format
	return nil
}

func (self *collectionManager) storeCollectionMetadata(request *flows_proto.ArtifactCollectorArgs) error {
	fd, err := self.container.Create("request.json", time.Now())
	if err != nil {
		return err
	}

	fd.Write(json.MustMarshalIndent(request))
	fd.Close()

	if len(self.custom_artifacts) > 0 {
		fd, err := self.container.Create("custom_artifacts.json", time.Now())
		if err != nil {
			return err
		}

		fd.Write(json.MustMarshalIndent(self.custom_artifacts))
		fd.Close()
	}

	return nil
}

func (self *collectionManager) collectQuery(
	subscope vfilter.Scope, query *actions_proto.VQLRequest) (err error) {

	query_start_time := time.Now()

	status := &crypto_proto.VeloStatus{
		Status: crypto_proto.VeloStatus_OK,
	}

	defer func() {
		self.mu.Lock()
		defer self.mu.Unlock()

		status.Duration = time.Now().UnixNano() - query_start_time.UnixNano()

		self.collection_context.QueryStats = append(
			self.collection_context.QueryStats, status)
	}()

	// Useful to know what is going on with the collection.
	if query.Name != "" {
		subscope.Log("Starting collection of %s", query.Name)
	}

	// If there is no container we just
	// return the rows to our caller.
	if self.container == nil {
		query_log := actions.QueryLog.AddQuery(query.VQL)

		vql, err := vfilter.Parse(query.VQL)
		if err != nil {
			status.Status = crypto_proto.VeloStatus_GENERIC_ERROR
			status.ErrorMessage = err.Error()
			return err
		}
		for row := range vql.Eval(self.ctx, subscope) {
			status.ResultRows++
			select {
			case <-self.ctx.Done():
				return nil
			case self.output_chan <- row:
			}
		}
		query_log.Close()

		status.LogRows = int64(self.logger.Count())

		return nil
	}

	total_rows, err := self.container.StoreArtifact(
		self.config_obj, self.ctx, subscope, query, self.format)
	if err != nil {
		status.Status = crypto_proto.VeloStatus_GENERIC_ERROR
		status.ErrorMessage = err.Error()
		return err
	}

	if query.Name != "" && total_rows > 0 {
		status.NamesWithResponse = append(status.NamesWithResponse, query.Name)
		subscope.Log("Collected %v rows for %s", total_rows, query.Name)
	}

	status.LogRows = int64(self.logger.Count())

	return nil
}

func (self *collectionManager) collect(request *flows_proto.ArtifactCollectorArgs) error {

	self.start_time = time.Now()
	self.collection_context.Request = request

	// Record the exact request in the collection for provenance.
	if self.container != nil {
		err := self.storeCollectionMetadata(request)
		if err != nil {
			return err
		}
	}
	// Create a sub scope to run the new collection in - based on our
	// existing scope but override the uploader with the container.
	builder := services.ScopeBuilderFromScope(self.scope)
	builder.Uploader = self.container

	if self.log_file != nil {
		self.logger = &logWriter{
			parent_scope: self.scope, log_file: self.log_file,
		}

		builder.Logger = log.New(self.logger, "", 0)
	}

	// When run within an ACL context, copy the ACL manager to the
	// subscope - otherwise the user can bypass the ACL manager and
	// get more permissions.
	acl_manager, ok := artifacts.GetACLManager(self.scope)
	if !ok {
		acl_manager = acl_managers.NullACLManager{}
	}

	launcher, err := services.GetLauncher(self.config_obj)
	if err != nil {
		return err
	}

	vql_requests, err := launcher.CompileCollectorArgs(
		self.ctx, self.config_obj, acl_manager, self.repository,
		services.CompilerOptions{}, request)
	if err != nil {
		return err
	}

	// Run each collection separately, one after the other.
	for _, vql_request := range vql_requests {

		// Make a new scope for each artifact.
		manager, err := services.GetRepositoryManager(self.config_obj)
		if err != nil {
			return err
		}

		// Create a new environment for each request.
		env := ordereddict.NewDict()
		for _, env_spec := range vql_request.Env {
			env.Set(env_spec.Key, env_spec.Value)
		}

		subscope := manager.BuildScope(builder)
		subscope.AppendVars(env)
		defer subscope.Close()

		self.collection_context.TotalRequests = int64(len(vql_request.Query))

		// Run each query and store the results in the container
		for _, query := range vql_request.Query {
			err := self.collectQuery(subscope, query)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (self *collectionManager) setTimeout(ns float64) {
	go func() {
		start := time.Now()

		select {
		case <-self.ctx.Done():
			return

		case <-time.After(time.Duration(ns) * time.Nanosecond):
			self.scope.Log("collect: <red>Timeout Error:</> Collection timed out after %v",
				time.Now().Sub(start))
			// Cancel the main context.
			self.cancel()
		}
	}()
}

func newCollectionManager(
	ctx context.Context,
	config_obj *config_proto.Config,
	output_chan chan vfilter.Row,
	scope vfilter.Scope) *collectionManager {
	subctx, cancel := context.WithCancel(ctx)

	return &collectionManager{
		ctx:                subctx,
		cancel:             cancel,
		config_obj:         config_obj,
		collection_context: flows.NewCollectionContext(config_obj),
		output_chan:        output_chan,
		scope:              scope,
	}
}

func (self *collectionManager) makeContainer(filename, password string, level int64) (err error) {
	// Should we encrypt it?
	if password != "" {
		self.scope.Log("Will password protect container")
	}

	self.scope.Log("Setting compression level to %v", level)

	self.Output = filename
	self.container, err = reporting.NewContainer(
		self.config_obj, filename, password, level, self.metadata)
	if err != nil {
		return err
	}

	self.scope.Log("Will create container at %s", filename)

	self.collection_context.SessionId = launcher.NewFlowId("")
	self.log_file, err = self.container.Create("log.json", time.Now())
	return err
}

func (self *collectionManager) Close() error {
	if self.container == nil || self.container.IsClosed() {
		return nil
	}

	if self.log_file != nil {
		_ = self.log_file.Close()
	}

	fd, err := self.container.Create("collection_context.json", time.Now())
	if err == nil {
		self.collection_context.StartTime = uint64(self.start_time.UnixNano())
		self.collection_context.CreateTime = uint64(self.start_time.UnixNano())

		flows.UpdateFlowStats(self.collection_context)

		fd.Write([]byte(json.MustMarshalIndent(self.collection_context)))
		fd.Close()
	}

	err = self.container.Close()

	// Emit the result set for consumption by the
	// rest of the query.
	select {
	case <-self.ctx.Done():
		return err

	case self.output_chan <- ordereddict.NewDict().
		Set("Container", self.Output):
	}

	return err
}

type logWriter struct {
	mu           sync.Mutex
	parent_scope vfilter.Scope
	log_file     io.WriteCloser
	count        int
}

func (self *logWriter) Count() int {
	if self == nil {
		return 0
	}

	self.mu.Lock()
	defer self.mu.Unlock()

	return self.count
}

func (self *logWriter) Write(b []byte) (int, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.parent_scope.Log("%s", string(b))
	self.count++

	level, msg := logging.SplitIntoLevelAndLog(b)
	now := int(time.Now().Unix())
	return self.log_file.Write([]byte(json.Format(
		"{\"_ts\":%d,\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
		now, now, level, msg)))
}
