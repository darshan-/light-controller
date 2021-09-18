package main

import (
	"fmt"
)

func main() {		
	if err := KbdOpen(); err != nil {
		panic(err)
	}
	defer func() {
		_ = KbdClose()
	}()

	fmt.Println("Press ESC to quit")
	for {
		char, key, err := KbdGetKey()
		if err != nil {
			panic(err)
		}
		fmt.Printf("You pressed: rune %q, key %X\r\n", char, key)
        if key == KeyEsc {
			break
		}
	}	
}
