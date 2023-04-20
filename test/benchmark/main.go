package main

import (
	"context"
	"fmt"
	"time"

	"github.com/OpenSourceOptimist/skyflow/test/help"
)

func main() {
	t := &PanicTestingT{}
	ctx := context.Background()
	conn, _, closer := help.NewSocket(ctx, t, help.WithURI("wss://relay.nostry.eu"))
	defer closer()
	count := 200
	before := time.Now()
	for i := 0; i < count; i++ {
		help.Publish(ctx, t,
			help.Event(t, help.EventOptions{Content: fmt.Sprintf("%d", i)}),
			conn)
	}
	fmt.Printf("average publish: %f ms\n", float64(time.Since(before).Milliseconds())/float64(count))
}

type PanicTestingT struct{}

func (t *PanicTestingT) Errorf(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}
func (t *PanicTestingT) FailNow() {
	panic("FailNow")
}
