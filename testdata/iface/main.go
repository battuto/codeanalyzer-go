package main

import "fmt"

type Greeter interface{ Greet() }

type A struct{}

func (A) Greet() { fmt.Println("A") }

type B struct{}

func (B) Greet() { fmt.Println("B") }

func main() {
	var g Greeter
	g = A{} // solo A fluisce nell'interfaccia
	g.Greet()
}
