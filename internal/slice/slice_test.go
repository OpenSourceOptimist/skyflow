package slice

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestChunk(t *testing.T) {
	require.Equal(t,
		Chunk([]string{"a", "b"}, 2),
		[][]string{{"a", "b"}},
	)
	require.Equal(t,
		Chunk([]string{"a", "b", "c"}, 2),
		[][]string{{"a", "b"}, {"c"}},
	)
	require.Equal(t,
		0,
		len(Chunk([]string(nil), 2)),
	)
}

func TestChanConcatenate(t *testing.T) {
	chan1 := make(chan int)
	go func() {
		chan1 <- 1
		chan1 <- 2
		close(chan1)
	}()
	chan2 := make(chan int)
	go func() {
		chan2 <- 3
		close(chan2)
	}()
	chan3 := make(chan int)
	go func() {
		chan3 <- 4
		chan3 <- 5
		close(chan3)
	}()
	res := ChanConcatenate(chan1, chan2, chan3)
	resarray := []int{
		<-res,
		<-res,
		<-res,
		<-res,
		<-res,
	}
	require.Equal(t, []int{1, 2, 3, 4, 5}, resarray)
}
