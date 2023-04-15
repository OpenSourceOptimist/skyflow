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
	conn, closer := help.NewSocket(ctx, t, help.WithURI("wss://relay.nostry.eu"))
	defer closer()
	before := time.Now()
	for i := 0; i < 100; i++ {
		help.Publish(ctx, t,
			help.ToEvent(help.NewSignedEvent(t, help.EventOptions{Content: fmt.Sprintf("%d", i)})),
			conn)
	}
	fmt.Println(fmt.Sprintf("average publish: %f ms", float64(time.Since(before).Milliseconds())/float64(count)))
}

type PanicTestingT struct{}

func (t *PanicTestingT) Errorf(format string, args ...interface{}) {
	panic(fmt.Sprintf(format, args...))
}
func (t *PanicTestingT) FailNow() {
	panic("FailNow")
}
