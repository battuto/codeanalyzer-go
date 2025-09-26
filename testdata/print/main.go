package main

import "fmt"

func main() {
	fmt.Println("hello")
	hello()
}

func hello() {
	fmt.Println("from helper")
}

// dead is never called
func dead() {
	fmt.Println("dead")
}
