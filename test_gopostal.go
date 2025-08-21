package main

import (
	"fmt"
	postal "github.com/openvenues/gopostal/parser"
)

func main() {
	fmt.Println("Testing gopostal integration...")
	
	address := "Flat 3, 123 High Street, Alton, Hampshire, GU34 1AB"
	fmt.Printf("Input: %s\n\n", address)
	
	components := postal.ParseAddress(address)
	
	fmt.Println("Parsed components:")
	for _, component := range components {
		fmt.Printf("  %s: %s\n", component.Label, component.Value)
	}
}