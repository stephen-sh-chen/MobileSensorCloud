package main

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/oschwald/geoip2-golang"
	//"github.com/pubnub/go/messaging"
	"html/template"
	"io"
	"net"
	"net/http"
	"strconv"
)

func home(w http.ResponseWriter, r *http.Request) {

	var templates = template.Must(template.New("locateip").ParseFiles("sensorAdd.html"))

	err := templates.ExecuteTemplate(w, "sensorAdd.html", nil)

	if err != nil {
		panic(err)
	}

}

func test(w http.ResponseWriter, r *http.Request) {

	var templates = template.Must(template.New("locateip").ParseFiles("index.html"))

	err := templates.ExecuteTemplate(w, "index.html", nil)

	if err != nil {
		panic(err)
	}

}

func testGmap(w http.ResponseWriter, r *http.Request) {

	var templates = template.Must(template.New("locateip").ParseFiles("gmapExample.html"))

	err := templates.ExecuteTemplate(w, "gmapExample.html", nil)

	if err != nil {
		panic(err)
	}

}

func returnLatLong(w http.ResponseWriter, r *http.Request) {

	// not recommended to open the file each time per request
	// we put it here for tutorial sake.

	db, err := geoip2.Open("GeoLite2-City.mmdb")

	if err != nil {
		fmt.Println(err)
	}

	defer db.Close()

	if r.Method == "POST" {
		ipAddress := r.FormValue("ajax_post_data")

		// If you are using strings that may be invalid, check that IP is not nil
		// and a valid IP address -- see https://www.socketloop.com/tutorials/golang-validate-ip-address

		if ipAddress != "" {
			ip := net.ParseIP(ipAddress)
			record, err := db.City(ip)
			if err != nil {
				fmt.Println(err)
			}

			fmt.Printf("Country name in English: %v\n", record.Country.Names["en"])
			fmt.Printf("Coordinates: Lat(%v), Long(%v)\n", record.Location.Latitude, record.Location.Longitude)

			w.Write([]byte(fmt.Sprintf("{\"Country\":\"%v\",\"Lat\": \"%v\",\"Long\":\"%v\"}", record.Country.Names["en"], record.Location.Latitude, record.Location.Longitude)))

		}
	}
}

type SensorProfile struct {
	ID            string `json:"SensorID"`
	Name          string `json:"SensorName"`
	Type          int
	State         int
	Value         float64
	Unit          string
	HostVehicleID string
	Group         int
	Lon           float64
	Lat           float64
}

type AddSensorReq struct {
	SensorName    string
	SensorType    int
	HostVehicleID string
}

type AddSensorRes struct {
	SensorID      string
	SensorName    string
	SensorType    int
	SensorState   int
	HostVehicleID string
}

// Global variables
var sensorMap = make(map[string]SensorProfile)
var trafficOn int = 0

// newUUID generates a random UUID according to RFC 4122
func newUUID() (string, error) {
	uuid := make([]byte, 16)
	n, err := io.ReadFull(rand.Reader, uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}
	// variant bits; see section 4.1.1
	uuid[8] = uuid[8]&^0xc0 | 0x80
	// version 4 (pseudo-random); see section 4.1.3
	uuid[6] = uuid[6]&^0xf0 | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

func publishSensorInfo() {
	// Block of PubNub message channel
	/*	my_pubkey := "pub-c-bcc7ac96-ccbe-4577-bd6f-66321585d73a"
		my_subkey := "sub-c-6d08ffd2-a589-11e6-80e1-0619f8945a4f"

		pubnub := messaging.NewPubnub(my_pubkey, my_subkey, "", "", false, "")
		fmt.Println("PubNub SDK for go;", messaging.VersionInfo())
		successChannel := make(chan []byte)
		errorChannel := make(chan []byte)
	*/
	for {
		if trafficOn > 0 {
			for _, value := range sensorMap {
				j, _ := json.Marshal(value)
				fmt.Println(string(j))
			}
		}
	}
	/*go pubnub.Publish("my_channel", "Hello from the PubNub Go SDK!", successChannel, errorChannel)

	select {
	case response := <-successChannel:
		fmt.Println(string(response))
	case err := <-errorChannel:
		fmt.Println(string(err))
	case <-messaging.Timeout():
		fmt.Println("Publish() timeout")
	}*/
}

func trafficOnHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("Traffic ON = " + strconv.Itoa(trafficOn))
	trafficOn = 1
	fmt.Println("Traffic ON = " + strconv.Itoa(trafficOn))
}

func trafficOffHandler(w http.ResponseWriter, r *http.Request) {

	fmt.Println("Traffic ON = " + strconv.Itoa(trafficOn))
	trafficOn = 0
	fmt.Println("Traffic ON = " + strconv.Itoa(trafficOn))
}

func addSensorHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("hello")

	if r.Method == "POST" {
		if r.Body == nil {
			http.Error(w, "Please send a request body", 400)
			return
		}

		var sReq AddSensorReq
		err := json.NewDecoder(r.Body).Decode(&sReq)
		if err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		var sNew SensorProfile
		sNew.ID, err = newUUID()
		sNew.Name = sReq.SensorName
		sNew.Type = sReq.SensorType
		sNew.State = 1
		sNew.Value = 0
		sNew.Unit = "db"
		sNew.HostVehicleID = sReq.HostVehicleID
		sNew.Group = 0
		//fmt.Println("Sesnot ID = " + sNew.ID)

		sensorMap[sNew.ID] = sNew // add sensor in the management map

		var sRes AddSensorRes
		sRes.SensorID = sensorMap[sNew.ID].ID
		sRes.SensorName = sensorMap[sNew.ID].Name
		sRes.SensorType = sNew.Type
		sRes.SensorState = sNew.State
		sRes.HostVehicleID = sNew.HostVehicleID
		json.NewEncoder(w).Encode(sRes)
	}
}

func main() {

	mux := mux.NewRouter()

	mux.HandleFunc("/", home)
	mux.HandleFunc("/returncoord", returnLatLong)
	mux.HandleFunc("/test/", test)
	mux.HandleFunc("/test/gmap/", testGmap)

	// Block of my sensor API
	mux.HandleFunc("/api/addSensor", addSensorHandler)
	mux.HandleFunc("/api/traffic/on", trafficOnHandler)
	mux.HandleFunc("/api/traffic/off", trafficOffHandler)

	// Enable to publish sensor info
	trafficOn = 1
	go publishSensorInfo()

	// Start to port 3000 for REST service
	http.ListenAndServe(":3000", mux)
}
