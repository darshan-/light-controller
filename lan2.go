package main

import (
	"fmt"

	"github.com/2tvenom/golifx"
)

func main() {
	//bulbs, _ := golifx.LookupBulbs()
	var b *golifx.Bulb
	{
		bulbs, err := golifx.LookupBulbs()
		if err != nil {
			fmt.Println("LookupBulbs err:", err)
		}
		b = bulbs[0]
	}

	//bulb.GetPowerState()
	//bulb.SetPowerState(true)

	//var hsbk golifx.HSBK

	state, err := b.GetColorState()
	if err != nil {
		fmt.Println("GetColorState err:", err)
	}

	fmt.Println("Hue       :", state.Color.Hue)
	fmt.Println("Saturation:", state.Color.Saturation)
	fmt.Println("Brightness:", state.Color.Brightness)
	fmt.Println("Kelvin    :", state.Color.Kelvin)

	// hsbk := &golifx.HSBK{
	// 	Hue:        2000,
	// 	Saturation: 13106,
	// 	Brightness: 65535,
	// 	Kelvin:     3200,
	// }

	// bulbs[0].SetColorState(hsbk, 500)
	// 	counter++
	// 	hsbk.Hue += 5000
	// 	if counter > 10 {
	// 		ticker.Stop()
	// 		break
	// 	}
	// }
}
