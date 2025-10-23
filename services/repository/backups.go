package repository

import (
	"context"
	"fmt"
	"sync"

	artifacts_proto "www.velocidex.com/golang/velociraptor/artifacts/proto"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/constants"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/utils"
	"www.velocidex.com/golang/vfilter"
)

type BackupRecord struct {
	Artifact string                            `json:"artifact"`
	Filename string                            `json:"filename"`
	Metadata *artifacts_proto.ArtifactMetadata `json:"metadata"`
}

type RepositoryBackupProvider struct {
	config_obj *config_proto.Config
}

func (self RepositoryBackupProvider) ProviderName() string {
	return "RepositoryBackupProvider"
}

func (self RepositoryBackupProvider) Name() []string {
	return []string{"custom_artifacts.json"}
}

// The backup will just dump out the contents of the hunt dispatcher.
func (self RepositoryBackupProvider) BackupResults(
	ctx context.Context, wg *sync.WaitGroup,
	container services.BackupContainerWriter) (<-chan vfilter.Row, error) {

	output_chan := make(chan vfilter.Row)

	manager, err := services.GetRepositoryManager(self.config_obj)
	if err != nil {
		close(output_chan)
		return output_chan, err
	}

	repository, err := manager.GetGlobalRepository(self.config_obj)
	if err != nil {
		close(output_chan)
		return output_chan, err
	}

	all_artifacts, err := repository.List(ctx, self.config_obj)
	if err != nil {
		close(output_chan)
		return output_chan, err
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		defer close(output_chan)

		for _, name := range all_artifacts {
			artifact, pres := repository.Get(ctx, self.config_obj, name)
			if !pres {
				continue
			}

			// Skip all these ones.
			if artifact.IsInherited ||
				artifact.BuiltIn ||
				artifact.CompiledIn ||
				artifact.IsAlias {
				continue
			}

			name := fmt.Sprintf("artifacts/%v.yaml", artifact.Name)
			writer, err := container.Create(name, utils.GetTime().Now())
			if err != nil {
				continue
			}

			_, err = writer.Write([]byte(artifact.Raw))
			if err != nil {
				writer.Close()
				continue
			}
			writer.Close()

			record := &BackupRecord{
				Artifact: artifact.Name,
				Filename: name,
			}

			if artifact.Metadata != nil {
				record.Metadata = artifact.Metadata
			}

			output_chan <- record
		}
	}()

	return output_chan, nil
}

func (self RepositoryBackupProvider) Restore(ctx context.Context,
	container services.BackupContainerReader,
	in <-chan vfilter.Row) (stat services.BackupStat, err error) {

	count := 0
	defer func() {
		if stat.Error == nil {
			stat.Error = err
		}
		stat.Message += fmt.Sprintf("Restored %v custom artifacts\n", count)
	}()

	manager, err := services.GetRepositoryManager(self.config_obj)
	if err != nil {
		return stat, err
	}

	repository, err := manager.GetGlobalRepository(self.config_obj)
	if err != nil {
		return stat, err
	}

	options := services.ArtifactOptions{
		ValidateArtifact: true,
	}

	for {
		select {
		case <-ctx.Done():
			return stat, nil

		case row, ok := <-in:
			if !ok {
				return stat, nil
			}

			serialized, err := json.MarshalWithOptions(
				row, json.DefaultEncOpts())
			if err != nil {
				continue
			}

			record := &BackupRecord{}
			err = json.Unmarshal(serialized, record)
			if err != nil {
				continue
			}

			fd, err := container.Open(record.Filename)
			if err != nil {
				continue
			}

			data, err := utils.ReadAllWithLimit(fd, constants.MAX_MEMORY)
			if err != nil {
				continue
			}

			_, err = repository.LoadYaml(string(data), options)
			if err != nil {
				stat.Message += fmt.Sprintf("Unable to load %v from %v: %v\n",
					record.Artifact,
					record.Filename, err)
				continue
			}
			count++
		}
	}
}
