package unified_logs

import (
	"context"
	"regexp"
	"time"

	ulogs "github.com/Velocidex/go-macos-unifiedlogs"
	"www.velocidex.com/golang/velociraptor/accessors"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/velociraptor/utils/files"
	"www.velocidex.com/golang/vfilter"
)

type VeloSourceFile struct {
	path     *accessors.OSPath
	accessor accessors.FileSystemAccessor
	scope    vfilter.Scope
	ctx      context.Context
}

func (self *VeloSourceFile) Reader() (*ulogs.ReadCursor, error) {
	files.Add(self.path.String())
	reader, err := self.accessor.OpenWithOSPath(self.path)
	if err != nil {
		return nil, err
	}

	res := ulogs.NewReadCursor(reader, ulogs.CursorOptions{
		Closer: func() {
			files.Remove(self.path.String())
		},
		Filename: self.path.String(),
	})
	return res, nil
}

func (self *VeloSourceFile) SourcePath() string {
	return self.path.String()
}

type VeloFileProvider struct {
	_Tracev3Files  []*accessors.OSPath
	_UUIDTextFiles []*accessors.OSPath
	uuid_lookup    map[string]*accessors.OSPath

	_DSCFiles      []*accessors.OSPath
	_TimesyncFiles []*accessors.OSPath

	accessor accessors.FileSystemAccessor
	scope    vfilter.Scope
	ctx      context.Context

	logger *ulogs.Logger
}

func (self *VeloFileProvider) Logger() *ulogs.Logger {
	return self.logger
}

func (self *VeloFileProvider) Tracev3Files() (res []ulogs.SourceFile) {
	for _, f := range self._Tracev3Files {
		res = append(res, &VeloSourceFile{
			path:     f,
			accessor: self.accessor,
			scope:    self.scope,
			ctx:      self.ctx,
		})
	}
	return res
}

func (self *VeloFileProvider) UUIDTextFiles() (res []ulogs.SourceFile) {
	for _, f := range self._UUIDTextFiles {
		res = append(res, &VeloSourceFile{
			path:     f,
			accessor: self.accessor,
			scope:    self.scope,
			ctx:      self.ctx,
		})
	}
	return res
}

func (self *VeloFileProvider) TimesyncFiles() (res []ulogs.SourceFile) {
	for _, f := range self._TimesyncFiles {
		res = append(res, &VeloSourceFile{
			path:     f,
			accessor: self.accessor,
			scope:    self.scope,
			ctx:      self.ctx,
		})
	}
	return res
}

func (self *VeloFileProvider) DSCFiles() (res []ulogs.SourceFile) {
	for _, f := range self._DSCFiles {
		res = append(res, &VeloSourceFile{
			path:     f,
			accessor: self.accessor,
			scope:    self.scope,
			ctx:      self.ctx,
		})
	}
	return res
}

func (self *VeloFileProvider) ReadDSCUUID(uuid string) (*ulogs.SharedCacheStrings, error) {
	normalized, err := ulogs.NormalizeUUID(uuid)
	if err != nil {
		return nil, err
	}

	for _, p := range self._DSCFiles {
		if p.Basename() == normalized {
			fd, err := self.accessor.OpenWithOSPath(p)
			if err != nil {
				return nil, err
			}

			return ulogs.ParseDSC(ulogs.NewReadCursor(fd), self.logger)
		}
	}

	return nil, utils.Wrap(
		utils.NotFoundError, "DSC File %v not found", normalized)
}

func (self *VeloFileProvider) ReadUUIDText(
	uuid string) (*ulogs.UUIDText, error) {

	normalized, err := ulogs.NormalizeUUID(uuid)
	if err != nil {
		return nil, err
	}

	p, pres := self.uuid_lookup[normalized]
	if !pres {
		return nil, utils.NotFoundError
	}

	fd, err := self.accessor.OpenWithOSPath(p)
	if err != nil {
		return nil, err
	}

	buf, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY_LARGE)
	if err != nil {
		return nil, err
	}
	return ulogs.ParseUUIDText(buf, self.logger)
}

