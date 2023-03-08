package slice

import (
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

func ReadSlice[T any](c chan T, timeout time.Duration) []T {
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
