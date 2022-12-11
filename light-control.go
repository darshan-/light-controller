package main

import (
	"context"
	"log"
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

const (
	max_brightness  = 65535
	brightness_step = 2185 // 1/30 of range // 1966 // 3% of max
	kelvin_step     = 250  // 1/30 of range

	cmdDeadline = 3 * time.Second
)

var (
	dev  light.Device
	conn net.Conn
	quit = make(chan struct{})
)

func initLocalDevice() {
	log.Println("initLocalDevice top")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	deviceChan := make(chan lifxlan.Device)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := lifxlan.Discover(ctx, deviceChan, ""); err != nil {
			if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				log.Printf("Discover failed: %v\n", err)
				return
			}
		}
	}()

	for device := range deviceChan {
		wg.Add(1)
		go func(device lifxlan.Device) {
			defer wg.Done()
			log.Println(device)
			gotDevice(device)

			// I'm not currently looking for more than one device; so just cancel once we get it
			cancel()
		}(device)
	}

	wg.Wait()
	log.Println("initLocalDevice bottom")
}

func gotDevice(d lifxlan.Device) {
	conn, err := d.Dial()
	if err != nil {
		log.Println("Device.Dial() error:", err)
		return
	}
	defer conn.Close() // Good idea?

	dev, err = light.Wrap(context.Background(), d, false)
	if err != nil {
		log.Println("light.Wrap error:", err)
		return
	}
}

func getColor() *lifxlan.Color {
	ctx, cancel := context.WithTimeout(context.Background(), cmdDeadline)
	defer cancel()
	color, err := dev.GetColor(ctx, conn)
	if err != nil {
		log.Println("GetColor error:", err)
		return nil
	}

	return color
}

func setColor(color *lifxlan.Color) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdDeadline)
	defer cancel()
	err := dev.SetColor(ctx, conn, color, 75*time.Millisecond, false)
	if err != nil {
		log.Println("SetColor error:", err)
		return
	}
}

func makeDimmer() {
	color := getColor()
	if color.Brightness <= brightness_step {
		color.Brightness = 0
	} else {
		color.Brightness -= brightness_step
	}
	setColor(color)
}

func makeBrighter() {
	color := getColor()
	if color.Brightness >= max_brightness-brightness_step {
		color.Brightness = max_brightness
	} else {
		color.Brightness += brightness_step
	}
	setColor(color)
}

func makeWarmer() {
	color := getColor()
	if color.Kelvin <= lifxlan.KelvinMin+kelvin_step {
		color.Kelvin = lifxlan.KelvinMin
	} else {
		color.Kelvin -= kelvin_step
	}
	setColor(color)
}

func makeCooler() {
	color := getColor()
	if color.Kelvin >= lifxlan.KelvinMax-kelvin_step {
		color.Kelvin = lifxlan.KelvinMax
	} else {
		color.Kelvin += kelvin_step
	}
	setColor(color)
}

func setPower(pow lifxlan.Power) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdDeadline)
	defer cancel()
	err := dev.SetPower(ctx, conn, pow, false)
	if err != nil {
		log.Println("SetPower error:", err)
		return
	}
}

func getPower() (pow lifxlan.Power) {
	ctx, cancel := context.WithTimeout(context.Background(), cmdDeadline)
	defer cancel()
	pow, err := dev.GetPower(ctx, conn)
	if err != nil {
		log.Println("GetPower error:", err)
	}

	return
}

// Note -- with current cat /dev/input/event0 >>fifo& approach, if you accidentally do that more than once,
//
//	you'll end up with 2 (or more) copies of each event.  If that's even, toggles won't work.  If it's greater
//	than one, that'll move things like brightness by too much.  Let's do a script to remove and set up fifo on
//	launch.
func togglePower() {
	log.Println("togglePower")
	if getPower() != lifxlan.PowerOn {
		setPower(lifxlan.PowerOn)
	} else {
		setPower(lifxlan.PowerOff)
	}
}

const lifxStateUrl = "https://api.lifx.com/v1/lights/all/state"

func power(state string) {
	putReq("application/x-www-form-urlencoded", "power="+state)
}

func putJson(json string) {
	putReq("application/json", json)
}

func setWhite(k uint16, b float32) {
	//putJson(`{"color": "kelvin:` + k + ` brightness:` + b + `", "power": "on"}`)
	setColor(&lifxlan.Color{Kelvin: k, Brightness: uint16(b * 65535)})
	setPower(lifxlan.PowerOn)
}

func putReq(contentType, body string) {
	req, err := http.NewRequest(http.MethodPut, lifxStateUrl, strings.NewReader(body))
	if err != nil {
		log.Println("Error creating request:", err)
		return
	}

	req.Header.Add("Authorization", "Bearer "+lifx_token)
	req.Header.Set("Content-Type", contentType)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Println("Error doing request:", err)
		return
	}
	defer resp.Body.Close()
}

func main() {
	log.Printf("Launching")

	sleepDur := 2 * time.Second
	for sleepDur <= 32*time.Second {
		initLocalDevice()
		log.Println("initLocalDevice returned...")

		if dev != nil {
			break
		}

		log.Printf("dev is null, sleeping for %v\n", sleepDur)

		time.Sleep(sleepDur)
		sleepDur *= 2
	}

	if dev == nil {
		log.Fatal("Couldn't get a device!")
	} else {
		log.Println("Got lifx device!")
	}

	go keys()
	go dial()

	<-quit
}

func keys() {
	f, err := os.Open("/dev/hidraw0")
	if err != nil {
		log.Printf("Error opening file: %v\n", err)
		return
	}
	defer f.Close()

	log.Println("Opened /dev/hidraw0 for keys")

	b := make([]byte, 16)

	for {
		n, err := f.Read(b)
		if err != nil {
			log.Printf("Error reading file: %v\n", err)
			return
		}
		log.Printf("read %d bytes: %#v\n", n, b)

		switch b[2] {
		case 0x29: // [ESC]
			togglePower()
		case 0x1e: // [1]
			setWhite(1500, 0.25)
		case 0x1f: // [2]
			setWhite(2000, 0.35)
		case 0x20: // [3]
			setWhite(2700, 0.5)
		case 0x21: // [4]
			setWhite(3500, 0.75)
		case 0x22: // [5]
			setWhite(4300, 1)
		case 0x23: // [6]
			setWhite(5200, 1)
		case 0x3d: // F4 (<<)
			makeWarmer()
		case 0x3e: // F5 (>>)
			makeCooler()
		case 0:
			// ignore
		default:
			log.Println("keycode:", b[2])
		}
	}
}

func dial() {
	f, err := os.Open("/dev/hidraw1")
	if err != nil {
		log.Printf("Error opening file: %v\n", err)
		return
	}
	defer f.Close()

	log.Println("Opened /dev/hidraw1 for dial")

	b := make([]byte, 16)

	for {
		n, err := f.Read(b)
		if err != nil {
			log.Printf("Error reading file: %v\n", err)
			return
		}
		log.Printf("read %d bytes: %#v\n", n, b)

		if b[0] != 0x3 {
			continue
		}

		switch b[1] {
		case 0xe9: // dial right
			makeBrighter()
		case 0xea: // dial left
			makeDimmer()
		case 0:
			// ignore
		default:
			log.Println("keycode:", b[1])
		}
	}
}
