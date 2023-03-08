package slice

import (
	"fmt"
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

func ReadSlice[T any](c chan T, lenght int, timeout time.Duration) ([]T, error) {
	res := make([]T, 0, lenght)
	timedout := time.After(timeout)
	for i := 0; i < lenght; i++ {
		select {
		case t := <-c:
			res = append(res, t)
		case <-timedout:
			return nil, fmt.Errorf("timed out waiting for result %d out of %d results expected from channel", i+1, lenght)
		}
	}
	return res, nil
}
