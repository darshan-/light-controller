package main

import (
	"context"
	"log"
	"net"
	"os"
	"time"

	"github.com/darshan-/lifxlan"
	"github.com/darshan-/lifxlan/light"
)

const (
	max_brightness  = 65535
	brightness_step = 2185 // 1/30 of range // 1966 // 3% of max
	kelvin_step     = 250  // 1/30 of range

	cmdDeadline = 1 * time.Second
	maxDeadline = 10 * time.Second
)

var (
	dev  light.Device
	conn net.Conn
	quit = make(chan struct{})
)

func initLocalDevice() {
	for i := 0; i < 5; i++ {
		err := doInitLocalDevice()
		if err == nil {
			log.Printf("Init complete!")
			return
		}

		time.Sleep(5 * time.Second)
	}

	log.Fatalf("Discover failed too many times!")
}

func doInitLocalDevice() error {
	log.Println("doInitLocalDevice...")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)

	deviceChan := make(chan lifxlan.Device)

	var d lifxlan.Device

	go func() {
		d = <- deviceChan
		cancel()
	}()

	err := lifxlan.Discover(ctx, deviceChan, "") // Control stays here until cancel() is called
	if err != nil && err != context.Canceled {
		log.Printf("Discover failed with err: %v", err)
		return err
	}

	log.Printf("Discovered device: %v", d)

	conn, err = d.Dial()
	if err != nil {
		log.Fatalf("Device.Dial() error: %v", err)
	}

	dev, err = light.Wrap(context.Background(), d, false)
	if err != nil {
		log.Fatalf("light.Wrap error: %v", err)
	}

	return nil
}

func getColor(deadline time.Duration) *lifxlan.Color {
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	color, err := dev.GetColor(ctx, conn)
	if err != nil {
		log.Println("GetColor error:", err)
		if deadline < maxDeadline {
			return getColor(deadline * 2)
		} else {
			log.Fatal("Max deadline exceeded")
		}
	}

	return color
}

func setColor(color *lifxlan.Color, deadline time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	err := dev.SetColor(ctx, conn, color, 75*time.Millisecond, false)
	if err != nil {
		log.Println("SetColor error:", err)
		if deadline < maxDeadline {
			setColor(color, deadline)
		} else {
			log.Fatal("Max deadline exceeded")
		}
	}
}

func makeDimmer() {
	color := getColor(cmdDeadline)
	if color.Brightness <= brightness_step {
		color.Brightness = 0
	} else {
		color.Brightness -= brightness_step
	}
	setColor(color, cmdDeadline)
}

func makeBrighter() {
	color := getColor(cmdDeadline)
	if color.Brightness >= max_brightness-brightness_step {
		color.Brightness = max_brightness
	} else {
		color.Brightness += brightness_step
	}
	setColor(color, cmdDeadline)
}

func makeWarmer() {
	color := getColor(cmdDeadline)
	if color.Kelvin <= lifxlan.KelvinMin+kelvin_step {
		color.Kelvin = lifxlan.KelvinMin
	} else {
		color.Kelvin -= kelvin_step
	}
	setColor(color, cmdDeadline)
}

func makeCooler() {
	color := getColor(cmdDeadline)
	if color.Kelvin >= lifxlan.KelvinMax-kelvin_step {
		color.Kelvin = lifxlan.KelvinMax
	} else {
		color.Kelvin += kelvin_step
	}
	setColor(color, cmdDeadline)
}

func setPower(pow lifxlan.Power, deadline time.Duration) {
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	err := dev.SetPower(ctx, conn, pow, false)
	if err != nil {
		log.Println("SetPower error:", err)
		if deadline < maxDeadline {
			setPower(pow, deadline*2)
		} else {
			log.Fatal("Max deadline exceeded")
		}
	}
}

func getPower(deadline time.Duration) (pow lifxlan.Power) {
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	pow, err := dev.GetPower(ctx, conn)
	if err != nil {
		log.Println("GetPower error:", err)
		if deadline < maxDeadline {
			return getPower(deadline * 2)
		} else {
			log.Fatal("Max deadline exceeded")
		}
	}

	return
}

// Note -- with current cat /dev/input/event0 >>fifo& approach, if you accidentally do that more than once,
// you'll end up with 2 (or more) copies of each event.  If that's even, toggles won't work.  If it's greater
// than one, that'll move things like brightness by too much.  Let's do a script to remove and set up fifo on
// launch.
func togglePower() {
	log.Println("togglePower")
	if getPower(cmdDeadline) != lifxlan.PowerOn {
		setPower(lifxlan.PowerOn, cmdDeadline)
	} else {
		setPower(lifxlan.PowerOff, cmdDeadline)
	}
}

func setWhite(k uint16, b float32) {
	setColor(&lifxlan.Color{Kelvin: k, Brightness: uint16(b * 65535)}, cmdDeadline)
	setPower(lifxlan.PowerOn, cmdDeadline)
}

func main() {
	log.Printf("-------------------- Initializing --------------------")

	initLocalDevice()

	go keys()
	go dial()
	go keepAlive()

	<-quit
}

// Exponential backoff worked, but I don't want to wait 15 seconds (1 + 2 + 4 + 8 (and then instantly work))
// to turn on my freaking light, and it really seems like either the pi (my guess) or the light
// (seems much less likely) is losing its connection.
//
// So I want to try regularly pinging the light with a getColor request and see if that resolves the issue

// Keep alive seems to be working -- getting a lot of timeouts, but those seem to work right away on second
// try, so so far seems to be keeping network active.

// Finally googled ("braved") [raspberry pi wifi powers down] and found suggesion that wifi does power down,
// lots of people complaining, and some folks saying you can check power save setting with:
//
//	iw dev wlan0 get power_save
//
// and set it with:
//
//	sudo iw dev wlan0 set power_save off
//
// Just checked, and that is reset to on when restarting the device.  So I want to check if I can quickly and
// easily figure out how to set that as the default, but if not, the easy solution is to just turn it off in
// start-up script.  If fact, that may actually be the best approach, because it keeps everything more self-
// contained and easier to set up if/when I move to another device.  Yeah, I think I will just do that.
//
// It's crazy how mixed the reports are of that fixing things versus doing nothing, and some people claiming
// with some authority that the value is reported incorrectly, but that the driver is hard-coded to never
// power save.  So, we'll try this for now, and see.  Maybe it helps, and we won't see the timeouts for our
// keepAlive getColor anymore, and everything will be perfect, which'd be great.  Or maybe it won't help, and
// keepAlive will come to the rescue.  Or maybe neithe thing will work, and we'll be down to 15-second delays
// sometimes, which would suck, but even *that* is an improvement over the status quo.  We'll see.

// Huh, okay, so I was starting to ge my hopse up that wifi power management did the trick, but after 4 hours
// and 15 minutes, we started getting a bunch of deadlines exceeded again.  But keepAlive does seem to be
// compensating effectively, so for now switch still seems to work every time I use it.  I still don't like
// this, because I could still get unlucky, and I hate that, but at least it's a lot better than it was...
func keepAlive() {
	for {
		color := getColor(cmdDeadline)
		if color == nil {
			log.Printf("----- keepAlive couldn't reach light!")
		}

		time.Sleep(4 * time.Second)
	}
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
