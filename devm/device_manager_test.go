package devm

import (
	"fmt"
	"io/ioutil"
	"os"
	"testing"
)

const SAMPLE_CONF = `DDNS-update-style ad-hoc;

subnet 129.15.11.0 netmask 255.255.255.0
{
   authoritative;
   option routers 129.15.11.1;
   option domain-name-servers 129.15.1.120, 129.15.1.121, 129.15.1.9;
   option domain-name "math.ou.edu";
   option broadcast-address 129.15.11.255;
   pool
   {
       range 129.15.11.130 129.15.11.249;
       deny unknown clients;
       default-lease-time 3600;
       max-lease-time 3800;
   }

   host ykim-laptop { hardware ethernet e0:ca:94:d4:4c:9f; }
   host ykim-phone { hardware ethernet 1c:99:4c:b5:af:9b; }
   host yli-eth { hardware ethernet 00:14:22:A6:22:44; }
   host yli-wi { hardware ethernet 00:14:A5:89:AC:63; }
   host zhu-spectre { hardware ethernet 68:94:23:11:56:53; }
#  host UNKNOWN-kbroku { hardware ethernet b0:a7:37:71:ce:38; }
#  host dfindley-laptop { hardware ethernet 10:68:3f:fd:e9:1d; }
#  host kblee-roku3wifi { hardware ethernet B0:A7:37:96:CD:8F; }
}`

func TestLoad(t *testing.T) {
	// Create a known file
	ioutil.WriteFile("TestLoad.conf", []byte(SAMPLE_CONF), 0664)

	// Create a device manager and load the file
	dm := NewDeviceManager("TestLoad.conf")
	err := dm.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Verify size
	if dm.NumDevices() != 8 {
		t.Fatal("File contained 8 devices, but dm loaded ", dm.NumDevices())
	}

	if dm.NumDevices() != len(dm.keys) {
		t.Fatal("Mismatched keys and device records.")
	}

	// Verify 3 of them are disabled
	disabled := 0
	for _, dev := range dm.devices {
		if dev.Enabled == false {
			disabled++
		}
	}
	if disabled != 3 {
		t.Fatal("File contained 3 disabled devices, but dm indicates ", disabled)
	}

	os.Remove("TestLoad.conf")
}

func TestSave(t *testing.T) {
	// Create a known test file
	ioutil.WriteFile("TestSave.conf", []byte(SAMPLE_CONF), 0664)

	// Create a device manager and load the file
	dm := NewDeviceManager("TestSave.conf")
	err := dm.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Change the confg file name save the config file
	dm.configFile = "TestSave2.conf"
	dm.Save()

	// Create a second device manager and load the new file
	dm2 := NewDeviceManager("TestSave2.conf")
	err = dm2.Load()
	if err != nil {
		t.Fatal(err)
	}

	// Compare the devices loaded by each manager
	for mac, dev := range dm.devices {
		if !dm2.Contains(mac) {
			t.Fatal("A device in the origional file did not survive save and reload.")
		}
		dev2 := dm2.devices[mac]
		if *dev != *dev2 {
			fmt.Printf("%v  !=  %v\n", *dev, *dev2)
			t.Fatal("A saved device (", mac, ") does not match the origional.")
		}
	}

	os.Remove("TestSave.conf")
	os.Remove("TestSave2.conf")
}

func TestAdd(t *testing.T) {
	// Create a known test file
	ioutil.WriteFile("TestAdd.conf", []byte(SAMPLE_CONF), 0664)

	// Create a device manager and load the file
	dm := NewDeviceManager("TestAdd.conf")
	err := dm.Load()
	if err != nil {
		t.Fatal(err)
	}

	preSize := dm.NumDevices()
	newDev := &Device{
		Name:    "dfindley-iPhone",
		Owner:   "dfindley",
		Device:  "iPhone",
		MAC:     "00:00:00:00:00:00",
		Enabled: true,
	}

	dm.Add(newDev)

	if dm.NumDevices() != len(dm.keys) {
		t.Fatal("Mismatched keys and device records.")
	}

	if dm.NumDevices() != preSize+1 {
		t.Fatal("Adding a device did not increase NumDevices by 1.")
	}

	if !dm.Contains(newDev.MAC) {
		t.Fatal("Device manager does not contain the newly added device.")
	}

	if *dm.Get(newDev.MAC) != *newDev {
		fmt.Printf("%v  !=  %v\n", *newDev, *dm.Get(newDev.MAC))
		t.Fatal("The new device is not the same as the one added to the manager.")
	}

	os.Remove("TestAdd.conf")
}

func TestRemove(t *testing.T) {
	// Create a known test file
	ioutil.WriteFile("TestRemove.conf", []byte(SAMPLE_CONF), 0664)

	// Create a device manager and load the file
	dm := NewDeviceManager("TestRemove.conf")
	err := dm.Load()
	if err != nil {
		t.Fatal(err)
	}

	preSize := dm.NumDevices()

	dm.Remove("00:14:A5:89:AC:63")

	if dm.NumDevices() != len(dm.keys) {
		t.Fatal("Mismatched keys and device records.")
	}

	if dm.NumDevices() != preSize-1 {
		t.Fatal("Adding a device did not increase NumDevices by 1.")
	}

	if dm.Contains("00:14:A5:89:AC:63") {
		t.Fatal("The removed devices is still in the manager.")
	}

	os.Remove("TestRemove.conf")
}

func TestListForUser(t *testing.T) {
	// Create a known test file

	// Create a device manager and load the file

	// Call for a users devices.

	// Ensure the correct number of devices is given

	// Ensure caller really owns all the returned devices.
}
