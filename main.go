package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"time"

	"golang.org/x/term"
)

/* defines/macros/functions */

var kilo_version = "0.0.1"
var kilo_tab_stop = 8
var kilo_quit_times = 3

func ctrlKey(k int) int {
	return k & 0x1f
}

const (
	BACKSPACE  = 127
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
	cx, cy          int         //cursor x and y pos relative to the rows and cols
	rx              int         //index to the render field (cx + amount of tabs * tab spaces)
	rowoff          int         //row the user scrolled to (vertical scroll offset)
	coloff          int         //col the user scrolled to (horizontal scroll offset)
	screenrows      int         //number of rows the terminal can display
	screencols      int         //number of cols the terminal can display
	numrows         int         //number of rows in the file
	row             []erow      //file rows
	dirty           int         //keeps track of amount of changes made
	filename        string      //name of the opened file
	statusmsg       string      //statusmessage for search and other input from the user
	statusmsg_time  time.Time   //timestamp of the message
	termios         *term.State //original terminal state, restored on exit
	quit_times      int         // keeping track of ctrl-q presses
	last_match      int         // last search match
	match_direction int         // direction in which to search the next match
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

func editorRowRxToCx(row *erow, rx int) int {
	cur_rx := 0
	cx := 0
	for cx = range len(row.chars) {
		if row.chars[cx] == '\t' {
			cur_rx += (kilo_tab_stop - 1) - (cur_rx % kilo_tab_stop)
		}
		cur_rx++
		if cur_rx > rx {
			return cx
		}
	}
	return cx
}

func editorUpdateRow(row *erow) {
	idx := 0
	row.render = nil
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

func editorInsertRow(at int, s []byte) {
	if at < 0 || at > E.numrows {
		return
	}

	E.row = append(E.row[:at], append([]erow{{chars: s}}, E.row[at:]...)...)

	// not needed in go but added for understanding
	E.row[at].rsize = 0
	E.row[at].render = nil

	editorUpdateRow(&E.row[at])

	E.numrows++
	E.dirty++
}

func editorDelRow(at int) {
	if at < 0 || at >= E.numrows {
		return
	}
	E.row = append(E.row[:at], E.row[at+1:]...)
	E.numrows--
	E.dirty++
}

func editorRowInsertChar(row *erow, at int, c int) {
	if at < 0 || at > len(row.chars) {
		at = len(row.chars)
	}
	row.chars = append(row.chars[:at], append([]byte{byte(c)}, row.chars[at:]...)...)
	editorUpdateRow(row)
	E.dirty++
}

func editorRowAppendString(row *erow, s []byte) {
	row.chars = append(row.chars, s...)
	editorUpdateRow(row)
	E.dirty++
}

func editorRowDelChar(row *erow, at int) {
	if at < 0 || at >= len(row.chars) {
		return
	}
	row.chars = append(row.chars[:at], row.chars[at+1:]...)
	editorUpdateRow(row)
	E.dirty++
}

/* editor operations */

func editorInsertChar(c int) {
	if E.cy == E.numrows {
		editorInsertRow(E.numrows, []byte(""))
	}
	editorRowInsertChar(&E.row[E.cy], E.cx, c)
	E.cx++
}

func editorInsertNewline() {
	if E.cx == 0 {
		editorInsertRow(E.cy, []byte(""))
	} else {
		editorInsertRow(E.cy+1, append([]byte{}, E.row[E.cy].chars[E.cx:]...))
		E.row[E.cy].chars = E.row[E.cy].chars[:E.cx]
		editorUpdateRow(&E.row[E.cy])
	}
	E.cy++
	E.cx = 0
}

func editorDelChar() {
	if E.cy == E.numrows {
		return
	}
	if E.cx == 0 && E.cy == 0 {
		return
	}

	row := &E.row[E.cy]
	if E.cx > 0 {
		editorRowDelChar(row, E.cx-1)
		E.cx--
	} else {
		E.cx = len(E.row[E.cy-1].chars)
		editorRowAppendString(&E.row[E.cy-1], row.chars)
		editorDelRow(E.cy)
		E.cy--
	}
}

/* file i/o */

func editorRowsToString() []byte {
	buf := make([]byte, 0)

	for y := range E.numrows {
		buf = append(buf, E.row[y].chars...)
		buf = append(buf, '\n')

	}
	return buf
}

func editorOpen(filename string) {
	E.filename = filename
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

		editorInsertRow(E.numrows, []byte(line))
	}
	f.Close()
	E.dirty = 0
}

func editorSave() {
	if E.filename == "" {
		E.filename = string(editorPrompt("Save as: %s (ESC to cancel)", nil))
		if E.filename == "" {
			editorSetStatusMessage("Save aborted")
			return
		}
	}

	buf := editorRowsToString()

	err := os.WriteFile(E.filename, buf, 0644)
	if err != nil {
		editorSetStatusMessage("Can't save! I/O error: %s", err.Error())
	}
	editorSetStatusMessage("%d bytes written to disk", len(buf))
	E.dirty = 0
}

/* find */

func editorFindCallBack(query []byte, key int) {

	if key == '\r' || key == '\x1b' {
		E.last_match = -1
		E.match_direction = 1
		return
	} else if key == ARROW_RIGHT || key == ARROW_DOWN {
		E.match_direction = 1
	} else if key == ARROW_LEFT || key == ARROW_UP {
		E.match_direction = -1
	} else {
		E.last_match = -1
		E.match_direction = 1
	}

	if E.last_match == -1 {
		E.match_direction = 1
	}
	current := E.last_match

	for range E.numrows {
		current += E.match_direction
		if current == -1 {
			current = E.numrows - 1
		} else if current == E.numrows {
			current = 0
		}

		row := &E.row[current]
		match := bytes.Index(row.render, query)
		if match != -1 {
			E.last_match = current
			E.cy = current
			E.cx = editorRowRxToCx(row, match)
			E.rowoff = E.numrows
			break
		}
	}
}

func editorFind() {
	saved_cx := E.cx
	saved_cy := E.cy
	saved_coloff := E.coloff
	saved_rowoff := E.rowoff

	query := editorPrompt("Search: %s (Use ESC/Arrows/Enter)", editorFindCallBack)

	if query == nil {
		E.cx = saved_cx
		E.cy = saved_cy
		E.coloff = saved_coloff
		E.rowoff = saved_rowoff
	}
}

/* output */

func editorScroll() {
	E.rx = 0

	if E.cy < E.numrows {
		E.rx = editorRowCxToRx(&E.row[E.cy], E.cx)
	}

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

		ab = append(ab, []byte("\r\n")...)
	}

	return ab
}

