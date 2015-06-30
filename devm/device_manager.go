package devm

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/go-fsnotify/fsnotify"
)

type DeviceManager struct {
	devices     map[string]*Device
	keys        []sortableKey
	configFile  string
	fileHead    string
	restartChan chan bool
	stopChan    chan bool
	watcher     *fsnotify.Watcher
	ignoreWrite bool
	sync.RWMutex
}

func NewDeviceManager(configFile string) *DeviceManager {
	return &DeviceManager{
		devices:     make(map[string]*Device),
		keys:        make([]sortableKey, 0),
		configFile:  configFile,
		restartChan: make(chan bool, 256),
		stopChan:    make(chan bool),
	}
}

func (m *DeviceManager) Load() error {
	// Dump device map
	m.devices = make(map[string]*Device)
	m.keys = make([]sortableKey, 0)
	m.fileHead = ""

	m.Lock()
	defer m.Unlock()
	file, err := os.Open(m.configFile)
	defer file.Close()
	if err != nil {
		return err
	}
	lineNumber := 0
	readingHead := true
	reader := bufio.NewReader(file)
	for line, err := reader.ReadString('\n'); err == nil; line, err = reader.ReadString('\n') {
		lineNumber++
		trimmedLine := strings.TrimSpace(line)
		if len(trimmedLine) < 4 {
			if readingHead {
				m.fileHead += line
			}
			continue
		}
		if trimmedLine[:4] != "host" && trimmedLine[:1] != "#" {
			if readingHead {
				m.fileHead += line
			}
			continue
		}
		readingHead = false
		name := new(string)
		MACString := new(string)
		enabled := true

		// Scan enabled device
		if trimmedLine[:4] == "host" {
			_, err := fmt.Sscanf(trimmedLine, "host %s { hardware ethernet %s }", name, MACString)
			if err != nil {
				log.Println("Failed to parse record on line ", lineNumber)
				continue
			}
		} else {
			_, err := fmt.Sscanf(trimmedLine, "# host %s { hardware ethernet %s }", name, MACString)
			if err != nil {
				log.Println("Failed to parse disabled record on line ", lineNumber)
				continue
			}
			enabled = false
		}

		*MACString = strings.TrimRight(*MACString, ";")
		_, err = net.ParseMAC(*MACString)
		if err != nil {
			log.Println("Failed to parse MAC address on line ", lineNumber)
			continue
		}

		d := &Device{
			Name:    *name,
			MAC:     *MACString,
			Enabled: enabled,
		}
		// Attempt to parse username from device name
		nameTokens := strings.SplitN(*name, "-", 2)
		if len(nameTokens) > 1 {
			d.Owner = nameTokens[0]
			d.Device = nameTokens[1]
		} else {
			d.Owner = "UNKNOWN"
			d.Device = nameTokens[0]
		}

		m.Add(d)
	}
	return nil
}

func (dm *DeviceManager) Start(dhcpdRestart string) {
	// Restart device manager when necessary
	go func() {
		cmdPieces := strings.Split(dhcpdRestart, " ")
		for {
			// Prevent the DHCP service from restarting more than
			// once per minute.
			time.Sleep(time.Minute)
			select {
			case _ = <-dm.restartChan:
				log.Println("Restarting DHCP service.")
				// Don't let the file change while dhcpd is reloading
				dm.Lock()
				rp := exec.Command(cmdPieces[0], cmdPieces[1:]...)
				err := rp.Run()
				if err != nil {
					log.Println(err)
				}
				// Clear the restartChannel
				dm.restartChan = make(chan bool, 256)

				dm.Unlock()
			case _ = <-dm.stopChan:
				log.Println("Stopping device manager.")
				return
			}
		}
	}()

	// Listen for changes in the config file
	go func() {
		var err error
		dm.watcher, err = fsnotify.NewWatcher()
		if err != nil {
			log.Fatal("Could not create fswatcher: ", err)
		}
		err = dm.watcher.Add(dm.configFile)
		if err != nil {
			log.Fatal("Could not start watching config file: ", err)
		}
		for {
			select {
			case event := <-dm.watcher.Events:
				log.Println("Watcher event: " + event.String())
				// File is removed on edit
				if event.Op == fsnotify.Remove {
					if !dm.ignoreWrite {
						log.Println("Detected config file edit (replaced)")
					}
					time.Sleep(time.Second)
					dm.Load()
					dm.watcher.Add(dm.configFile)
				}

				if event.Op == fsnotify.Write {
					if !dm.ignoreWrite {
						log.Println("Detected config file edit.")
						dm.Load()
					}
				}
			case err := <-dm.watcher.Errors:
				log.Println("Watcher error: ", err)
			case _ = <-dm.stopChan:
				return
			}
		}
	}()
}

func (dm *DeviceManager) Stop() {
	dm.stopChan <- true
	dm.watcher.Close()
}

func (dm *DeviceManager) Save() {
	dm.Lock()
	defer dm.Unlock()
	dm.ignoreWrite = true
	go func() { time.Sleep(time.Second); dm.ignoreWrite = false }()
	log.Println("Saving device manager, writing config file.")
	err := ioutil.WriteFile(dm.configFile, []byte(dm.String()), 0660)
	if err != nil {
		log.Println(err)
	}
	dm.restartChan <- true
}

func (dm *DeviceManager) Get(mac string) *Device {
	return dm.devices[mac]
}

func (dm *DeviceManager) Set(d *Device) {
	if _, e := dm.devices[d.MAC]; !e {
		return
	}
	dm.Remove(d.MAC)
	dm.Add(d)
}

func (dm *DeviceManager) Add(d *Device) {
	// Do nothing if the device already exists.
	if _, e := dm.devices[d.MAC]; e {
		return
	}

	k := sortableKey{
		Name:    d.Name,
		MAC:     d.MAC,
		Enabled: d.Enabled,
	}
	dm.keys = append(dm.keys, k)
	dm.devices[d.MAC] = d
	sort.Sort(ByName(dm.keys))
}

func (dm *DeviceManager) Remove(mac string) {
	delete(dm.devices, mac)
	for i, k := range dm.keys {
		if k.MAC == mac {
			dm.keys = append(dm.keys[:i], dm.keys[i+1:]...)
			break
		}
	}
}

func (dm *DeviceManager) ListForUser(owner string) []*Device {
	result := make([]*Device, 0)
	for _, k := range dm.keys {
		d := dm.devices[k.MAC]
		if d.Owner == owner {
			result = append(result, d)
		}
	}
	return result
}

func (dm *DeviceManager) ListAll() []*Device {
	result := make([]*Device, 0, len(dm.devices))
	for _, k := range dm.keys {
		d := dm.devices[k.MAC]
		result = append(result, d)
	}
	return result
}

func (dm *DeviceManager) Contains(mac string) bool {
	if _, exists := dm.devices[mac]; exists {
		return true
	}
	return false
}

func (dm *DeviceManager) NumDevices() int {
	return len(dm.devices)
}

func (dm *DeviceManager) String() string {
	result := ""
	// Write preamble
	result += dm.fileHead

	for _, k := range dm.keys {
		dev := dm.devices[k.MAC]
		// TODO
		// FIXME
		if dev == nil {
			log.Fatal("Found the problem")
		}
		if dev.Enabled {
			result += fmt.Sprintf("   host %s-%s { hardware ethernet %s; }\n", dev.Owner, dev.Device, dev.MAC)
		} else {
			result += fmt.Sprintf("#  host %s-%s { hardware ethernet %s; }\n", dev.Owner, dev.Device, dev.MAC)
		}
	}

	// Write tail
	result += "}\n"
	return result
}
