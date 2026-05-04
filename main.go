package main

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

/* defines/macros/functions */

var version = "0.0.1"

func ctrlKey(k int) int {
	return k & 0x1f
}

const (
	ARROW_LEFT = 1000 + iota
	ARROW_RIGHT
	ARROW_UP
	ARROW_DOWN
	DEL_KEY
	HOME_KEY
	END_KEY
	PAGE_UP
	PAGE_DOWN
)

/* data */

type editorConfig struct {
	cx, cy     int
	screenrows int
	screencols int
	termios    *term.State
}

var E editorConfig

/* terminal */

func die(err error) {
	// clear screen
	os.Stdout.Write([]byte("\x1b[2J"))
	// move cursor to top left
	os.Stdout.Write([]byte("\x1b[H"))

	panic(err)
}

func enableRawMode() {
	var err error
	E.termios, err = term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		die(err)
	}
	if err := setRawModeReadTimeout(int(os.Stdin.Fd())); err != nil {
		die(err)
	}

}

func disableRawMode() {
	err := term.Restore(int(os.Stdin.Fd()), E.termios)
	if err != nil {
		die(err)
	}
}

func editorReadKey() int {
	buf := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(buf)
		if err != nil && err != io.EOF {
			die(err)
		}
		if n == 1 {
			break
		}
	}

	c := buf[0]

	if c == '\x1b' {
		seq0 := make([]byte, 1)
		seq1 := make([]byte, 1)
		seq2 := make([]byte, 1)

		n, _ := os.Stdin.Read(seq0)
		if n != 1 {
			return '\x1b'
		}

		n, _ = os.Stdin.Read(seq1)
		if n != 1 {
			return '\x1b'
		}

		if seq0[0] == '[' {

			if seq1[0] >= '0' && seq1[0] <= '9' {
				n, _ = os.Stdin.Read(seq2)
				if n != 1 {
					return '\x1b'
				}

				if seq2[0] == '~' {
					switch seq1[0] {
					case '1':
						return HOME_KEY
					case '3':
						return DEL_KEY
					case '4':
						return END_KEY
					case '5':
						return PAGE_UP
					case '6':
						return PAGE_DOWN
					case '7':
						return HOME_KEY
					case '8':
						return END_KEY
					}
				}
			} else {
				switch seq1[0] {
				case 'A':
					return ARROW_UP
				case 'B':
					return ARROW_DOWN
				case 'C':
					return ARROW_RIGHT
				case 'D':
					return ARROW_LEFT
				case 'H':
					return HOME_KEY
				case 'F':
					return END_KEY
				}
			}
		} else if seq0[0] == 'O' {
			switch seq1[0] {
			case 'H':
				return HOME_KEY
			case 'F':
				return END_KEY
			}
		}
		return '\x1b'
	} else {
		return int(c)
	}
}

func getWindowSize() (int, int) {
	cols, rows, err := term.GetSize(int(os.Stdin.Fd()))
	if err != nil {
		die(err)
	}
	return rows, cols
}

/* output */

func editorDrawRows(ab []byte) []byte {
	for y := range E.screenrows {
		if y == E.screenrows/3 {
			welcome := fmt.Sprintf("Kilo editor -- version %s", version)
			if len(welcome) > E.screencols {
				welcome = welcome[:E.screencols]
			}
			padding := (E.screencols - len(welcome)) / 2
			ab = append(ab, '~')
			padding--
			for range padding {
				ab = append(ab, ' ')
			}
			ab = append(ab, []byte(welcome)...)
		} else {
			ab = append(ab, '~')
		}

		// clear line
		ab = append(ab, []byte("\x1b[K")...)
		if y < E.screenrows-1 {
			ab = append(ab, []byte("\r\n")...)
		}
	}

	return ab
}

func editorRefreshScreen() {
	ab := make([]byte, 0)
	// hide cursor
	ab = append(ab, []byte("\x1b[?25l")...)
	// move cursor to top left
	ab = append(ab, []byte("\x1b[H")...)

	ab = editorDrawRows(ab)

	// move cursor to coordinates
	buf := fmt.Sprintf("\x1b[%d;%dH", E.cy+1, E.cx+1)
	ab = append(ab, []byte(buf)...)

	// show cursor
	ab = append(ab, []byte("\x1b[?25h")...)

	os.Stdout.Write(ab)
}

/* input */

func editorMoveCursor(key int) {
	switch key {
	case ARROW_LEFT:
		if E.cx != 0 {
			E.cx--
		}
	case ARROW_RIGHT:
		if E.cx != E.screencols-1 {
			E.cx++
		}
	case ARROW_UP:
		if E.cy != 0 {
			E.cy--
		}
	case ARROW_DOWN:
		if E.cy != E.screenrows-1 {
			E.cy++
		}
	}
}

func editorProcessKeypress() bool {
	c := editorReadKey()

	switch c {
	case ctrlKey('q'):
		// clear screen
		os.Stdout.Write([]byte("\x1b[2J"))
		// move cursor to top left
		os.Stdout.Write([]byte("\x1b[H"))
		return false
	case HOME_KEY:
		E.cx = 0
	case END_KEY:
		E.cx = E.screencols - 1
	case PAGE_UP:
		for range E.screenrows {
			editorMoveCursor(ARROW_UP)
		}
	case PAGE_DOWN:
		for range E.screenrows {
			editorMoveCursor(ARROW_DOWN)
		}
	case ARROW_UP:
		editorMoveCursor(c)
	case ARROW_LEFT:
		editorMoveCursor(c)
	case ARROW_DOWN:
		editorMoveCursor(c)
	case ARROW_RIGHT:
		editorMoveCursor(c)
	}
	return true
}

/* init */

func initEditor() {
	E.cx = 0
	E.cy = 0
	E.screenrows, E.screencols = getWindowSize()
}

func main() {
	enableRawMode()
	defer disableRawMode()
	initEditor()

	for {
		editorRefreshScreen()
		if !editorProcessKeypress() {
			break
		}
	}
}
