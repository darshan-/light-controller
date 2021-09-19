package main

import (
	"fmt"
	"os"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/rjeczalik/notify"
)

// Use cat to do some magic, I dont know what, so let's cheat and have it help us:
//  cat /dev/input/by-id/usb-SONiX_USB-Keyboard-event-kbd >>/home/pi/kbd
//  Then run this program

// Ooh!  Got it!  Keys are event0 (linked to from bi-* dirs) but wheel is in event 1, which *isn't*
//  linked to by either bi-* dir.  So
//    cat /dev/input/event0 >>kbd&
//    cat /dev/input/event1 >>kbd&

// Hmm, or maybe even read from stdin and do this?:
//  cat </dev/input/event0 </dev/input/event1 | ./light-control
// No idea if that would work...
// Nope, it's trying to finish first (event1, think?) first, then concatenate; not merging in realtime

// Maybe a FIFO?  The idea here is maybe reading from stdin rather than using watcher, so I don't read
//  a file that keeps growing, just read, with hopefully blocking working correctly and not getting EOF
//  like I do from /dev file.  (Although... *is* there a way to determine *real* EOF?)

// /dev/input/by-path/platform-fd500000.pcie-pci-0000\:01\:00.0-usb-0\:1.4.4\:1.1-event on pi
// Or maybe /dev/input/by-id/usb-SONiX_USB-Keyboard-event-kbd

func main() {
	fmt.Println("Hi!")
	old()
}

func n() {
	fmt.Println("n")

	c := make(chan notify.EventInfo, 1)

	// if err := notify.Watch("/dev/input/by-id/usb-SONiX_USB-Keyboard-event-kbd", c, notify.InModify); err != nil {
	// 	fmt.Printf("Error setting up watch: %v\n", err)
	// 	return
	// }
	//err := notify.Watch("/dev/input/by-id/usb-SONiX_USB-Keyboard-event-kbd", c,
	err := notify.Watch("/home/pi/kbd", c,
		notify.InAccess, notify.InModify, notify.InCloseWrite, notify.InCloseNowrite, notify.InOpen)
	if err != nil {
		fmt.Printf("Error setting up watch: %v\n", err)
		return
	}
	defer notify.Stop(c)

	for {
		// Block until an event is received.
		switch ei := <-c; ei.Event() {
		default:
			fmt.Println("Got an event!")
		}
	}
}

func fsn() {
	fmt.Println("fsn")

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		fmt.Printf("Error creatnig new watcher: %v\n", err)
		return
	}
	defer watcher.Close()

	err = watcher.Add("/dev/input/by-id/usb-SONiX_USB-Keyboard-event-kbd")
	if err != nil {
		fmt.Printf("Error adding file to watcher: %v\n", err)
		return
	}

	fmt.Println("Added file to watcher")

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()

		fmt.Println("Goroutine started")

		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}

				fmt.Println("Got an event! (%v)", event)
				// if event.Op&fsnotify.Write == fsnotify.Write {
				// 	log.Println("modified file:", event.Name)
				// }
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}

				fmt.Printf("Watcher error: %v\n", err)
			}
		}
	}()

	wg.Wait()

	// for {
	// 	char, key, err := KbdGetKey()
	// 	if err != nil {
	// 		panic(err)
	// 	}
	// 	fmt.Printf("You pressed: rune %q, key %X\r\n", char, key)
	// 	if key == KeyEsc {
	// 		break
	// 	}
	// }
}

func old() {
	fmt.Println("old!")

	//f, err := os.Open("/dev/input/by-id/usb-SONiX_USB-Keyboard-event-kbd")
	//f, err := os.Open("/home/pi/kbd")
	//f, err := os.Open("/dev/input/event0")
	f, err := os.Open("/home/pi/fifo")
	if err != nil {
		fmt.Printf("Error opening file: %v\n", err)
		return
	}
	defer f.Close()

	fmt.Println("Opened dev file")

	b := make([]byte, 16)

	// If shift is down when we launch, that's messy.  We could turn shift to true for autorepeat, but we'd still
	//  potentially get another input (like dial rotation) while shift was down without knowing it at first...
	// So not perfect, but good enough for my personal use.
	shift := false

	for {
		n, err := f.Read(b)
		if err != nil {
			fmt.Printf("Error reading file: %v\n", err)
			return
		}
		if n > 0 {
			// for i:=0; i < 16; i++ {
			// 	fmt.Printf("%02X ", int(b[i]))
			// 	if i % 4 == 3 {
			// 		fmt.Printf(" ")
			// 	}
			// }
			// fmt.Println("")

			// Skip 8 bytes for timestamp struct, then:
			typ := uint16(b[8])
			code := uint16(b[10])
			value := uint16(b[12])

			fmt.Printf("%v, %v, %v\n", typ, code, value)
			if typ == 1 { // Key event
				if value == 1 { // key down
					switch code {
					case 114: // dial left
						if shift {
							fmt.Println("turn down the kelvin")
						} else {
							fmt.Println("turn down the brightness")
						}
					case 115: // dial right
						if shift {
							fmt.Println("turn up the kelvin")
						} else {
							fmt.Println("turn up the brightness")
						}
					case 42: // shift
						shift = true
					}
				} else if value == 0 { // key up; only care for modifier keys
					switch code {
					case 42: // shift
						fmt.Println("shift up")
						shift = false
					default:
						fmt.Println("something else up")
					}
				} else if value == 2 { // value == 2 is for autorepeat, which we dont' care about
					switch code {
					case 42: // shift
						shift = true
					}
				}
			}
		} else {
			fmt.Println("!")
		}
	}
}
