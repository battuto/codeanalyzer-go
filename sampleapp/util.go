package main

// add returns the sum of two ints.
func add(a, b int) int { return a + b }

// DoTwice applies fn two times to x.
func DoTwice(fn func(int) int, x int) int {
	return fn(fn(x))
}

// Person is a simple struct with a receiver method.
type Person struct {
	Name string
	Age  int
}

// Birthday increases the age by 1.
func (p *Person) Birthday() { p.Age++ }
