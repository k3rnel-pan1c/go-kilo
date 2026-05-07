package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

/* defines/macros/functions */

var kilo_version = "0.0.1"
var kilo_tab_stop = 8

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

type erow struct {
	rsize  int    //number of chars in render
	chars  []byte //raw line content
	render []byte //line content with tabs expanded to spaces
}

type editorConfig struct {
	cx, cy     int         //cursor x and y pos relative to the rows and cols
	rx         int         //index to the render field (cx + amount of tabs * tab spaces)
	rowoff     int         //row the user scrolled to (vertical scroll offset)
	coloff     int         //col the user scrolled to (horizontal scroll offset)
	screenrows int         //number of rows the terminal can display
	screencols int         //number of cols the terminal can display
	numrows    int         //number of rows in the file
	row        []erow      //file rows
	termios    *term.State //original terminal state, restored on exit
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

/* row operations */

func editorRowCxToRx(row *erow, cx int) int {
	rx := 0
	for y := range cx {
		if row.chars[y] == '\t' {
			rx += (kilo_tab_stop - 1) - (rx % kilo_tab_stop)
		}
		rx++
	}
	return rx
}

func editorUpdateRow(row *erow) {
	idx := 0
	for _, c := range row.chars {
		if c == '\t' {
			row.render = append(row.render, ' ')
			idx++
			for idx%kilo_tab_stop != 0 {
				row.render = append(row.render, ' ')
				idx++
			}
		} else {
			row.render = append(row.render, c)
			idx++
		}
	}

	row.rsize = len(row.render)
}

func editorAppendRow(s []byte) {
	at := E.numrows

	E.row = append(E.row, erow{chars: s})

	// not needed in go but added for understanding
	E.row[at].rsize = 0
	E.row[at].render = nil

	editorUpdateRow(&E.row[at])

	E.numrows++
}

/* file i/o */

func editorOpen(filename string) {
	f, err := os.Open(filename)
	if err != nil {
		die(err)
	}

	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := scanner.Bytes()
		line = bytes.TrimRight(line, "\n")
		line = bytes.TrimRight(line, "\r")
		line = bytes.TrimRight(line, "\r\n")

		editorAppendRow(line)
	}
	f.Close()

}

/* output */

func editorScroll() {
	E.rx = E.cx

	//vertical
	if E.cy < E.rowoff {
		E.rowoff = E.cy
	}
	if E.cy >= E.rowoff+E.screenrows {
		E.rowoff = E.cy - E.screenrows + 1
	}

	// horizontal
	if E.rx < E.coloff {
		E.coloff = E.rx
	}
	if E.rx >= E.coloff+E.screencols {
		E.coloff = E.rx - E.screencols + 1
	}

}

func editorDrawRows(ab []byte) []byte {
	for y := range E.screenrows {
		filerow := y + E.rowoff
		if filerow >= E.numrows {
			//open without file
			if E.numrows == 0 && y == E.screenrows/3 {
				welcome := fmt.Sprintf("Kilo editor -- version %s", kilo_version)
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
		} else {
			len := E.row[filerow].rsize - E.coloff
			if len < 0 {
				len = 0
			}
			if len > E.screencols {
				len = E.screencols
			}
			ab = append(ab, E.row[filerow].render[E.coloff:E.coloff+len]...)
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
	editorScroll()

	ab := make([]byte, 0)
	// hide cursor
	ab = append(ab, []byte("\x1b[?25l")...)
	// disable text wrapping
	ab = append(ab, []byte("\x1b[?7l")...)
	// move cursor to top left
	ab = append(ab, []byte("\x1b[H")...)

	ab = editorDrawRows(ab)

	// move cursor to coordinates
	buf := fmt.Sprintf("\x1b[%d;%dH", (E.cy-E.rowoff)+1, (E.rx-E.coloff)+1)
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
		} else if E.cy > 0 {
			E.cy--
			E.cx = len(E.row[E.cy].chars)
		}
	case ARROW_RIGHT:
		if E.cy < E.numrows && E.cx < len(E.row[E.cy].chars) {
			E.cx++
		} else if E.cy < E.numrows && E.cx == len(E.row[E.cy].chars) {
			E.cy++
			E.cx = 0
		}

	case ARROW_UP:
		if E.cy != 0 {
			E.cy--
		}
	case ARROW_DOWN:
		if E.cy < E.numrows {
			E.cy++
		}
	}

	rowlen := len(E.row[E.cy].chars)
	if E.cx > rowlen {
		E.cx = rowlen
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
	E.rx = 0
	E.rowoff = 0
	E.coloff = 0
	E.numrows = 0
	E.row = nil

	E.screenrows, E.screencols = getWindowSize()

}

func main() {
	enableRawMode()
	defer disableRawMode()
	initEditor()
	if len(os.Args) >= 2 {
		editorOpen(os.Args[1])
	}

	for {
		editorRefreshScreen()
		if !editorProcessKeypress() {
			break
		}
	}
}
