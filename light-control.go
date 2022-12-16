package main

import (
	"context"
	"log"
	"net"
	"os"
	"reflect"
	"runtime"
	"time"

	"github.com/darshan-/lifxlan"
	"github.com/darshan-/lifxlan/light"
)

const (
	max_brightness  = 65535
	brightness_step = 2185 // 1/30 of range
	kelvin_step     = 250  // 1/30 of range

	cmdDeadline = 1 * time.Second
	maxDeadline = 10 * time.Second

	MAX_DISCOVER_ATTEMPTS = 10
)

var (
	dev  light.Device
	conn net.Conn
	quit = make(chan struct{})
)

func findDevice() {
	log.Println("Scanning for devices...")

	var d lifxlan.Device

	for i := 1; ; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i)*2*time.Second)
		deviceChan := make(chan lifxlan.Device)

		go func() {
			d = <-deviceChan // Discover closes chan before returning
			cancel()         // If we're here because chan was closed, cancel() is still safe to call
		}()

		err := lifxlan.Discover(ctx, deviceChan, "") // Control stays here until cancel, err, or timeout
		if err == context.Canceled {
			log.Printf("Discovered device: %v", d)
			break
		}

		log.Printf("Discover failed with err: %v", err)

		time.Sleep(time.Duration(i) * time.Second)

		if i > MAX_DISCOVER_ATTEMPTS {
			log.Panicf("Discover failed too many times!")
		}
	}

	var err error
	conn, err = d.Dial()
	if err != nil {
		log.Panicf("Device.Dial() error: %v", err)
	}

	dev, err = light.Wrap(context.Background(), d, false)
	if err != nil {
		log.Panicf("light.Wrap error: %v", err)
	}

	log.Printf("Initialization complete!")
}

func getColor(deadline time.Duration) *lifxlan.Color {
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	color, err := dev.GetColor(ctx, conn)
	if err != nil {
		log.Println("GetColor error:", err)
		if deadline < maxDeadline {
			time.Sleep(2 * time.Second)
			return getColor(deadline * 2)
		} else {
			log.Panicf("Max deadline exceeded")
		}
	}

	if err == nil && deadline > cmdDeadline {
		log.Printf("GetColor success after previous error")
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
			time.Sleep(2 * time.Second)
			setColor(color, deadline)
		} else {
			log.Panicf("Max deadline exceeded")
		}
	}

	if err == nil && deadline > cmdDeadline {
		log.Printf("SetColor success after previous error")
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
			time.Sleep(2 * time.Second)
			setPower(pow, deadline*2)
		} else {
			log.Panicf("Max deadline exceeded")
		}
	}

	if err == nil && deadline > cmdDeadline {
		log.Printf("SetPower success after previous error")
	}
}

func getPower(deadline time.Duration) (pow lifxlan.Power) {
	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()
	pow, err := dev.GetPower(ctx, conn)
	if err != nil {
		log.Println("GetPower error:", err)
		if deadline < maxDeadline {
			time.Sleep(2 * time.Second)
			return getPower(deadline * 2)
		} else {
			log.Panicf("Max deadline exceeded")
		}
	}

	if err == nil && deadline > cmdDeadline {
		log.Printf("GetPower success after previous error")
	}

	return
}

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
	log.SetFlags(log.Ldate | log.Lmicroseconds)

	defer func() {
		recover()
		conn.Close()
		log.Print("Recovered from a panic; let's run again...")
		main()
	}()

	log.Printf("-------------------- Initializing --------------------")

	findDevice()

	go handleinput("/dev/hidraw0", keys)
	go handleinput("/dev/hidraw1", dial)

	<-quit
}

func handleinput(dev string, handle func([]byte)) {
	f, err := os.Open(dev)
	if err != nil {
		log.Printf("Error opening file '%s': %v\n", dev, err)
		return
	}
	defer f.Close()

	handlerName := runtime.FuncForPC(reflect.ValueOf(handle).Pointer()).Name()[5:]
	log.Printf("Opened %s with handler: %s", dev, handlerName)

	b := make([]byte, 16)

	for {
		n, err := f.Read(b)
		if err != nil {
			log.Printf("Error reading file: %v\n", err)
			return
		}
		log.Printf("read %d bytes for handler '%s': %d\n", n, handlerName, b[:n])

		handle(b)
	}
}

func keys(k []byte) {
	switch k[2] {
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
		log.Println("unhandled keycode:", k[2])
	}
}

func dial(k []byte) {
	if k[0] != 0x3 {
		return
	}

	switch k[1] {
	case 0xe9: // dial right
		makeBrighter()
	case 0xea: // dial left
		makeDimmer()
	case 0:
		// ignore
	default:
		log.Println("keycode:", k[1])
	}
}