func editorDrawStatusBar(ab []byte) []byte {
	// reverse video swaps foreground and background colors
	ab = append(ab, []byte("\x1b[7m")...)

	filename := E.filename
	if filename == "" {
		filename = "[No Name]"
	}

	sdirty := ""
	if E.dirty > 0 {
		sdirty = "(modified)"
	}

	status := fmt.Sprintf("%.20s - %d lines %s", filename, E.numrows, sdirty)
	status = status[:min(len(status), E.screencols)]
	slen := len(status)

	rstatus := fmt.Sprintf("%d/%d", E.cy+1, E.numrows)
	rlen := len(rstatus)

	ab = append(ab, []byte(status)...)

	for range E.screencols {
		if slen == E.screencols-rlen {
			ab = append(ab, []byte(rstatus)...)
			break
		} else {
			ab = append(ab, " "...)
			slen++
		}
	}
	// return the video mode to normal
	ab = append(ab, []byte("\x1b[m")...)
	ab = append(ab, []byte("\r\n")...)
	return ab
}

func editorDrawMessageBar(ab []byte) []byte {
	ab = append(ab, []byte("\x1b[K")...)
	msg := E.statusmsg[:min(len(E.statusmsg), E.screencols)]
	if time.Since(E.statusmsg_time) < 5*time.Second && len(msg) > 0 {
		ab = append(ab, []byte(msg)...)
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
	ab = editorDrawStatusBar(ab)
	ab = editorDrawMessageBar(ab)

	// move cursor to coordinates
	buf := fmt.Sprintf("\x1b[%d;%dH", (E.cy-E.rowoff)+1, (E.rx-E.coloff)+1)
	ab = append(ab, []byte(buf)...)

	// show cursor
	ab = append(ab, []byte("\x1b[?25h")...)

	os.Stdout.Write(ab)
}

func editorSetStatusMessage(format string, args ...any) {
	E.statusmsg = fmt.Sprintf(format, args...)
	E.statusmsg_time = time.Now()

}

/* input */

func editorPrompt(prompt string, callback func([]byte, int)) []byte {

	buf := make([]byte, 0)
	for {
		editorSetStatusMessage(prompt, buf)
		editorRefreshScreen()

		c := editorReadKey()
		if c == DEL_KEY || c == ctrlKey('h') || c == BACKSPACE {
			if len(buf) != 0 {
				buf = buf[:len(buf)-1]
			}

		} else if c == '\x1b' {
			editorSetStatusMessage("")
			if callback != nil {
				callback(buf, c)
			}
			return nil
		} else if c == '\r' {
			if len(buf) != 0 {
				editorSetStatusMessage("")
				if callback != nil {
					callback(buf, c)
				}
				return buf
			}
		} else if c >= 32 && c < 127 {
			buf = append(buf, byte(c))
		}
		if callback != nil {
			callback(buf, c)
		}

	}
}

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

	if E.cy < E.numrows {
		rowlen := len(E.row[E.cy].chars)
		if E.cx > rowlen {
			E.cx = rowlen
		}
	}

}

