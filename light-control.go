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

// Build (and copy to pi) with:
//   GOARCH=arm go build light-control.go secret.go && scp light-control pi@pi:/home/pi/
//
// First need to stop on pi with:
//   sudo systemctl stop light-control

const (
	max_brightness  = 65535
	min_brightness  = 328  // Minimum brightness that is still on (327 shuts lamp off)
	brightness_step = 2185 // 1/30 of range
	kelvin_step     = 250  // 1/30 of range

	cmdDeadline = 1 * time.Second
	maxDeadline = 10 * time.Second

	MAX_DISCOVER_ATTEMPTS = 10

	MAX_DEVICES = 2 // Number of devices I have on the network; wait to find this many if possible
)

var (
	devs  []light.Device
	conns []net.Conn
	quit  = make(chan struct{})

	cachedColor *lifxlan.Color // To be used for key repeats, otherwise still want to read current value
)

// TODO: Hmm, cachedColor seems to be working in practice, but I think I'm using it from multiple goroutines, so
// it should technically be wrapped in a Mutex or otherwise protected.  In practice, it's hard to see multiple
// threads reading or writing it at the same time, but I'm still pretty sure this is technically wrong.

// Ah, okay, I was thinking, let's just talk to the light on only one goroutine, but already repeater's goroutine
// is the only one calling the key handler, so other than ping, we're already there!  And while ping does call
// getColor, it only ever does so with useCached set to false, so the condition fails out and useCached is never
// read, even for the nil check.  So I believe this is technically correct, although I'm guessing some or most
// engineers might consider it bad form.

func findDevices() {
	log.Printf("Scanning for %v devices...", MAX_DEVICES)

	var found []lifxlan.Device

	for i := 1; ; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(i)*2*time.Second)
		deviceChan := make(chan lifxlan.Device)

		go func() {
			for {
				d := <-deviceChan // Discover closes chan before returning

				if d == nil {
					// deviceChan must be closed -- presumably timeout occured (or other error); just exit goroutine.
					return
				}

				log.Printf("Found one: %v", d)

				found = append(found, d)

				log.Printf("Number of devices found so far: %v", len(found))

				if len(found) == MAX_DEVICES {
					log.Printf("Great!  We've found all %v devices.  Continuing...", MAX_DEVICES)

					cancel() // Note: If we're here because chan was closed, cancel() is still safe to call
					return
				}
			}
		}()

		err := lifxlan.Discover(ctx, deviceChan, "") // Control stays here until cancel, err, or timeout
		if err == context.Canceled {
			log.Printf("Discovered all %v devices: %v", MAX_DEVICES, found)
			break
		}

		log.Printf("Discover failed with err: %v", err)

		// TODO: Set up for continuous scanning, so we an add and remove devices dynamically...
		if len(found) > 0 {
			log.Printf("However, we have %v devices, so we'll continue", len(found))
			break
		}

		found = nil

		time.Sleep(time.Duration(i) * time.Second)

		if i >= MAX_DISCOVER_ATTEMPTS {
			log.Panicf("Discover failed too many times!")
		}
	}

	for i, d := range found {
		log.Printf("Dialing and wrapping device %v of %v: %v", i+1, len(found), d)

		conn, err := d.Dial()
		if err != nil {
			log.Panicf("Device.Dial() error: %v", err)
		}

		dev, err := light.Wrap(context.Background(), d, false)
		if err != nil {
			log.Panicf("light.Wrap error: %v", err)
		}

		conns = append(conns, conn)
		devs = append(devs, dev)
	}

	log.Printf("Initialization complete!")
}

func getColor(deadline time.Duration, useCached bool) *lifxlan.Color {
	if useCached && cachedColor != nil {
		log.Printf("Returning cached color")

		return cachedColor
	}

	dev := devs[0]
	conn := conns[0]

	ctx, cancel := context.WithTimeout(context.Background(), deadline)
	defer cancel()

	start := time.Now()
	color, err := dev.GetColor(ctx, conn)
	log.Printf("GetColor call took %v", time.Since(start))
	if err != nil {
		log.Println("GetColor error:", err)

		if deadline < maxDeadline {
			time.Sleep(2 * time.Second)
			return getColor(deadline * 2, useCached)
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
	for i, conn := range conns {
		dev := devs[i]

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
		} else {
			cachedColor = color
		}

		if err == nil && deadline > cmdDeadline {
			log.Printf("SetColor success after previous error")
		}
	}
}

func makeDimmer(isRepeat bool) {
	color := getColor(cmdDeadline, isRepeat)

	if color.Brightness <= brightness_step {
		color.Brightness = 0
	} else {
		color.Brightness -= brightness_step
	}

	setColor(color, cmdDeadline)
}

func makeBrighter(isRepeat bool) {
	color := getColor(cmdDeadline, isRepeat)

	if color.Brightness >= max_brightness-brightness_step {
		color.Brightness = max_brightness
	} else {
		color.Brightness += brightness_step
	}

	setColor(color, cmdDeadline)
}

func setMinBrightness() {
	color := getColor(cmdDeadline, false)

	color.Brightness = uint16(min_brightness)

	setColor(color, cmdDeadline)
}

// brightness b in range 0 - 1
func setBrightness(b float64) {
	color := getColor(cmdDeadline, false)

	if b < 0 {
		b = 0
	} else if b > 1 {
		b = 1
	}

	color.Brightness = uint16(max_brightness * b)

	setColor(color, cmdDeadline)
}

// temp t in range 0 - 1
func setColorTemp(t float64) {
	color := getColor(cmdDeadline, false)

	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}

	color.Kelvin = lifxlan.KelvinMin + uint16(float64(lifxlan.KelvinMax - lifxlan.KelvinMin) * t)

	setColor(color, cmdDeadline)
}

