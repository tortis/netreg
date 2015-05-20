package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"

	"github.com/gorilla/mux"
	"github.com/mavricknz/ldap"
	"github.com/tortis/netreg/devm"
	"github.com/tortis/netreg/token"
)

const LDAP_PATH = "uid=%s,ou=people,dc=math,dc=nor,dc=ou,dc=edu"

var ldapConn *ldap.LDAPConnection
var deviceManager *devm.DeviceManager
var key []byte = []byte("fj2389ruhj8hfj2039d8j23")

func main() {
	// Start the config file manager (device manager)
	deviceManager = devm.NewDeviceManager("test.config")
	err := deviceManager.Load()
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Loaded ", deviceManager.NumDevices(), " devices.")

	// Start the ldap connection
	ldapConn = ldap.NewLDAPConnection("origin.math.nor.ou.edu", 389)
	err = ldapConn.Connect()
	if err != nil {
		log.Fatal(err)
	}
	defer ldapConn.Close()

	// Create the routing mux
	router := mux.NewRouter()
	router.HandleFunc("/login", loginHandler).Methods("POST")
	router.HandleFunc("/devices", listDevices).Methods("GET")
	router.HandleFunc("/devices/{did}", removeDevice).Methods("DELETE")
	router.HandleFunc("/devices", addDevice).Methods("POST")
	router.HandleFunc("/devices/{did}", updateDevice).Methods("PUT")
	router.PathPrefix("/").Handler(http.FileServer(http.Dir("./public")))

	log.Fatal(http.ListenAndServe(":3000", router))
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	// Get the username and password.
	username := r.FormValue("un")
	password := r.FormValue("pw")

	// Attempt LDAP bind
	ldapUser := fmt.Sprintf(LDAP_PATH, username)
	err := ldapConn.Bind(ldapUser, password)
	if err != nil {
		log.Println("User failed to authenticate")
		http.Error(w, "Incorrect username or password", http.StatusBadRequest)
		return
	}

	// Create JWT
	t := token.NewToken(token.EXP_6HOUR)
	t.Contents["username"] = username
	res, err := t.Sign(key)
	if err != nil {
		log.Println("Failed to generate token for user.")
		http.Error(w, "Could not generate token", http.StatusInternalServerError)
		return
	}
	w.Write(res)
}

func listDevices(w http.ResponseWriter, r *http.Request) {
	// Extract and validate JWT
	t := validateToken(w, r)
	if t == nil {
		return
	}

	// Loop up devices using the device manager
	devices := deviceManager.ListForUser(t.Contents["username"])

	// Encode as json and write
	encoder := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json")
	err := encoder.Encode(devices)
	if err != nil {
		http.Error(w, "Server failed to generate response", http.StatusInternalServerError)
		return
	}
}

func removeDevice(w http.ResponseWriter, r *http.Request) {
	// Extract and validate JWT
	t := validateToken(w, r)
	if t == nil {
		return
	}

	// Get device from url
	mac := mux.Vars(r)["did"]

	// See if the device exists
	if deviceManager.Contains(mac) {
		dev := deviceManager.Get(mac)
		// Check if caller is owner
		if t.Contents["username"] != dev.Owner {
			http.Error(w, "No such device exists.", http.StatusBadRequest)
			return
		}
	} else {
		http.Error(w, "No such device exists.", http.StatusBadRequest)
		return
	}

	// Remove device using device manager
	deviceManager.Remove(mac)
	deviceManager.Save()
	fmt.Fprint(w, "Device removed successfully.")
}

func addDevice(w http.ResponseWriter, r *http.Request) {
	// Extract and validate JWT
	t := validateToken(w, r)
	if t == nil {
		return
	}

	// Parse device from request body
	newDevice := new(devm.Device)
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(newDevice)
	if err != nil {
		http.Error(w, "Unable to parse request.", http.StatusBadRequest)
		return
	}

	// Validate the new device
	mac, err := net.ParseMAC(newDevice.MAC)
	if err != nil {
		http.Error(w, "Could not parse MAC address.", http.StatusBadRequest)
		return
	}
	newDevice.MAC = mac.String()
	re := regexp.MustCompile("[^0-9a-zA-Z\\-]")
	newDevice.Device = re.ReplaceAllString(newDevice.Device, "")
	newDevice.Owner = t.Contents["username"]
	newDevice.Name = newDevice.Owner + "-" + newDevice.Device
	newDevice.Enabled = true

	// Check if the device already exists
	if deviceManager.Contains(newDevice.MAC) {
		http.Error(w, "This MAC address is already registered.", http.StatusBadRequest)
		return
	}

	// Add the device to the device manager
	deviceManager.Add(newDevice)
	deviceManager.Save()

	// Encode as json and write
	encoder := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json")
	err = encoder.Encode(newDevice)
	if err != nil {
		http.Error(w, "Server failed to generate response", http.StatusInternalServerError)
		return
	}
}

func updateDevice(w http.ResponseWriter, r *http.Request) {
	// Extract and validate JWT
	t := validateToken(w, r)
	if t == nil {
		return
	}

	// Get old MAC from url
	oldMAC := mux.Vars(r)["did"]

	// Parse device from request body
	changedDevice := new(devm.Device)
	decoder := json.NewDecoder(r.Body)
	err := decoder.Decode(changedDevice)
	if err != nil {
		http.Error(w, "Unable to parse request.", http.StatusBadRequest)
		return
	}

	// Validate the new device
	mac, err := net.ParseMAC(changedDevice.MAC)
	if err != nil {
		http.Error(w, "Could not parse MAC address.", http.StatusBadRequest)
		return
	}
	changedDevice.MAC = mac.String()
	re := regexp.MustCompile("[^0-9a-zA-Z\\-]")
	changedDevice.Device = re.ReplaceAllString(changedDevice.Device, "")
	changedDevice.Owner = t.Contents["username"]
	changedDevice.Name = changedDevice.Owner + "-" + changedDevice.Device

	// Update in device manager
	if oldMAC == changedDevice.MAC {
		if deviceManager.Contains(oldMAC) {
			deviceManager.Add(changedDevice)
		} else {
			http.Error(w, "Device does not exist.", http.StatusBadRequest)
			return
		}
	} else {
		if deviceManager.Contains(changedDevice.MAC) {
			http.Error(w, "This MAC address is already registered.", http.StatusBadRequest)
			return
		}
		deviceManager.Remove(oldMAC)
		deviceManager.Add(changedDevice)
	}
	log.Println("Saving device manager")
	deviceManager.Save()
	log.Println("Finished save")

	// Encode as json and write
	encoder := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json")
	err = encoder.Encode(changedDevice)
	if err != nil {
		log.Println(err)
		http.Error(w, "Server failed to generate response", http.StatusInternalServerError)
		return
	}
}

func validateToken(w http.ResponseWriter, r *http.Request) *token.Token {
	tokenString := r.Header.Get("Authorization")
	t, err := token.Validate([]byte(tokenString), key)
	if err != nil {
		if err == token.ERR_EXPIRED {
			http.Error(w, "Token is expired", http.StatusBadRequest)
			return nil
		} else if err == token.ERR_MALFORMED_TOKEN {
			http.Error(w, "Invalid token", http.StatusBadRequest)
			return nil
		} else if err == token.ERR_INVALID_SIG {
			http.Error(w, "Invalid token", http.StatusBadRequest)
			return nil
		} else {
			http.Error(w, "Server failed to process token.", http.StatusInternalServerError)
			return nil
		}
	}
	return t
}
