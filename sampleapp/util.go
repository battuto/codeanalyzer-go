package main

import "fmt"

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

// Calculator definisce operazioni aritmetiche di base.
type Calculator interface {
	// Add somma due numeri.
	Add(a, b int) int
	// Multiply moltiplica due numeri.
	Multiply(a, b int) int
}

// Transform applica una trasformazione a una stringa e la stampa
// solo se il risultato è abbastanza lungo. Questo crea sia
// data dependencies (input → result) che control dependencies (if → print).
func Transform(input string) string {
	result := "[" + input + "]"
	if len(result) > 5 {
		fmt.Println("Transformed:", result)
	}
	return result
}
