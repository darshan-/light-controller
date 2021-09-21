package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/darshan-/lifxlan"
	"github.com/darshan-/lifxlan/light"
)

/*
Disable ttys and X
*/

// Having first done `mkfifo fifo`
// cat /dev/input/event0 >>fifo&
// cat /dev/input/event1 >>fifo&

var dev light.Device
var conn net.Conn

func initLocalDevice() {
	ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
	defer cancel()

	deviceChan := make(chan lifxlan.Device)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := lifxlan.Discover(ctx, deviceChan, ""); err != nil {
			if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				fmt.Printf("Discover failed: %v\n", err)
				return
			}
		}
	}()

	for device := range deviceChan {
		wg.Add(1)
		go func(device lifxlan.Device) {
			defer wg.Done()
			fmt.Println(device)
			gotDevice(device)

			// I'm not currently looking for more than one device; so just cancel once we get it
			cancel()
		}(device)
	}

	wg.Wait()
}

func gotDevice(d lifxlan.Device) {
	conn, err := d.Dial()
	if err != nil {
		fmt.Println("Device.Dial() error:", err)
		return
	}
	defer conn.Close() // Good idea?

	dev, err = light.Wrap(context.Background(), d, false)
	if err != nil {
		fmt.Println("light.Wrap error:", err)
		return
	}
}

func getColor() *lifxlan.Color {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	color, err := dev.GetColor(ctx, conn)
	if err != nil {
		fmt.Println("GetColor error:", err)
		return nil
	}

	return color
}

func setColor(color *lifxlan.Color) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := dev.SetColor(ctx, conn, color, 0, false)
	if err != nil {
		fmt.Println("SetColor error:", err)
		return
	}
}

func setPower(pow lifxlan.Power) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := dev.SetPower(ctx, conn, pow, false)
	if err != nil {
		fmt.Println("SetPower error:", err)
		return
	}
}

func getPower() (pow lifxlan.Power) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	pow, err := dev.GetPower(ctx, conn)
	if err != nil {
		fmt.Println("GetPower error:", err)
	}

	return
}

// Note -- with current cat /dev/input/event0 >>fifo& approach, if you accidentally do that more than once,
//  you'll end up with 2 (or more) copies of each event.  If that's even, toggles won't work.  If it's greater
//  than one, that'll move things like brightness by too much.  Let's do a script to remove and set up fifo on
//  launch.
func togglePower() {
	fmt.Println("togglePower")
	if getPower() != lifxlan.PowerOn {
		setPower(lifxlan.PowerOn)
	} else {
		setPower(lifxlan.PowerOff)
	}
}

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

	initLocalDevice()

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
						//power("off")
						togglePower()
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
