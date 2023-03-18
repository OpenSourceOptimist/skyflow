package slice

import (
	"context"
	"time"
)

func Map[T any, K any](slice []T, f func(T) K) []K {
	res := make([]K, 0, len(slice))
	for _, t := range slice {
		res = append(res, f(t))
	}
	return res
}

func Contains[T comparable](slice []T, element T) bool {
	for _, t := range slice {
		if element == t {
			return true
		}
	}
	return false
}

func ReadSlice[T any](c <-chan T, timeout time.Duration) []T {
	res := make([]T, 0)
	for {
		select {
		case t := <-c:
			res = append(res, t)
		case <-time.After(timeout):
			return res
		}
	}
}

func FindAll[T any, K any](slice []T, f func(T) (K, bool)) []K {
	res := make([]K, 0, len(slice))
	for _, t := range slice {
		k, ok := f(t)
		if ok {
			res = append(res, k)
		}
	}
	return res
}

func ChanConcatenate[T any](head <-chan T, tail <-chan T) <-chan T {
	res := make(chan T)
	go func() {
		for t := range head {
			res <- t
		}
		for t := range tail {
			res <- t
		}
	}()
	return res
}
func AsyncWrite[T any](ctx context.Context, c chan<- T, t T) {
	go func() {
		select {
		case c <- t:
		case <-ctx.Done():
		}
	}()
}

func Chunk[T any](slice []T, size int) [][]T {
	res := make([][]T, 0)
	chunk := make([]T, 0)
	for _, t := range slice {
		chunk = append(chunk, t)
		if len(chunk) >= size {
			res = append(res, chunk)
			chunk = nil
		}
	}
	if len(chunk) > 0 {
		res = append(res, chunk)
	}
	return res
}
