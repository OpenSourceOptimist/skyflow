package component

import (
	//"github.com/nbd-wtf/go-nostr"
	"fmt"
	"os"
	"testing"

	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
)

func TestMain(m *testing.M) {
	fmt.Println("starting component test")
	pool, err := dockertest.NewPool("")
	if err != nil {
		panic("dockertest.NewPool: " + err.Error())
	}

	fmt.Println("building container")
	resource, err := pool.BuildAndRunWithOptions("./Containerfile", &dockertest.RunOptions{
		Name: "skyflow",
		PortBindings: map[docker.Port][]docker.PortBinding{
			"80/tcp": {{HostPort: "80"}},
		},
	})
	if err != nil {
		panic("pool.BuildAndRun: " + err.Error())
	}
	fmt.Println("running tests")
	code := m.Run()
	resource.Close()
	os.Exit(code)
}