func editorProcessKeypress() bool {
	c := editorReadKey()

	switch c {
	case '\r':
		editorInsertNewline()

	case ctrlKey('q'):
		if E.dirty > 0 && E.quit_times > 0 {
			editorSetStatusMessage("WARNING!!! File has unsaved changes. Press Ctrl-q %d more times to quit", E.quit_times)
			E.quit_times--
			return true
		}

		// clear screen
		os.Stdout.Write([]byte("\x1b[2J"))
		// move cursor to top left
		os.Stdout.Write([]byte("\x1b[H"))
		return false

	case ctrlKey('s'):
		editorSave()

	case HOME_KEY:
		E.cx = 0
	case END_KEY:
		if E.cy < E.numrows {
			E.cx = len(E.row[E.cy].chars)
		}

	case ctrlKey('f'):
		editorFind()

	case BACKSPACE, ctrlKey('h'):
		editorDelChar()
	case DEL_KEY:
		editorMoveCursor(ARROW_RIGHT)
		editorDelChar()

	case PAGE_UP:
		E.cy = E.rowoff
		for range E.screenrows {
			editorMoveCursor(ARROW_UP)
		}
	case PAGE_DOWN:
		E.cy = E.rowoff + E.screenrows - 1
		if E.cy > E.numrows {
			E.cy = E.numrows
		}
		for range E.screenrows {
			editorMoveCursor(ARROW_DOWN)
		}
	case ARROW_UP, ARROW_LEFT, ARROW_DOWN, ARROW_RIGHT:
		editorMoveCursor(c)

	case ctrlKey('l'):
	/* TODO */
	case '\x1b':
		/* TODO */

	default:
		editorInsertChar(c)
	}
	E.quit_times = kilo_quit_times
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
	E.dirty = 0
	E.filename = ""
	E.statusmsg = ""
	E.quit_times = kilo_quit_times

	E.screenrows, E.screencols = getWindowSize()
	E.screenrows -= 2

	E.last_match = -1
	E.match_direction = 1
}

func main() {
	enableRawMode()
	defer disableRawMode()
	initEditor()
	if len(os.Args) >= 2 {
		editorOpen(os.Args[1])
	}

	editorSetStatusMessage("HELP: Ctrl-s = save | Ctrl-q = quit | Ctrl-f = find")

	for {
		editorRefreshScreen()
		if !editorProcessKeypress() {
			break
		}
	}
}