func makeWarmer(isRepeat bool) {
	color := getColor(cmdDeadline, isRepeat)

	if color.Kelvin <= lifxlan.KelvinMin+kelvin_step {
		color.Kelvin = lifxlan.KelvinMin
	} else {
		color.Kelvin -= kelvin_step
	}

	setColor(color, cmdDeadline)
}

func makeCooler(isRepeat bool) {
	color := getColor(cmdDeadline, isRepeat)

	if color.Kelvin >= lifxlan.KelvinMax-kelvin_step {
		color.Kelvin = lifxlan.KelvinMax
	} else {
		color.Kelvin += kelvin_step
	}

	setColor(color, cmdDeadline)
}

func setPower(pow lifxlan.Power, deadline time.Duration) {
	for i, conn := range conns {
		dev := devs[i]

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
}

func getPower(deadline time.Duration) (pow lifxlan.Power) {
	dev := devs[0]
	conn := conns[0]

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

		for _, conn := range conns {
			conn.Close()
		}

		log.Print("Recovered from a panic; let's run again...")

		main()
	}()

	log.Printf("-------------------- Initializing --------------------")

	findDevices()

	go handleInput("/dev/hidraw0", keys)
	//go handleInput("/dev/hidraw1", dial) // Probably won't need anymore?  There are both...
	go pingLight()

	<-quit
}

// I'd like to get rid of this at some point, but for now I want to regularly talk to light
// and log what happens.
func pingLight() {
	defer func() {
		recover()
		log.Print("pingLight recovered from a panic; let's run again...")
		pingLight()
	}()

	for {
		color := getColor(cmdDeadline, false)
		if color == nil {
			log.Printf("----- keepAlive couldn't reach light!")
		}

		time.Sleep(4 * time.Second)
	}
}

func handleInput(dev string, handle func(byte, bool)) {
	defer func() {
		recover()
		log.Print("handleInput recovered from a panic; let's run again...")
		handleInput(dev, handle)
	}()

	f, err := os.Open(dev)
	if err != nil {
		log.Printf("Error opening file '%s': %v\n", dev, err)
		return
	}
	defer f.Close()

	keyDown := make(chan byte, 256)

	go repeater(keyDown, handle)

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

		keyDown <- b[2]
	}
}

func repeater(keyDown chan byte, handle func(byte, bool)) {
	var key byte

	ticker := &time.Ticker{}
	timer := &time.Timer{}

	for {
		select {
		case key = <-keyDown:
			if key == 0 {
				ticker.Stop()
				timer.Stop()
			} else {
				handle(key, false)
				timer = time.NewTimer(time.Millisecond * 500)
			}

		case <-timer.C:
			handle(key, true)
			ticker = time.NewTicker(time.Millisecond * 100)

		case <-ticker.C:
			handle(key, true)
		}
	}
}

// Seems like we have 2 bytes of 0 followed by a list of the keys that are
// currently down.  Not sure if all 14 are that or if there are other things
// after the keys list.  But in the typical case it's all 0s except for the
// single down key in the third slot (index 2), and then all 0s when that key
// is released.

// At least for now, I'm going to explicitly not handle key combinations, and just
// always look at index 2 to see what single key is considered down, and consider
// it released when that slot is 0.

func keys(k byte, isRepeat bool) {
	log.Printf("Read (or repeating) key code: %v\n", k)

	switch k {
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
	// case 0x3d: // F4 (<<)
	// 	makeWarmer()
	// case 0x3e: // F5 (>>)
	// 	makeCooler()
	case 0x17: // [T]
		setWhite(2000, 1)
	case 0x3a: // [G1] / paddle up
		makeBrighter(isRepeat)
	case 0x2c: // [--] / paddle down
		makeDimmer(isRepeat)
	case 0x3b: // [G2]
		setBrightness(1.0)
	case 0x3c: // [G3]
		setBrightness(0.58)
	case 0x3d: // [G4]
		setBrightness(0.28)
	case 0x3e: // [G5]
		setMinBrightness()
	case 0x1d: // [Z]
		setColorTemp(0.0)
	case 0x1b: // [X]
		setColorTemp(0.25)
	case 0x06: // [C]
		setColorTemp(0.5)
	case 0x19: // [V]
		setColorTemp(0.75)
	case 0x05: // [B]
		setColorTemp(1.0)
	case 0x11: // [N]
		makeWarmer(isRepeat)
	case 0x13: // [P]
		makeCooler(isRepeat)
	case 0:
		// ignore
	default:
		log.Printf("unhandled keycode: 0x%x\n", k)
	}
}

// func dial(k byte) {
// 	if k[0] != 0x3 {
// 		return
// 	}

// 	switch k[1] {
// 	case 0xe9: // dial right
// 		makeBrighter()
// 	case 0xea: // dial left
// 		makeDimmer()
// 	case 0:
// 		// ignore
// 	default:
// 		log.Println("keycode:", k[1])
// 	}
// }
