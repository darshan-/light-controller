package main

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"go.yhsif.com/lifxlan"
	"go.yhsif.com/lifxlan/light"
)

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
			tryGetColor(device)

			// I'm not currently looking for more than one device; so just cancel once we get it
			cancel()
		}(device)
	}

	wg.Wait()
}

func tryGetColor(d lifxlan.Device) {
	device, err  := light.Wrap(context.Background(), d, false)
	if err != nil {
		fmt.Println("light.Wrap error:", err)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	color, err := device.GetColor(ctx, nil)
	if err != nil {
		fmt.Println("GetColor error:", err)
		return
	}

	fmt.Println(color)
}
