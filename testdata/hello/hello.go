package hello

import "fmt"

type T struct{}

func (t *T) Do() { fmt.Println("hi") }

func Free() {}
