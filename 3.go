package main

import (
	"fmt"
	"github.com/gdamore/tcell/v2"
	//"github.com/gdamore/tcell/v2/encoding"
	"os"
	"time"
)

var defStyle tcell.Style

func main() {
	s, e := tcell.NewScreen()

	if e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}
	if e := s.Init(); e != nil {
		fmt.Fprintf(os.Stderr, "%v\n", e)
		os.Exit(1)
	}

	defStyle = tcell.StyleDefault.Background(tcell.ColorReset).Foreground(tcell.ColorReset)

	s.SetStyle(defStyle)
	s.Clear()

	ecnt := 0

	defer s.Fini()
	events := make(chan tcell.Event)
	go func() {
		for {
			ev := s.PollEvent()
			events <- ev
		}
	}()

	go func() {
		for {
			ev := <-events
			switch ev := ev.(type) {
			case *tcell.EventKey:
				fmt.Println("Got *some* key event!")
				if ev.Key() == tcell.KeyEscape {
					ecnt++
					if ecnt > 1 {
						s.Fini()
						os.Exit(0)
					}
				} else if ev.Key() == tcell.KeyEnter {
				} else if ev.Key() == tcell.KeyRune {
				} else if ev.Key() == tcell.KeyBackspace2 || ev.Key() == tcell.KeyBackspace {
				}
			}
		}
	}()

	t := time.NewTicker(time.Second)
	for {
		select {
		case <-t.C:
		}

	}
}
