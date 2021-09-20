package main

import (
	//"bytes"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"syscall"
	"unsafe"

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

// Having first done `mkfifo fifo`
// cat /dev/input/event0 >>fifo&
// cat /dev/input/event1 >>fifo&

// #define EVIOCGKEY(len)		_IOC(_IOC_READ, 'E', 0x18, len)
/*
#define _IOC(dir,type,nr,size) \
	(((dir)  << _IOC_DIRSHIFT) | \
	 ((type) << _IOC_TYPESHIFT) | \
	 ((nr)   << _IOC_NRSHIFT) | \
	 ((size) << _IOC_SIZESHIFT))

#define _IOC_READ	2U
#define _IOC_NRBITS	8
#define _IOC_TYPEBITS	8
#define _IOC_SIZEBITS	14
#define _IOC_DIRBITS	2
#define _IOC_NRSHIFT	0
#define _IOC_TYPESHIFT	(_IOC_NRSHIFT+_IOC_NRBITS)
#define _IOC_SIZESHIFT	(_IOC_TYPESHIFT+_IOC_TYPEBITS)
#define _IOC_DIRSHIFT	(_IOC_SIZESHIFT+_IOC_SIZEBITS)
*/


const KEY_MAX = 0x2ff

var key_map [KEY_MAX/8 + 1]uint8
var kmp = uintptr(unsafe.Pointer(&key_map))

/*
const ioc_read = 2
const ioc_nrbits = 8
const ioc_typebits = 8
const ioc_sizebits = 14
const ioc_dirbits = 2
const ioc_nrshift = 0
const ioc_typeshift = ioc_nrshift + 
*/

const eviocgkey = 2 << (8+8+14) | 69 << 8 | 0x18 << 0 | (KEY_MAX/8 + 1) << (8+8)
var fd uintptr

func main() {
	fmt.Println("Hi!")

	f, err := os.Open("/dev/input/event0")
	if err != nil {
		fmt.Printf("Error opening device file: %v\n", err)
		return
	}
	defer f.Close()
	fd = f.Fd()

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


// https://www.arv.io/articles/raw-keyboard-input-erlang-linux
// https://elixir.bootlin.com/linux/latest/source/include/uapi/asm-generic/ioctl.h
func shiftDown() bool {
	//ioctl(fd, EVIOCGKEY(sizeof(key_map)), key_map);

	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, eviocgkey, kmp)
	if errno != 0 {
		fmt.Println("Ioctl return error code:", errno)
	}


	// int keyb = key_map[key/8];  //  The key we want (and the seven others arround it)
    // int mask = 1 << (key % 8);  //  Put a one in the same column as out key state will be in;

	for i := 0; i < len(key_map); i++ {
		if key_map[i] != 0 {
			fmt.Printf("index %v is %v\n", i, key_map[i])
		}
	}

	return false
}

const lifxStateUrl = "https://api.lifx.com/v1/lights/all/state"

func power(state string) {
	//body := bytes.NewBuffer(`{"color": "kelvin:3500 brightness:1", "power": "on"`)
	//body := bytes.NewBuffer([]byte(`power=off`))
	body := strings.NewReader("power=" + state)
	req, err := http.NewRequest(http.MethodPut, lifxStateUrl, body)

	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	//req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", "Bearer " + lifx_token)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	//fmt.Println(req)
	//httpClient := &http.Client{}

	//resp, err := httpClient.Do(req)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Error doing request:", err)
		return
	}
	defer resp.Body.Close()
	//fmt.Println("Got resp:", resp)
}

func turnOff() {
	power("off")
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

			//fmt.Printf("%v, %v, %v\n", typ, code, value)
			if typ == 1 { // Key event
				if value == 1 { // key down
					switch code {
						// 1 key -> keycode2 and up by ones through 6 key -> keycode 7
						// QWERT -> 16-20
						// ASDFG -> 30-34
						// ZXCVB -> 44-48
						// TAB 15, CAPS 58, SHIFT 42, CTRL 29, ALT 56
						// ESC 1, F1-F6 -> 59-63, `~ 41
						//
					case 1: // ESC
						power("off")
					case 41: // `
						power("on")
					case 114: // dial left
						if shiftDown() {
							fmt.Println("turn down the kelvin")
						} else {
							fmt.Println("turn down the brightness")
						}
					case 115: // dial right
						if shiftDown() {
							fmt.Println("turn up the kelvin")
						} else {
							fmt.Println("turn up the brightness")
						}
					default:
						fmt.Println("keycode:", code)
					}
				} else if value == 0 { // key up, which we don't care about
				} else if value == 2 { // autorepeat, which we don't care about
				}
			}
		} else {
			fmt.Println("!")
		}
	}
}
