package main

import (
	"fmt"
	"time"
)

// Greeter is a tiny example interface.
type Greeter interface {
	Greet(name string) string
}

// ConsoleGreeter prints greetings with a prefix.
type ConsoleGreeter struct {
	Prefix string
}

// Greet implements Greeter.
func (c ConsoleGreeter) Greet(name string) string {
	if name == "" {
		name = "world"
	}
	return fmt.Sprintf("%sHello, %s!", c.Prefix, name)
}

func main() {
	g := ConsoleGreeter{Prefix: "[app] "}
	msg := g.Greet("Katia")
	fmt.Println(msg)

	// fire a tiny goroutine just to have something asynchronous
	go func() {
		fmt.Println(g.Greet(""))
	}()
	// wait a moment to let the goroutine print
	time.Sleep(10 * time.Millisecond)

	fmt.Println("2+3=", add(2, 3))
	fmt.Println("DoTwice(+1, 5)=", DoTwice(func(x int) int { return x + 1 }, 5))

	p := &Person{Name: "Ada", Age: 41}
	p.Birthday() // receiver method
}
