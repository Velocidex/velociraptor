package repository

/*
   Velociraptor - Dig Deeper
   Copyright (C) 2019-2025 Rapid7 Inc.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published
   by the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Velocidex/yaml/v2"
	"github.com/go-errors/errors"
	"google.golang.org/protobuf/proto"
	"www.velocidex.com/golang/velociraptor/acls"
	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/logging"
	"www.velocidex.com/golang/velociraptor/paths"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	vql_subsystem "www.velocidex.com/golang/velociraptor/vql"
	"www.velocidex.com/golang/vfilter"
)

// Holds multiple artifact definitions.
type Repository struct {
	mu       sync.Mutex
	Data     map[string]*artifacts_proto.Artifact
	metadata *metadataManager

	// Each repository may have a parent - we search for the artifact
	// in our parents as well.
	parent            services.Repository
	parent_config_obj *config_proto.Config
}

// A Parent repository is another repository we can delegate to if we
// dont have the artifact definition we require.
func (self *Repository) SetParent(
	parent services.Repository, parent_config_obj *config_proto.Config) {
	self.mu.Lock()
	defer self.mu.Unlock()

	self.parent = parent
	self.parent_config_obj = parent_config_obj
}

func (self *Repository) Copy() services.Repository {
	self.mu.Lock()
	defer self.mu.Unlock()

	result := &Repository{
		Data:              make(map[string]*artifacts_proto.Artifact),
		parent:            self.parent,
		parent_config_obj: self.parent_config_obj,
	}
	for k, v := range self.Data {
		result.Data[k] = v
	}
	return result
}

var query_regexp = regexp.MustCompile(`(?im)^[\s-]*(precondition|query):\s*$`)
var queries_regexp = regexp.MustCompile(`(?im)(^ +- +)(SELECT|LET|//)`)

// Fix common YAML errors.
func sanitize_artifact_yaml(data string) string {
	// First convert to standard line ending.
	data = strings.Replace(data, "\r\n", "\n", -1)

	// YAML has two types of block level scalars. The default one
	// (which is more intuitive to use) does not preserve white
	// space. This leads to terrible rendering in the GUI and
	// elsewhere because the query appears all on the one
	// line. The user should use the literal scalar (i.e. '- |'
	// form) but this is a difficult yaml rule to remember and
	// makes the artifact look terrible.

	// Therefore we just transform one form into the other in
	// order to trick the yaml decoder to do the right thing.

	result := query_regexp.ReplaceAllStringFunc(data, func(m string) string {
		return m + " |\n"
	})

	result = queries_regexp.ReplaceAllStringFunc(result, func(m string) string {
		parts := queries_regexp.FindStringSubmatch(m)
		return parts[1] + "|\n" + strings.Repeat(" ", len(parts[1])) + parts[2]
	})

	return result
}

func (self *Repository) LoadYaml(
	data string,
	options services.ArtifactOptions) (*artifacts_proto.Artifact, error) {
	artifact := &artifacts_proto.Artifact{}
	err := yaml.UnmarshalStrict([]byte(sanitize_artifact_yaml(data)), artifact)
	if err != nil {
		return nil, errors.Wrap(err, 0)
	}

	artifact.Raw = data

	return self.LoadProto(artifact, options)
}

func (self *Repository) LoadProto(
	artifact *artifacts_proto.Artifact,
	options services.ArtifactOptions) (*artifacts_proto.Artifact, error) {

	if artifact == nil {
		return nil, errors.New("Invalid artifact")
	}

	// Make a copy of the artifact to store in the repository.
	artifact = proto.Clone(artifact).(*artifacts_proto.Artifact)

	// Clear fields that are used internally and should not be set by
	// the yaml.
	artifact.Compiled = false
	artifact.IsAlias = false
	artifact.IsInherited = false
	artifact.Metadata = nil
	artifact.BuiltIn = options.ArtifactIsBuiltIn
	artifact.CompiledIn = options.ArtifactIsCompiledIn

	if len(options.Tags) > 0 {
		artifact.Metadata = &artifacts_proto.ArtifactMetadata{
			Tags: options.Tags,
		}
		if self.metadata != nil {
			self.metadata.Set(artifact.Name, artifact.Metadata)
		}
	}

	err := validateArtifactName(artifact.Name)
	if err != nil {
		return nil, err
	}

	// Validate the artifact.
	for _, report := range artifact.Reports {
		report.Type = strings.ToLower(report.Type)
		switch report.Type {
		case "monitoring_daily", "server_event", "client",
			"internal", "hunt", "templates":
		case "":
			report.Type = "CLIENT"

		case "html": // HTML reports form a main HTML page for report exports.
		default:
			return nil, fmt.Errorf("Invalid report type %s", report.Type)
		}
	}

	// Make sure none of the aliases already exist
	for _, alias := range artifact.Aliases {
		_, pres := self.Data[alias]
		if pres && !options.AllowOverridingAlias {
			return nil, fmt.Errorf("%s: Artifact Alias is already taken %s",
				artifact.Name, alias)
		}
	}

	// Normalize the type.
	artifact.Type = strings.ToLower(artifact.Type)
	switch artifact.Type {
	case "":
		// By default use the client type.
		artifact.Type = "client"

	case "client", "client_event", "server",
		"server_event", "notebook", "internal":
		// These types are acceptable.

	default:
		return nil, errors.New("Artifact type invalid.")
	}

	// Normalize the artifact by converting the deprecated Queries
	// field to the Query field.
	for _, source := range artifact.Sources {
		if source.Query == "" {
			for _, query := range source.Queries {
				source.Query += "\n" + query
			}
		}

		// Remove the Queries field - from now it will contain
		// optimized queries.
		source.Queries = nil
	}

	// Validate the artifact contains syntactically correct
	// VQL. We do not need to validate embedded artifacts since we
	// assume they are ok if they passed CI.
	if options.ValidateArtifact {
		// Check RequiredPermissions
		for _, perm := range artifact.RequiredPermissions {
			if acls.GetPermission(perm) == acls.NO_PERMISSIONS {
				return nil, fmt.Errorf("Invalid artifact permission: %v", perm)
			}
		}

		// Ensure precodition has correct syntax - it should be a VQL
		// query.
		if artifact.Precondition != "" {
			_, err := vfilter.MultiParse(artifact.Precondition)
			if err != nil {
				return nil, fmt.Errorf(
					"While parsing artifact precondition: %w",
					reportError(err, artifact, "precondition", 0))
			}
		}

		// Ensure export has correct syntax
		if artifact.Export != "" {
			_, err := vfilter.MultiParse(artifact.Export)
			if err != nil {
				return nil, fmt.Errorf(
					"While parsing artifact export: %w",
					reportError(err, artifact, "export", 0))
			}
		}

		// Check each source for validity
		for idx, source := range artifact.Sources {
			if source.Precondition != "" {
				if artifact.Precondition != "" {
					return nil, fmt.Errorf(
						"Artifact %s contains a top level precondition "+
							"and a source precondition.", artifact.Name)
				}

				_, err := vfilter.MultiParse(source.Precondition)
				if err != nil {
					return nil, fmt.Errorf("While parsing precondition: %w",
						reportError(err, artifact,
							"sources.[].precondition", idx))
				}
			}

			if len(source.Query) > 0 {

				// Check we can parse it properly.
				queries, err := vfilter.MultiParse(source.Query)
				if err != nil {
					return nil, fmt.Errorf("While parsing source query %v: %w",
						source.Name,
						reportError(err, artifact,
							"sources.[].query", idx))
				}

				// Make sure the source format is correct
				for idx2, vql := range queries {
					if idx2 < len(queries)-1 {
						if vql.Let == "" {
							return nil, fmt.Errorf(
								"Invalid artifact %s: All Queries in a source "+
									"must be LET queries, except for the "+
									"final one.", artifact.Name)
						}
					} else {
						if vql.Let != "" {
							return nil, fmt.Errorf(
								"Invalid artifact  %s: All Queries in a source "+
									"must be LET queries, except for the "+
									"final one.", artifact.Name)
						}
					}
				}
			}

			// If the source defines any notebook cells check they are
			// valid.
			for _, cell := range source.Notebook {
				cell.Type = strings.ToLower(cell.Type)
				switch cell.Type {
				case "md", "markdown", "vql", "vql_suggestion", "notebook", "none":
				default:
					return nil, fmt.Errorf(
						"Artifact %s contains an invalid notebook cell type: %v",
						artifact.Name, cell.Type)
				}
			}
		}
	}

	// Prevent built in artifacts from being overridden.
	if !options.ArtifactIsBuiltIn {
		self.mu.Lock()
		existing_artifact, pres := self.Data[artifact.Name]
		self.mu.Unlock()
		if pres && existing_artifact.BuiltIn {
			return nil, fmt.Errorf("Unable to override built in artifact %v",
				artifact.Name)
		}
	}

	self.mu.Lock()
	self.Data[artifact.Name] = artifact
	for _, alias := range artifact.Aliases {
		// Make a copy of the artifact definition
		artifact_copy := proto.Clone(artifact).(*artifacts_proto.Artifact)
		artifact_copy.Name = alias
		artifact_copy.IsAlias = true
		self.Data[alias] = artifact_copy
	}
	self.mu.Unlock()

	return artifact, nil
}

func (self *Repository) GetArtifactType(
	ctx context.Context, config_obj *config_proto.Config,
	artifact_name string) (string, error) {
	artifact, pres := self.Get(ctx, config_obj, artifact_name)
	if !pres {
		return "", fmt.Errorf("Artifact %s not known", artifact_name)
	}

	return artifact.Type, nil
}

func (self *Repository) GetSource(
	ctx context.Context, config_obj *config_proto.Config,
	name string) (*artifacts_proto.ArtifactSource, bool) {
	artifact_name, source_name := paths.SplitFullSourceName(name)
	artifact, pres := self.Get(ctx, config_obj, artifact_name)
	if !pres {
		return nil, false
	}
	for _, source := range artifact.Sources {
		if source.Name == source_name {
			return source, true
		}
	}

	return nil, false
}

func (self *Repository) DecorateMetadata(
	artifact *artifacts_proto.Artifact) *artifacts_proto.Artifact {

	if self.metadata == nil {
		return artifact
	}

	// Query the metadataManager for metadata about this artifact.
	metadata, pres := self.metadata.Get(artifact.Name)
	if pres {
		artifact.Metadata = metadata
	}
	return artifact
}

func (self *Repository) Get(
	ctx context.Context, config_obj *config_proto.Config,
	name string) (*artifacts_proto.Artifact, bool) {
	self.mu.Lock()
	cached_artifact, pres := self.get(name)
	if !pres {
		self.mu.Unlock()

		// If we have a parent repository just get it from there.
		if self.parent != nil {
			artifact, found := self.parent.Get(ctx, self.parent_config_obj, name)
			if !found || artifact == nil {
				return nil, false
			}

			// Tag this artifact as inherited.
			artifact = self.DecorateMetadata(artifact)
			artifact.IsInherited = true

			return artifact, true
		}
		return nil, false
	}

	// Return a copy to keep the repository pristine.
	result := proto.Clone(cached_artifact).(*artifacts_proto.Artifact)
	self.mu.Unlock()

	if result.Compiled {
		return self.DecorateMetadata(result), true
	}

	// Delay processing until we need it. This means loading
	// artifacts is faster.
	err := compileArtifact(ctx, config_obj, result)
	if err != nil {
		logger := logging.GetLogger(config_obj, &logging.GenericComponent)
		logger.Error("While compiling artifact %v: %v", name, err)
		return nil, false
	}

	// Store the compiled version in the repository for next time.
	self.mu.Lock()
	_, pres = self.Data[name]
	if pres {
		self.Data[name] = proto.Clone(result).(*artifacts_proto.Artifact)
	}
	self.mu.Unlock()

	return self.DecorateMetadata(result), true
}

func (self *Repository) get(name string) (*artifacts_proto.Artifact, bool) {
	artifact_name, source_name := paths.SplitFullSourceName(name)

	res, pres := self.Data[artifact_name]
	if !pres {
		return nil, false
	}

	// Caller did not specify a source - just give them a copy of
	// the complete artifact.
	if source_name == "" {
		return res, pres
	}

	// Caller asked for only a specific source in the artifact -
	// we therefore hand them back a copy with other sources
	// removed.
	new_artifact := proto.Clone(res).(*artifacts_proto.Artifact)
	new_artifact.Sources = nil

	for _, source := range res.Sources {
		if source.Name == source_name {
			new_artifact.Sources = append(new_artifact.Sources, source)
			return new_artifact, true
		}
	}

	// If we get here the source is not found in the artifact.
	return nil, false
}

func (self *Repository) Del(name string) {
	self.mu.Lock()
	defer self.mu.Unlock()

	delete(self.Data, name)

	if self.metadata != nil {
		self.metadata.Clear(name)
	}
}

func (self *Repository) Tags(ctx context.Context,
	config_obj *config_proto.Config) ([]string, error) {
	if self.metadata != nil {
		return self.metadata.Tags(), nil
	}
	return nil, utils.NotFoundError
}

func (self *Repository) List(ctx context.Context,
	config_obj *config_proto.Config) ([]string, error) {
	self.mu.Lock()
	defer self.mu.Unlock()

	results := self.list()
	if self.parent != nil {
		seen := make(map[string]bool)
		for _, name := range results {
			seen[name] = true
		}

		parent_list, err := self.parent.List(ctx, self.parent_config_obj)
		if err == nil {
			for _, name := range parent_list {
				_, pres := seen[name]
				if !pres {
					results = append(results, name)
				}
			}
		}
	}

	sort.Strings(results)

	return results, nil
}

func (self *Repository) list() []string {
	result := make([]string, 0, len(self.Data))
	for k := range self.Data {
		result = append(result, k)
	}
	return result
}

func NewArtifactRepositoryPlugin(
	self services.Repository, config_obj *config_proto.Config) vfilter.PluginGeneratorInterface {
	return &ArtifactRepositoryPlugin{
		repository:      self,
		config_obj:      config_obj,
		mocks:           make(map[string][]vfilter.Row),
		mock_call_count: make(map[string]int),
	}
}

func splitQueryToQueries(query string) ([]string, error) {
	vqls, err := vfilter.MultiParse(query)
	if err != nil {
		return nil, fmt.Errorf("While parsing query: %w", err)
	}

	scope := vql_subsystem.MakeScope()
	result := []string{}
	for _, vql := range vqls {
		result = append(result, vfilter.FormatToString(scope, vql))
	}

	return result, nil
}

func compileArtifact(
	ctx context.Context, config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact) error {

	if artifact.Compiled {
		return nil
	}

	for _, source := range artifact.Sources {
		if source.Queries == nil && source.Query != "" {
			// The Queries field contains the compiled queries -
			// removing any comments.
			queries, err := splitQueryToQueries(source.Query)
			if err != nil {
				return fmt.Errorf("While compiling %v/%v: %w",
					artifact.Name, source.Name, err)
			}
			source.Queries = queries
		}
	}

	// Also compile the export section to alert immediately about any
	// problems.
	if artifact.Export != "" {
		_, err := splitQueryToQueries(artifact.Export)
		if err != nil {
			return fmt.Errorf("While compiling Exports from %v: %w",
				artifact.Name, err)
		}
	}
	artifact.Compiled = true

	return updateTools(ctx, config_obj, artifact)
}

func updateTools(
	ctx context.Context, config_obj *config_proto.Config,
	artifact *artifacts_proto.Artifact) error {
	// Make sure tools are all defined.
	inventory, err := services.GetInventory(config_obj)
	if err != nil {
		return nil
	}

	for _, tool := range artifact.Tools {
		tool_request := proto.Clone(tool).(*artifacts_proto.Tool)
		tool_request.Artifact = artifact.Name
		err := inventory.AddTool(ctx, config_obj, tool_request,
			services.ToolOptions{
				Upgrade:            true,
				ArtifactDefinition: true,
			})
		if err != nil {
			return err
		}
	}
	return nil
}

var (
	artifactNameRegex      = regexp.MustCompile("^[a-zA-Z0-9_.]+$")
	artifactComponentRegex = regexp.MustCompile("^[0-9]")
)

func validateArtifactName(name string) error {
	if !artifactNameRegex.MatchString(name) {
		return errors.New(
			"Invalid artifact name. Can only contain characted in this set 'a-zA-Z0-9_.'")
	}

	for _, part := range strings.Split(name, ".") {
		if artifactComponentRegex.MatchString(part) {
			return errors.New(
				"Invalid artifact name. Name parts can not start with a number'")
		}
	}

	return nil
}