func (self *VeloFileProvider) Debug() []string {
	res := []string{}
	res = append(res, "Tracev3Files")
	for _, i := range self._Tracev3Files {
		res = append(res, i.String())
	}
	res = append(res, "TimesyncFiles\n")
	for _, i := range self._TimesyncFiles {
		res = append(res, i.String())
	}
	res = append(res, "DSCFiles\n")
	for _, i := range self._DSCFiles {
		res = append(res, i.String())
	}
	res = append(res, "UUIDText\n")
	for _, i := range self._UUIDTextFiles {
		res = append(res, i.String())
	}
	return res
}

func NewVeloFileProvider(
	ctx context.Context,
	scope vfilter.Scope,
	arg *ParseUnifiedPluginArgs) (*VeloFileProvider, error) {

	accessor, err := accessors.GetAccessor(arg.Accessor, scope)
	if err != nil {
		return nil, err
	}

	// If TimesyncFiles is not specified, try to use the Tracev3Files.
	res := &VeloFileProvider{
		// UUID->Path spec
		uuid_lookup: make(map[string]*accessors.OSPath),

		accessor: accessor,
		scope:    scope,
		ctx:      ctx,

		logger: &ulogs.Logger{
			Logger: scope.GetLogger(),
		},
	}

	// Try to collect all the files from the directory provided
	all_files := collect_all_files(scope, accessor, arg.BaseDir)
	res._Tracev3Files = filterFiles(all_files, tracev3Regex)
	res._TimesyncFiles = filterFiles(all_files, timesyncRegex)
	res._UUIDTextFiles = filterFiles(all_files, uuidtextRegex)
	res._DSCFiles = filterFiles(all_files, dscUUIDRegex)

	// Index the uuid files
	for _, p := range res._UUIDTextFiles {
		// UUID Text files live in a directory which is the first part
		// of the uuid, then the filename is the second part.
		prefix := p.Dirname().Basename()
		rest := p.Basename()

		normalized, err := ulogs.NormalizeUUID(prefix + rest)
		if err != nil {
			continue
		}
		// Build the lookup based on the whole uuid.
		res.uuid_lookup[normalized] = p
	}
	return res, nil
}

var (
	tracev3Regex  = regexp.MustCompile(`.tracev3$`)
	timesyncRegex = regexp.MustCompile(`.timesync$`)
	dscUUIDRegex  = regexp.MustCompile(`dsc[/\\][0-9A-Fa-f]{32}$`)

	// Look like a GUID minus one byte - should still be unique
	// enough.
	uuidtextRegex = regexp.MustCompile(`[/\\][0-9A-F]{30}$`)
)

func deduplicate(paths []*accessors.OSPath) (res []*accessors.OSPath) {
	seen := make(map[string]*accessors.OSPath)

	for _, p := range paths {
		_, pres := seen[p.String()]
		if pres {
			continue
		}
		seen[p.String()] = p
		res = append(res, p)
	}

	return res
}

func collect_all_files(
	scope vfilter.Scope,
	accessor accessors.FileSystemAccessor,
	search_paths *accessors.OSPath) (res []*accessors.OSPath) {

	start := utils.GetTime().Now()
	defer func() {
		scope.Log("DEBUG:parse_unified_logs: collected %v files in %v",
			len(res), utils.GetTime().Now().Sub(start).Round(time.Second))
	}()

	return append(res, _collect_all_files(accessor, search_paths, 0)...)
}

func filterFiles(
	paths []*accessors.OSPath, filter *regexp.Regexp) (res []*accessors.OSPath) {
	for _, p := range paths {
		if filter.MatchString(p.String()) {
			res = append(res, p)
		}
	}
	return res
}

func _collect_all_files(accessor accessors.FileSystemAccessor,
	base *accessors.OSPath, depth int) (res []*accessors.OSPath) {

	if depth > 10 {
		return nil
	}

	children, err := accessor.ReadDirWithOSPath(base)
	if err != nil {
		return nil
	}

	for _, c := range children {
		c_path := c.OSPath()
		if c.IsDir() {
			res = append(res, _collect_all_files(accessor, c_path, depth+1)...)
		}

		res = append(res, c.OSPath())
	}

	return res
}
