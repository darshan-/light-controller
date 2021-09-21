package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/darshan-/lifxlan"
	"github.com/darshan-/lifxlan/light"
	//"go.yhsif.com/lifxlan"
	//"go.yhsif.com/lifxlan/light"
)

// I think my net.Conn might sometimes go stale?
// On error, maybe try redialing?  Even rediscovering device?

func main() {
	//ctx, cancel := context.WithCancel(context.Background())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	deviceChan := make(chan lifxlan.Device)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := lifxlan.Discover(ctx, deviceChan, ""); err != nil {
			if err != nil && err != context.Canceled && err != context.DeadlineExceeded {
				log.Fatalf("Discover failed: %v", err)
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

	ld, err := light.Wrap(context.Background(), d, false)
	if err != nil {
		fmt.Println("light.Wrap error:", err)
		return
	}

	color := getColor(ld, conn)
	//fmt.Println(color)
	fmt.Println("Hue       :", color.Hue)
	fmt.Println("Saturation:", color.Saturation)
	fmt.Println("Brightness:", color.Brightness)
	fmt.Println("Kelvin    :", color.Kelvin)

	color.Brightness += 655 // 2^16 / 100
	setColor(ld, conn, color)
}

func getColor(d light.Device, conn net.Conn) *lifxlan.Color {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	color, err := d.GetColor(ctx, conn)
	if err != nil {
		fmt.Println("GetColor error:", err)
		return nil
	}

	return color
}

func setColor(d light.Device, conn net.Conn, color *lifxlan.Color) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := d.SetColor(ctx, conn, color, 0, false)
	if err != nil {
		fmt.Println("SetColor error:", err)
		return
	}

	fmt.Println("Set color:", color)
	fmt.Println(getColor(d, conn))
}
