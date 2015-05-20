package devm

import (
	"fmt"
)

type Device struct {
	Name    string
	Owner   string
	Device  string
	MAC     string
	Enabled bool
}

func (d *Device) String() string {
	return fmt.Sprintf("OWNER: %s DEVICE: %s (%s)", d.Owner, d.Device, d.MAC)
}

type sortableKey struct {
	Name    string
	MAC     string
	Enabled bool
}

type ByName []sortableKey

func (a ByName) Len() int      { return len(a) }
func (a ByName) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByName) Less(i, j int) bool {
	if a[i].Enabled && !a[j].Enabled {
		return true
	}
	if !a[i].Enabled && a[j].Enabled {
		return false
	}
	return a[i].Name < a[j].Name
}
