package journal_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"www.velocidex.com/golang/velociraptor/config"
	config_proto "www.velocidex.com/golang/velociraptor/config/proto"
	"www.velocidex.com/golang/velociraptor/services"
	"www.velocidex.com/golang/velociraptor/services/journal"
	"www.velocidex.com/golang/velociraptor/services/orgs"
	"www.velocidex.com/golang/velociraptor/vtesting/assert"
)

func BenchmarkArtifactGet(b *testing.B) {
	config_obj, err := new(config.Loader).
		WithFileLoader("../../http_comms/test_data/server.config.yaml").
		LoadAndValidate()
	if err != nil {
		b.Fatalf("LoadConfig: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*60)
	wg := &sync.WaitGroup{}
	defer wg.Wait()
	defer cancel()

	config_obj.Services = &config_proto.ServerServicesConfig{
		JournalService:    true,
		RepositoryManager: true,
	}

	err = orgs.StartTestOrgManager(ctx, wg, config_obj, nil)
	if err != nil {
		b.Fatalf("StartTestOrgManager: %v", err)
	}

	manager, err := services.GetRepositoryManager(config_obj)
	if err != nil {
		b.Fatalf("Get manager: %v", err)
	}

	repository, err := manager.GetGlobalRepository(config_obj)
	if err != nil {
		b.Fatalf("Get repository: %v", err)
	}

	// Set a built in artifact
	_, err = repository.LoadYaml(`
name: Custom.ClientArtifact
type: CLIENT
`,
		services.ArtifactOptions{
			ArtifactIsBuiltIn: true,
			ValidateArtifact:  true,
		})
	if err != nil {
		b.Fatalf("LoadYaml: %v", err)
	}

	artifact_name := "Custom.ClientArtifact"
	t, err := repository.GetArtifactType(ctx, config_obj, artifact_name)
	if err == nil {
		assert.Equal(b, t, "client")
	}

	client_id := "C.123"
	for b.Loop() {
		journal.GetArtifactMode(ctx, config_obj, services.JournalOptions{
			ArtifactName: artifact_name,
			ClientId:     client_id})
	}
}
