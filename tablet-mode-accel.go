package main

import (
	"flag"
	"fmt"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/godbus/dbus"
	"github.com/gosuri/uilive"
	"github.com/holoplot/go-evdev"
)

const interval = 1 * time.Second

type accelerometer struct {
	x float64
	y float64
	z float64
}

const (
	unknown int = iota
	laptop
	tablet
)

func main() {
	debug := flag.Bool("debug", false, "show accelerometer values and calculations")
	flag.Parse()

	if *debug {
		runDebug()
	} else {
		runSwitch()
	}
}

func runDebug() {
	writer := uilive.New()
	writer.Start()

	for {
		time.Sleep(interval)

		disp, err := readDisplayAccel()
		if err != nil {
			continue
		}
		base, err := readBaseAccel()
		if err != nil {
			continue
		}

		haa := hingeAxleAngle(disp)
		if haa < 10 || haa > 170 {
			fmt.Fprintf(writer, "Hinge axle angle: %.1f°", haa)
		} else {
			da := math.Atan2(disp.z, disp.x) * 180 / math.Pi
			ba := math.Atan2(base.z, base.x) * 180 / math.Pi
			lc := lidClosed()
			ha := hingeAngle(disp, base, lc)

			var tm string
			switch tabletMode(ha) {
			case unknown:
				tm = "unknown"
			case laptop:
				tm = "laptop"
			case tablet:
				tm = "tablet"
			}

			fmt.Fprintf(writer, `Hinge axle angle: %.1f°
Display angle: %.1f°
Base angle: %.1f°
Hinge angle: %.1f°
Lid closed: %t

Tablet mode: %s
`, haa, da, ba, ha, lc, tm)
		}
	}
}

func runSwitch() {
	dev, err := evdev.CreateDevice(
		"Software Tablet Mode",
		evdev.InputID{
			BusType: 0x03,
			Vendor:  0x4711,
			Product: 0x0816,
			Version: 1,
		},
		map[evdev.EvType][]evdev.EvCode{
			evdev.EV_SW: {
				evdev.SW_TABLET_MODE,
			},
		},
	)
	if err != nil {
		fmt.Printf("failed to create device: %s\n", err.Error())
		os.Exit(1)
	}

	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)

	lastMode := unknown
exit:
	for {
		select {
		case <-c:
			break exit
		case <-time.After(interval):
			disp, err := readDisplayAccel()
			if err != nil {
				break
			}

			base, err := readBaseAccel()
			if err != nil {
				break
			}

			hingeAxleAngle := hingeAxleAngle(disp)
			// We cannot calc hinge angle reliable if hinge axle is almost vertical
			if hingeAxleAngle < 10 || hingeAxleAngle > 170 {
				break
			}
			hingeAngle := hingeAngle(disp, base, lidClosed())

			mode := tabletMode(hingeAngle)
			if lastMode != mode {
				if mode == tablet {
					fmt.Println("SW_TABLET_MODE 1")
					writeSwTabletMode(dev, true)
				} else {
					fmt.Println("SW_TABLET_MODE 0")
					writeSwTabletMode(dev, false)
				}
				lastMode = mode
			}
		}
	}

	dev.Close()
	fmt.Println("Cleaned up.")
	os.Exit(0)
}

func readDisplayAccel() (r accelerometer, err error) {
	return readAccel("/sys/bus/iio/devices/iio:device0")
}

func readBaseAccel() (r accelerometer, err error) {
	return readAccel("/sys/bus/iio/devices/iio:device1")
}

func readAccel(iioPath string) (r accelerometer, err error) {
	x, err := readValue(iioPath + "/in_accel_x_raw")
	if err != nil {
		return
	}

	y, err := readValue(iioPath + "/in_accel_y_raw")
	if err != nil {
		return
	}

	z, err := readValue(iioPath + "/in_accel_z_raw")
	if err != nil {
		return
	}

	return accelerometer{x, y, z}, nil
}

func readValue(path string) (v float64, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}

	i, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return
	}

	v = float64(i)
	return
}

func hingeAxleAngle(disp accelerometer) float64 {
	return math.Atan2(math.Sqrt(disp.z*disp.z+disp.x*disp.x), disp.y) * 180 / math.Pi
}

func hingeAngle(disp, base accelerometer, lidClosed bool) float64 {
	angle := (math.Atan2(disp.z, disp.x) - math.Atan2(base.z, base.x)) * 180 / math.Pi
	if angle < -180 {
		angle += 360
	}
	if angle > 180 {
		angle -= 360
	}
	angle += 180
	// Handle the corner case: tablet is closed or bended by 360 deg.
	// The calculated hinge angle may be wrapped around 0/360 deg.
	// Here, check the lid state and normalize
	if lidClosed {
		angle = 0
	} else if angle < 5 {
		angle = 360
	}
	return angle
}

func lidClosed() bool {
	conn, err := dbus.SystemBus()
	if err != nil {
		return false
	}

	v, err := conn.Object("org.freedesktop.login1", "/org/freedesktop/login1").
		GetProperty("org.freedesktop.login1.Manager.LidClosed")
	if err != nil {
		return false
	}

	closed, ok := v.Value().(bool)
	return ok && closed
}

func tabletMode(hingeAngle float64) int {
	// If hinge axle is vertical
	if hingeAngle < 0 {
		return unknown
	} else if hingeAngle < 190 {
		return laptop
	} else {
		return tablet
	}
}

func writeSwTabletMode(dev *evdev.InputDevice, on bool) error {
	evTime := syscall.NsecToTimeval(int64(time.Now().Nanosecond()))
	var evValue int32
	if on {
		evValue = 1
	} else {
		evValue = 0
	}

	err := dev.WriteOne(&evdev.InputEvent{
		Time:  evTime,
		Type:  evdev.EV_SW,
		Code:  evdev.SW_TABLET_MODE,
		Value: evValue,
	})
	if err != nil {
		return err
	}

	err = dev.WriteOne(&evdev.InputEvent{
		Time:  evTime,
		Type:  evdev.EV_SYN,
		Code:  evdev.SYN_REPORT,
		Value: 0,
	})
	return err
}
