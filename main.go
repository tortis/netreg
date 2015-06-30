package main

import (
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"regexp"

	"github.com/go-ldap/ldap"
	"github.com/gorilla/mux"

	"github.com/tortis/netreg/devm"
	"github.com/tortis/netreg/token"
)

var ldapSearchPath string
var webPort string
var ldapServer string
var ldapPort int
var dhcpdConfigFile string
var dhcpdRestartCmd string
var hostHTML bool
var enableCORS bool
var htmlDir string
var adminUser string

var ldapConn *ldap.Conn
var deviceManager *devm.DeviceManager
var key []byte

func init() {
	flag.StringVar(&webPort, "web-port", ":3000", "port that the web server will listen on.")
	flag.StringVar(&ldapServer, "ldap-server", "localhost", "LDAP server to connect to.")
	flag.IntVar(&ldapPort, "ldap-port", 389, "Port to connect to LDAP server on.")
	flag.StringVar(&ldapSearchPath, "ldap-search-path", "uid=%s,ou=people,dc=math,dc=nor,dc=ou,dc=edu", "Format string for ldap bind DN")
	flag.StringVar(&dhcpdConfigFile, "dhcpd-conf-file", "/etc/dhcp/dhcpd.conf", "dhcpd config file to use.")
	flag.StringVar(&dhcpdRestartCmd, "dhcpd-restart", "/sbin/service dhcpd restart", "command to restart the dhcp server.")
	flag.StringVar(&htmlDir, "htmldir", "./public", "Path to the HTML directory")
	flag.StringVar(&adminUser, "adminuser", "dfindley", "Username that will receive admin privs.")
	flag.BoolVar(&hostHTML, "hosthtml", false, "If set, the the server will also host static html from 'htmldir'")
	flag.BoolVar(&enableCORS, "enablecors", true, "If set, the server will send cross-origin headers.")

	// Generate a random token key
	key = make([]byte, 16)
	rand.Read(key)
}

func main() {
	flag.Parse()
	// Start the config file manager (device manager)
	deviceManager = devm.NewDeviceManager(dhcpdConfigFile)
	err := deviceManager.Load()
	if err != nil {
		log.Fatal(err)
	}
	log.Println("Loaded ", deviceManager.NumDevices(), " devices.")
	deviceManager.Start(dhcpdRestartCmd)
	defer deviceManager.Stop()

	// Start the ldap connection
	log.Printf("Attempting to connect to LDAP on: %s:%d\n", ldapServer, ldapPort)
	ldapConn, err = ldap.Dial("tcp", fmt.Sprintf("%s:%d", ldapServer, ldapPort))
	if err != nil {
		log.Fatal("Failed to connect to LDAP server:", err)
	}
	defer ldapConn.Close()
	log.Println("LDAP connection established.")

	// Create the routing mux
	router := mux.NewRouter()
	router.HandleFunc("/login", loginHandler).Methods("POST")
	router.HandleFunc("/devices", listDevices).Methods("GET")
	router.HandleFunc("/devices/{did}", removeDevice).Methods("DELETE")
	router.HandleFunc("/devices", addDevice).Methods("POST")
	router.HandleFunc("/devices/{did}", updateDevice).Methods("PUT")

	// Server HTML
	if hostHTML {
		router.PathPrefix("/").Handler(http.FileServer(http.Dir(htmlDir)))
	}

	// Add middleware for CORS
	if enableCORS {
		http.Handle("/", corsMiddleware(router))
	} else {
		http.Handle("/", router)
	}

	log.Println("Serving requests on ", webPort)
	log.Fatal(http.ListenAndServe(webPort, nil))
}

func corsMiddleware(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Println("[CORS] OPTIONS handler called.")
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "POST, GET, OPTIONS, PUT, DELETE")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	// Get the username and password.
	username := r.FormValue("un")
	password := r.FormValue("pw")

	// Attempt LDAP bind
	ldapUser := fmt.Sprintf(ldapSearchPath, username)
	lde := ldapConn.Bind(ldapUser, password)
	if lde != nil {
		log.Println("User failed to authenticate")
		http.Error(w, "Incorrect username or password", http.StatusBadRequest)
		log.Println("[LOGIN](fail) ", username)
		return
	}

	// Create JWT
	t := token.NewToken(token.EXP_6HOUR)
	t.Contents["username"] = username
	if username == adminUser {
		t.Contents["admin"] = "yes"
	}
	res, err := t.Sign(key)
	if err != nil {
		log.Println("Failed to generate token for user.")
		http.Error(w, "Could not generate token", http.StatusInternalServerError)
		return
	}
	w.Write(res)
	log.Println("[LOGIN](success) ", username)
}

func listDevices(w http.ResponseWriter, r *http.Request) {
	// Extract and validate JWT
	t := validateToken(w, r)
	if t == nil {
		return
	}

	// Loop up devices using the device manager
	var devices []*devm.Device
	if t.Contents["admin"] == "yes" {
		devices = deviceManager.ListAll()
	} else {
		devices = deviceManager.ListForUser(t.Contents["username"])
	}

	// Encode as json and write
	encoder := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json")
	err := encoder.Encode(devices)
	if err != nil {
		http.Error(w, "Server failed to generate response", http.StatusInternalServerError)
		return
	}
	log.Println("[LIST](", len(devices), "devices ) ", t.Contents["username"])
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
		if t.Contents["username"] != dev.Owner && t.Contents["admin"] != "yes" {
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
	log.Println("[REMOVE](", mac, " ) ", t.Contents["username"])
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
	if t.Contents["admin"] != "yes" {
		newDevice.Owner = t.Contents["username"]
	}
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
	log.Println("[ADD](", newDevice.MAC, " ) ", t.Contents["username"])
}

func updateDevice(w http.ResponseWriter, r *http.Request) {
	// Extract and validate JWT
	t := validateToken(w, r)
	if t == nil {
		return
	}

	// Get old MAC from url
	oldMAC := mux.Vars(r)["did"]
	oldDev := deviceManager.Get(oldMAC)
	if oldDev == nil {
		http.Error(w, "Device does not exist.", http.StatusBadRequest)
		return
	}

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
	changedDevice.Owner = oldDev.Owner
	changedDevice.Name = changedDevice.Owner + "-" + changedDevice.Device

	// If the mac has not changed
	if oldMAC == changedDevice.MAC {
		deviceManager.Set(changedDevice)
	} else { // If the mac has changed, create a new device
		if deviceManager.Contains(changedDevice.MAC) {
			http.Error(w, "This MAC address is already registered.", http.StatusBadRequest)
			return
		}
		deviceManager.Remove(oldMAC)
		deviceManager.Add(changedDevice)
	}
	deviceManager.Save()

	// Encode as json and write
	encoder := json.NewEncoder(w)
	w.Header().Set("Content-Type", "application/json")
	err = encoder.Encode(changedDevice)
	if err != nil {
		log.Println(err)
		http.Error(w, "Server failed to generate response", http.StatusInternalServerError)
		return
	}
	log.Println("[UPDATE](", changedDevice.MAC, " ) ", t.Contents["username"])
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
			log.Println(err)
			http.Error(w, "Server failed to process token.", http.StatusInternalServerError)
			return nil
		}
	}
	return t
}
