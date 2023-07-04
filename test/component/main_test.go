package component

import (
	"context"
	"flag"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/OpenSourceOptimist/skyflow/test/help"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestMain(m *testing.M) {
	code := 1
	defer func() { os.Exit(code) }()
	fmt.Println("starting component test")
	rURI := flag.String("relayURI", "", "relay to run test against, if omitted it starts up a relay locally")
	dockerfilePath := flag.String("dockerfilePath", "./Dockerfile", "relay to run test against, if omitted it starts up a relay locally")
	flag.Parse()
	relayURI := *rURI
	if relayURI != "" {
		help.SetDefaultURI(relayURI)
		code = m.Run()
	}
	pool, err := dockertest.NewPool("")
	if err != nil {
		fmt.Println("dockertest.NewPool: " + err.Error())
		return
	}
	fmt.Println("starting mongo")
	_ = pool.RemoveContainerByName("test_db")
	mongoResource, err := pool.RunWithOptions(
		&dockertest.RunOptions{
			Name:       "test_db",
			Repository: "mongo",
			Tag:        "6.0.5",
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
	_ = pool.RemoveContainerByName("test_skyflow")
	_, err = pool.BuildAndRunWithOptions(*dockerfilePath, &dockertest.RunOptions{
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
		fmt.Println("dockerfilePath: " + *dockerfilePath)
		return
	}
	//help.SetDefaultURI("ws://localhost:80")
	pool.MaxWait = 10 * time.Second
	_ = pool.Retry(func() error {
		t := ErrTestingT{}
		_, _, closer := help.NewSocket(ctx, &t)
		closer()
		err = t.err
		return t.err
	})
	if err != nil {
		fmt.Println("failed to open connection to skyflow: " + err.Error())
		return
	}
	fmt.Println("running tests")
	code = m.Run()
}

type ErrTestingT struct {
	err error
}

func (t *ErrTestingT) Errorf(format string, args ...interface{}) {
	t.err = fmt.Errorf(format, args...)
}
func (t *ErrTestingT) FailNow() {
	t.err = fmt.Errorf("FailNow")
}
