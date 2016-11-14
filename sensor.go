package main

import (
	crypto_rand "crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/oschwald/geoip2-golang"
	"github.com/pubnub/go/messaging"
	"html/template"
	"io"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"time"
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
	Lont          float64
	Lat           float64
}

type SensorSignal struct {
	SignalID  string
	SensorID  string
	TimeStamp string
	Value     float64
	Unit      string
	Lont      float64
	Lat       float64
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
var my_pubkey = "pub-c-bcc7ac96-ccbe-4577-bd6f-66321585d73a"
var my_subkey = "sub-c-6d08ffd2-a589-11e6-80e1-0619f8945a4f"
var my_channel = "my_channel"

// newUUID generates a random UUID according to RFC 4122
func newUUID() (string, error) {
	uuid := make([]byte, 16)
	n, err := io.ReadFull(crypto_rand.Reader, uuid)
	if n != len(uuid) || err != nil {
		return "", err
	}
	// variant bits; see section 4.1.1
	uuid[8] = uuid[8]&^0xc0 | 0x80
	// version 4 (pseudo-random); see section 4.1.3
	uuid[6] = uuid[6]&^0xf0 | 0x40
	return fmt.Sprintf("%x-%x-%x-%x-%x", uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:]), nil
}

func float_rand(start_num float64, end_num float64) float64 {
	rand.Seed(time.Now().UnixNano())
	return rand.Float64()*(end_num-start_num) + start_num
}

func noiseGen() float64 {
	return float_rand(40, 130) // the unit is dB: decibel
}

func airPollutionGen() float64 {
	return float_rand(1, 10) // the unit is AQI: Air Quaility Index
}

func signalHelper(sensor *SensorProfile, signal *SensorSignal) {
	signal.SignalID, _ = newUUID()
	signal.SensorID = sensor.ID
	signal.TimeStamp = strconv.FormatInt(time.Now().UnixNano(), 10)

	// sensor type is noise
	if sensor.Type == 1 {
		signal.Value = noiseGen()
		signal.Unit = "dB"
	} else if sensor.Type == 2 {
		signal.Value = airPollutionGen()
		signal.Unit = "AQI"
	}
	signal.Lont = 30.4524
	signal.Lat = -123.56
}

func publishSensorInfo() {
	pubnub := messaging.NewPubnub(my_pubkey, my_subkey, "", "", false, "")
	fmt.Println("PubNub SDK for go;", messaging.VersionInfo())
	successChannel := make(chan []byte)
	errorChannel := make(chan []byte)

	for {
		if trafficOn > 0 {
			for _, sensor := range sensorMap {
				time.Sleep(1000 * time.Millisecond)
				//j, _ := json.Marshal(value)
				//fmt.Println(string(j))
				if sensor.State == 1 {
					var signal SensorSignal
					signalHelper(&sensor, &signal)

					j, _ := json.Marshal(signal)
					go pubnub.Publish(my_channel, string(j), successChannel, errorChannel)

					select {
					case response := <-successChannel:
						fmt.Println(string(response))
						fmt.Println("Sent Message " + string(j))
					case err := <-errorChannel:
						fmt.Println(string(err))
					case <-messaging.Timeout():
						fmt.Println("Publish() timeout")
					}
				}
			}
		}
	}
}

func subscribeSensorInfo() {
	pubnub := messaging.NewPubnub(my_pubkey, my_subkey, "", "", false, "")
	successChannel := make(chan []byte)
	errorChannel := make(chan []byte)

	go pubnub.Subscribe(my_channel, "", successChannel, false, errorChannel)

	go func() {
		for {
			select {
			case response := <-successChannel:
				var msg []interface{}

				err := json.Unmarshal(response, &msg)
				if err != nil {
					fmt.Println(err)
					return
				}
				fmt.Println("got msg!") //Test
				switch m := msg[0].(type) {
				case float64:
					fmt.Println(msg[1].(string))
				case []interface{}:
					fmt.Printf("Received message '%s' on channel '%s'\n", m[0], msg[2])
					//return
				default:
					panic(fmt.Sprintf("Unknown type: %T", m))
				}

			case err := <-errorChannel:
				fmt.Println(string(err))
			case <-messaging.SubscribeTimeout():
				fmt.Println("Subscribe() timeout")
			}
		}
	}()
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
	fmt.Println("Enter addSensorHandler!")

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
		sNew.Type = sReq.SensorType // 1: noise sensor; 2: air sensor
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

func showSensorListHandler(w http.ResponseWriter, r *http.Request) {

	fmt.Println("Enter showSensorListHandler!")

	if r.Method == "GET" {
		if r.Body == nil {
			http.Error(w, "Please send a request body", 400)
			return
		}

		json.NewEncoder(w).Encode(sensorMap)
	}

}

func main() {

	mux := mux.NewRouter()

	mux.HandleFunc("/", home) // sensorAdd.html
	mux.HandleFunc("/returncoord", returnLatLong)
	mux.HandleFunc("/test", test)          // index.html
	mux.HandleFunc("/test/gmap", testGmap) // gmapExample.html

	// Block of my sensor API
	mux.HandleFunc("/api/addSensor", addSensorHandler)             // POST method
	mux.HandleFunc("/api/show-sensor-list", showSensorListHandler) // Get  method
	mux.HandleFunc("/api/traffic/on", trafficOnHandler)            // Get  method
	mux.HandleFunc("/api/traffic/off", trafficOffHandler)          // Get  method

	// Enable to publish sensor info
	trafficOn = 1
	subscribeSensorInfo()
	go publishSensorInfo()

	// Start to port 3000 for REST service
	http.ListenAndServe(":3000", mux)
}
