package main

import (
	"math"
	"testing"
)

// run TestAdd*
// go test -timeout 30s -v  -run ^TestAdd

// go test -timeout 30s -run ^TestAddInt$ .
func TestAddInt(t *testing.T) {
	IntCase := [][3]int{
		{1, 2, 3},
		{5, 8, 13},
		{0, 0, 0},
		{1, 1, 2},
		{2, 2, 4},
	}
	for _, v := range IntCase {
		if result := Add(v[0], v[1]); result != v[2] {
			t.Errorf("%v + %v expect: %v, got: %v", v[0], v[1], v[2], result)
		}
	}
}

// go test -timeout 30s -run ^TestAddFloat$ .
func TestAddFloat(t *testing.T) {
	IntCase := [][3]float64{
		{1.1, 2.2, 3.3},
		{5.1, 8.2, 13.3},
		{0, 0, 0},
		{1.2, 1.3, 2.5},
		{2.3, 2.4, 4.7},
	}

	for _, v := range IntCase {
		if result := Add(v[0], v[1]); math.Dim(math.Max(result, v[2]), math.Min(result, v[2])) > 0.001 {
			t.Errorf("%v + %v expect: %v, got: %v", v[0], v[1], v[2], result)
		}
	}
}

// go test -test.benchmem -test.bench .

// go test -benchmem -run=^$ -bench ^BenchmarkAddInt$ .
func BenchmarkAddInt(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Add(5, 7)
	}
}

// go test -benchmem -run=^$ -bench ^BenchmarkAddFloat$ .
func BenchmarkAddFloat(b *testing.B) {
	for i := 0; i < b.N; i++ {
		Add(5.5, 7.7)
	}
}
