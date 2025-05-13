# tablet-mode-accel

Many x86 convertibles lack a functional tablet mode switch when running Linux. Some models, however, have accelerometers built into their base and display halves. These accelerometers can be used to infer when the device has entered tablet mode.

This Go program reads those accelerometers and creates a virtual input device that emits SW_TABLET_MODE events. On GNOME, this event enables the screen rotation feature and disables the keyboard and trackpad. The tablet detection algorithm is based off of Yauhen Kharuzhy's yet-to-be-merged [merge request](https://gitlab.freedesktop.org/hadess/iio-sensor-proxy/-/merge_requests/338) to iio-sensor-proxy. Unlike petitstrawberry's [minibook-support](https://github.com/petitstrawberry/minibook-support) daemon, this program does not attempt to suppress keyboard and trackpad events by wrapping them in virtual input devices. On Wayland, libinput automatically suppresses these inputs after receiving a SW_TABLET_MODE event.

This program has been tested specifically on a Piccolo Series 81x N150 minibook, but it should work with any convertible equipped with dual accelerometers.
