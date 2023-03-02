package component

import (
	//"github.com/nbd-wtf/go-nostr"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMain(m *testing.M) {
	fmt.Println("starting component test")
	pool, err := dockertest.NewPool("")
	if err != nil {
		fmt.Println("dockertest.NewPool: " + err.Error())
		return
	}

	fmt.Println("starting mongo")

	pool.RemoveContainerByName("test_db")
	mongoResource, err := pool.RunWithOptions(
		&dockertest.RunOptions{
			Name:       "test_db",
			Repository: "mongo",
			Tag:        "5.0.15",
			PortBindings: map[docker.Port][]docker.PortBinding{
				"27017/tcp": {{HostPort: "27017"}},
			},
		},
	)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("verifying mongo connection")
	ctx := context.Background()
	mongoClient, err := mongo.Connect(ctx, options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		fmt.Println(err.Error())
		return
	}
	err = pool.Retry(func() error {
		err := mongoClient.Ping(ctx, nil)
		if err != nil {
			fmt.Println(err)
		}
		return err
	})
	if err != nil {
		fmt.Println(err.Error())
	}

	fmt.Println("building and starting skyflow container")
	pool.RemoveContainerByName("test_skyflow")
	_, err = pool.BuildAndRunWithOptions("./Containerfile", &dockertest.RunOptions{
		Name: "test_skyflow",
		PortBindings: map[docker.Port][]docker.PortBinding{
			"80/tcp": {{HostPort: "80"}},
		},
		Env: []string{
			"MONGODB_URI=mongodb://" + mongoResource.Container.NetworkSettings.IPAddress + ":27017",
		},
	})
	if err != nil {
		fmt.Println("pool.BuildAndRun: " + err.Error())
		return
	}
	fmt.Println("running tests")
	code := m.Run()
	fmt.Println("cleaning up")
	os.Exit(code)
}
