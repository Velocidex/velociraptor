package collector

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/Velocidex/ordereddict"
	"github.com/shirou/gopsutil/v3/host"
	"www.velocidex.com/golang/velociraptor/actions"
	actions_proto "www.velocidex.com/golang/velociraptor/actions/proto"
	api_proto "www.velocidex.com/golang/velociraptor/api/proto"
	"www.velocidex.com/golang/velociraptor/artifacts"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	crypto_proto "www.velocidex.com/golang/velociraptor/crypto/proto"
	"www.velocidex.com/golang/velociraptor/file_store/path_specs"
	"www.velocidex.com/golang/velociraptor/flows"
	flows_proto "www.velocidex.com/golang/velociraptor/flows/proto"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/reporting"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/launcher"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/vql"
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
	log_file         *reporting.ContainerResultSetWriter

	start_time         time.Time
	collection_context *flows.CollectionContext
	logger             *logWriter

	// The VQL requests we actuall collected. We store those in the
	// container for provenance.
	requests api_proto.ApiFlowRequestDetails

	output_chan chan vfilter.Row

	metadata []vfilter.Row

	format reporting.ContainerFormat

	scope vfilter.Scope
}

func (self *collectionManager) GetRepository(extra_artifacts vfilter.Any) (err error) {
	self.mu.Lock()
	defer self.mu.Unlock()

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
func (self *collectionManager) AddThrottler(
	ops_per_sec float64, cpu_limit float64, iops_limit float64,
	progress_timeout float64) {

	self.mu.Lock()
	defer self.mu.Unlock()

	throttler := actions.NewThrottler(self.ctx, self.scope,
		ops_per_sec, cpu_limit, iops_limit)

	if progress_timeout > 0 {
		throttler = actions.NewProgressThrottler(
			self.ctx, self.scope, self.cancel, throttler,
			time.Duration(progress_timeout*1e9)*time.Nanosecond)
	}

	self.scope.SetThrottler(throttler)
}

func (self *collectionManager) SetMetadata(metadata vfilter.StoredQuery) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.metadata = types.Materialize(self.ctx, self.scope, metadata)
}

func (self *collectionManager) SetFormat(
	format reporting.ContainerFormat) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.format = format
	return nil
}

func (self *collectionManager) storeHostInfo() error {
	return nil

	fd, err := self.container.Create("info.json", Clock.Now())
	if err != nil {
		return err
	}
	defer fd.Close()

	version := config.GetVersion()
	var info_dict *ordereddict.Dict
	host_info, err := host.Info()
	if err != nil {
		info_dict = ordereddict.NewDict()
	} else {
		info_dict = vql.GetInfo(host_info)
	}

	fd.Write(json.MustMarshalIndent(info_dict.
		Set("Name", version.Name).
		Set("BuildTime", version.BuildTime).
		Set("build_url", version.CiBuildUrl)))

	return nil
}

func (self *collectionManager) storeCollectionMetadata() error {
	fd, err := self.container.Create("requests.json", Clock.Now())
	if err != nil {
		return err
	}

	fd.Write(json.MustMarshalIndent(self.requests))
	fd.Close()

	if len(self.custom_artifacts) > 0 {
		fd, err := self.container.Create("custom_artifacts.json", Clock.Now())
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

	query_start_time := Clock.Now()

	status := &crypto_proto.VeloStatus{
		Status: crypto_proto.VeloStatus_OK,
	}

	defer func() {
		status.Duration = Clock.Now().UnixNano() - query_start_time.UnixNano()

		self.collection_context.QueryStats = append(
			self.collection_context.QueryStats, status)
		self.collection_context.TotalCollectedRows += uint64(status.ResultRows)
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
		self.config_obj, self.ctx, subscope, query,
		path_specs.NewUnsafeFilestorePath("results"),
		self.format)

	status.LogRows = int64(self.logger.Count())
	status.ResultRows = int64(total_rows)

	if err != nil {
		status.Status = crypto_proto.VeloStatus_GENERIC_ERROR
		status.ErrorMessage = err.Error()
		return err
	}

	if query.Name != "" && total_rows > 0 {
		status.NamesWithResponse = append(status.NamesWithResponse, query.Name)
		subscope.Log("Collected %v rows for %s", total_rows, query.Name)
	}

	return nil
}

func (self *collectionManager) Collect(request *flows_proto.ArtifactCollectorArgs) error {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.start_time = Clock.Now()
	self.collection_context.Request = request

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
	for request_number, vql_request := range vql_requests {

		// Emulate the same type of requests a client would receive so
		// the import is smoother.
		self.requests.Items = append(self.requests.Items,
			&crypto_proto.VeloMessage{
				SessionId:       self.collection_context.SessionId,
				RequestId:       uint64(request_number + 1),
				VQLClientAction: vql_request})

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

func (self *collectionManager) SetTimeout(ns float64) {
	go func() {
		start := Clock.Now()

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

func (self *collectionManager) MakeContainer(filename, password string, level int64) (err error) {
	self.mu.Lock()
	defer self.mu.Unlock()

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
	self.log_file, err = reporting.NewResultSetWriter(
		self.container, "log.json")
	return err
}

func (self *collectionManager) Close() error {
	self.mu.Lock()
	defer self.mu.Unlock()

	if self.container == nil || self.container.IsClosed() {
		return nil
	}

	if self.log_file != nil {
		self.log_file.Close()
	}

	fd, err := self.container.Create("collection_context.json", Clock.Now())
	if err == nil {
		self.collection_context.StartTime = uint64(self.start_time.UnixNano())
		self.collection_context.CreateTime = uint64(self.start_time.UnixNano())

		flows.UpdateFlowStats(self.collection_context)

		// Merge in the container stats
		container_stats := self.container.Stats()
		self.collection_context.TotalUploadedFiles = container_stats.TotalUploadedFiles
		self.collection_context.TotalUploadedBytes = container_stats.TotalUploadedBytes
		self.collection_context.TotalExpectedUploadedBytes = container_stats.TotalUploadedBytes

		fd.Write([]byte(json.MustMarshalIndent(self.collection_context)))
		fd.Close()
	}

	// Record the collection metadata.
	err = self.storeCollectionMetadata()
	if err != nil {
		return err
	}

	// Store host information
	err = self.storeHostInfo()
	if err != nil {
		return err
	}

	// Finalize the container now.
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
	log_file     *reporting.ContainerResultSetWriter
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
	now := int(Clock.Now().Unix())
	return self.log_file.WriteJSONL([]byte(json.Format(
		"{\"_ts\":%d,\"client_time\":%d,\"level\":%q,\"message\":%q}\n",
		now, now, level, msg)))
}
