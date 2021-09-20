package main

import (
	"fmt"
	"net/http"
	"os"
	"strings"
)

/*
Disable ttys and X
*/

// Having first done `mkfifo fifo`
// cat /dev/input/event0 >>fifo&
// cat /dev/input/event1 >>fifo&

const lifxStateUrl = "https://api.lifx.com/v1/lights/all/state"

func power(state string) {
	putReq("application/x-www-form-urlencoded", "power=" + state)
}

func putJson(json string) {
	putReq("application/json", json)
}

func setWhite(k, b string) {
	putJson(`{"color": "kelvin:` + k + ` brightness:` + b + `", "power": "on"}`)
}

func putReq(contentType, body string) {
	req, err := http.NewRequest(http.MethodPut, lifxStateUrl, strings.NewReader(body))
	if err != nil {
		fmt.Println("Error creating request:", err)
		return
	}

	req.Header.Add("Authorization", "Bearer " + lifx_token)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("Error doing request:", err)
		return
	}
	defer resp.Body.Close()
}

func main() {
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
			// 8 bytes for timestamp struct, then:
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
					case 1: // [ESC]
						power("off")
					case 2: // [1]
						setWhite("1500", "0.25")
					case 3: // [2]
						setWhite("2000", "0.35")
					case 4: // [3]
						setWhite("2700", "0.5")
					case 5: // [4]
						setWhite("3500", "0.75")
					case 6: // [5]
						setWhite("4300", "1")
					case 7: // [6]
						setWhite("5200", "1")
					case 41: // [`]
						power("on")
					case 114: // dial left
					case 115: // dial right
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
