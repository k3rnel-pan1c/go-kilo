package main

import (
	"fmt"
	"os"
	"unicode"

	"golang.org/x/term"
)

/* terminal */

func enableRawMode() (*term.State, error) {
	return term.MakeRaw(int(os.Stdin.Fd()))
}

func disableRawMode(oldState *term.State) {
	err := term.Restore(int(os.Stdin.Fd()), oldState)
	if err != nil {
		panic(err)
	}
}

/* init */

func main() {
	oldState, err := enableRawMode()
	if err != nil {
		panic(err)
	}
	defer disableRawMode(oldState)

	buf := make([]byte, 1)

	for {
		_, err := os.Stdin.Read(buf)
		if err != nil {
			panic(err)
		}

		c := buf[0]

		if unicode.IsControl(rune(c)) {
			fmt.Printf("%d\r\n", c)
		} else {
			fmt.Printf("%d ('%c')\r\n", c, c)
		}

		if c == 'q' {
			break
		}
	}
}
