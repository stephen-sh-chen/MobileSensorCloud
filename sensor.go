package main

import (
	crypto_rand "crypto/rand"
	"encoding/json"
	"fmt"
	"github.com/gorilla/mux"
	"github.com/kr/pretty"
	"github.com/oschwald/geoip2-golang"
	"github.com/pubnub/go/messaging"
	"golang.org/x/net/context"
	"googlemaps.github.io/maps"
	"gopkg.in/mgo.v2"
	"html/template"
	"io"
	"log"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
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
	Long          float64
	Lat           float64
}

/*
type SensorProfile struct {
	ID            string  `json:"sensor_id"`
	Name          string  `json:"sensor_name"`
	Type          int     `json:"sensor_type"`
	State         int     `json:"sensor_status"`
	Value         float64 `json:"sensor_name"`
	Unit          string  `json:"sensor_unit"`
	HostVehicleID string  `json:"sensor_host_vehicle_id"`
	Group         int     `json:"sensor_group"`
	Long          float64 `json:"sensor_longitude"`
	Lat           float64 `json:"sensor_latitude"`
}*/

type SensorSignal struct {
	SignalID  string  `json:"signal_id"`
	SensorID  string  `json:"sensor_id"`
	TimeStamp string  `json:"last_update"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Long      float64 `json:"longitude"`
	Lat       float64 `json:"latitude"`
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

type Coordinate struct {
	longitude float64
	latitude  float64
	altitude  float64
}

// Global variables
var sensorMap = make(map[string]SensorProfile)
var trafficOn int = 0
var my_pubkey = "pub-c-bcc7ac96-ccbe-4577-bd6f-66321585d73a"
var my_subkey = "sub-c-6d08ffd2-a589-11e6-80e1-0619f8945a4f"
var my_channel = "my_channel"
var db_addr = "54.191.90.246:27017"
var xy []Coordinate
var xy_index = 0
var xy_direction = 1

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

	//sensor.Long += float_rand(0.001, 0.002)
	//sensor.Lat += float_rand(0.001, 0.002)
	if xy_index == len(xy)-1 {
		xy_direction = -1
	} else if xy_index == 0 {
		xy_direction = 1
	}
	xy_index += xy_direction

	sensor.Long = xy[xy_index].longitude
	sensor.Lat = xy[xy_index].latitude
	sensorMap[sensor.ID] = *sensor
	signal.Long = sensor.Long
	signal.Lat = sensor.Lat
}

func publishSensorInfo() {
	pubnub := messaging.NewPubnub(my_pubkey, my_subkey, "", "", false, "")
	fmt.Println("PubNub SDK for go;", messaging.VersionInfo())
	successChannel := make(chan []byte)
	errorChannel := make(chan []byte)

	for {
		if trafficOn > 0 {
			for _, sensor := range sensorMap {
				time.Sleep(100 * time.Millisecond)
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

func trafficAllONHandler(w http.ResponseWriter, r *http.Request) {
	fmt.Println("All Traffic ON = " + strconv.Itoa(trafficOn))
	trafficOn = 1
	fmt.Println("All Traffic ON = " + strconv.Itoa(trafficOn))
}

func trafficSensorONHandler(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query()["sensorid"][0]
	if sensorID == "" {
		var str string = "Please give a sensor ID"
		http.Error(w, str, 400)
		return
	}

	fmt.Println("Query Sensor ID = ", sensorID)
	sensor, ok := sensorMap[sensorID]
	if !ok {
		var str string = "Not found Sensor ID: " + sensorID
		http.Error(w, str, 400)
		return
	} else {
		sensor.State = 1
		sensorMap[sensorID] = sensor
		fmt.Println("Sensor state is %d on now ID %s", sensorMap[sensorID], sensorID)
		json.NewEncoder(w).Encode(sensor)
	}
}

func trafficAllOFFHandler(w http.ResponseWriter, r *http.Request) {

	fmt.Println("All Traffic OFF = " + strconv.Itoa(trafficOn))
	trafficOn = 0
	fmt.Println("All Traffic OFF = " + strconv.Itoa(trafficOn))
}

func trafficSensorOFFHandler(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query()["sensorid"][0]
	if sensorID == "" {
		var str string = "Please give a sensor ID"
		http.Error(w, str, 400)
		return
	}

	fmt.Println("Query Sensor ID = ", sensorID)
	sensor, ok := sensorMap[sensorID]
	if !ok {
		var str string = "Not found Sensor ID: " + sensorID
		http.Error(w, str, 400)
		return
	} else {
		sensor.State = 0
		sensorMap[sensorID] = sensor
		fmt.Println("Sensor state is %d on now ID %s", sensorMap[sensorID], sensorID)
		json.NewEncoder(w).Encode(sensor)
	}
}

func deleteSensorHandler(w http.ResponseWriter, r *http.Request) {
	sensorID := r.URL.Query()["sensorid"][0]
	if sensorID == "" {
		var str string = "Please give a sensor ID"
		http.Error(w, str, 400)
		return
	}

	fmt.Println("Query Sensor ID = ", sensorID)
	_, ok := sensorMap[sensorID]
	if !ok {
		var str string = "Not found Sensor ID: " + sensorID
		http.Error(w, str, 400)
		return
	} else {
		delete(sensorMap, sensorID)
		fmt.Println("Delete Sensor ID %s", sensorID)
		json.NewEncoder(w).Encode(sensorMap)
	}
}

func showGmap(w http.ResponseWriter, r *http.Request) {
	fmt.Println("enter show google map")
	sensorID := r.URL.Query()["sensorid"][0]
	if sensorID == "" {
		var str string = "Please give a sensor ID"
		http.Error(w, str, 400)
		return
	}

	var templates = template.Must(template.New("locateip").ParseFiles("gmapExample.html"))
	p := SensorProfile{ID: sensorID}
	err := templates.ExecuteTemplate(w, "gmapExample.html", p)

	if err != nil {
		panic(err)
	}

}

func addSensor2DB(sensor SensorProfile) {
	session, err := mgo.Dial(db_addr)
	if err != nil {
		panic(err)
		fmt.Println("cannot connet to the mongo DB!!!!")
	}
	defer session.Close()
	session.SetMode(mgo.Monotonic, true)
	fmt.Println("ready to add sensor to DB!")

	/*var s SensorProfile
	j, _ := json.Marshal(sensor)
	err = json.Unmarshal([]byte(j), &s)
	if err != nil {
		fmt.Println("json decode fail!!!")
		return
	}*/

	c := session.DB("fullstack").C("sensor")
	err = c.Insert(sensor)
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println("Added a new sensor %s to DB", sensor.ID)
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
		if sReq.SensorType == 1 {
			sNew.Unit = "db"
		} else if sReq.SensorType == 2 {
			sNew.Unit = "AQI"
		}
		sNew.HostVehicleID = sReq.HostVehicleID
		sNew.Group = 0
		sNew.Long = xy[xy_index].longitude
		sNew.Lat = xy[xy_index].latitude

		//fmt.Println("Sesnot ID = " + sNew.ID)

		sensorMap[sNew.ID] = sNew // add sensor in the management map
		addSensor2DB(sNew)

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

func gmapHandler() {
	c, err := maps.NewClient(maps.WithAPIKey("AIzaSyDNJXVRtebwIzQQauD8yddaXB8y4wyFqfg"))
	if err != nil {
		log.Fatalf("fatal error: %s", err)
	}
	r := &maps.DirectionsRequest{
		//Origin: "37.32741,-121.81021",
		//Origin:      "2240 E Capitol Expy, San Jose, CA 95122, USA",
		//Destination: "1249 Great Mall Dr, Milpitas, CA 95035, USA",
		Origin:      "37.32242, -121.81496",
		Destination: "37.4133, -121.89826",
	}
	resp, wayPoint, err := c.Directions(context.Background(), r)
	if err != nil {
		log.Fatalf("fatal error: %s", err)
	}

	pretty.Println(resp)
	pretty.Println(wayPoint)
}

func parseCoordinates() {
	for _, str := range strings.Split(cor, " ") {
		s := strings.Split(str, ",")
		var location Coordinate
		location.longitude, _ = strconv.ParseFloat(s[0], 64)
		location.latitude, _ = strconv.ParseFloat(s[1], 64)
		location.altitude, _ = strconv.ParseFloat(s[2], 64)
		xy = append(xy, location)
	}

	//pretty.Println(xy)
}

func main() {

	mux := mux.NewRouter()

	mux.HandleFunc("/returncoord", returnLatLong)
	mux.HandleFunc("/test", test) // index.html

	// Block of my sensor API
	mux.HandleFunc("/sensor-provider/", home)                                      // sensorAdd.html
	mux.HandleFunc("/sensor-provider/api/add-sensor", addSensorHandler)            // POST method
	mux.HandleFunc("/sensor-provider/api/show-sensor-list", showSensorListHandler) // Get  method
	mux.HandleFunc("/sensor-provider/api/traffic/on/", trafficAllONHandler)        // Get  method
	mux.HandleFunc("/sensor-provider/api/traffic/on", trafficSensorONHandler)      // Get  method
	mux.HandleFunc("/sensor-provider/api/traffic/off/", trafficAllOFFHandler)      // Get  method
	mux.HandleFunc("/sensor-provider/api/traffic/off", trafficSensorOFFHandler)    // Get  method
	mux.HandleFunc("/sensor-provider/api/delete-sensor", deleteSensorHandler)      // Get method
	mux.HandleFunc("/sensor-provider/api/gmap", showGmap)                          // gmapExample.html

	// Enable to publish sensor info
	trafficOn = 0
	//subscribeSensorInfo()
	go publishSensorInfo()
	//gmapHandler()
	parseCoordinates()

	// Start to port 3000 for REST service
	http.ListenAndServe(":3000", mux)
}

var cor = "-121.81496,37.32242,0.0 -121.8151,37.32242,0.0 -121.81531,37.32244,0.0 -121.81541,37.32246,0.0 -121.81552,37.32248,0.0 -121.81562,37.32252,0.0 -121.81582,37.3226,0.0 -121.81594,37.32266,0.0 -121.81604,37.32255,0.0 -121.81608,37.32243,0.0 -121.81661,37.32213,0.0 -121.81734,37.3217,0.0 -121.81805,37.32127,0.0 -121.81878,37.32087,0.0 -121.8195,37.32044,0.0 -121.82021,37.32002,0.0 -121.82094,37.31959,0.0 -121.82165,37.31917,0.0 -121.82243,37.31873,0.0 -121.82265,37.31861,0.0 -121.82287,37.31846,0.0 -121.82305,37.31834,0.0 -121.82316,37.31827,0.0 -121.82305,37.31834,0.0 -121.82287,37.31846,0.0 -121.82265,37.31861,0.0 -121.82243,37.31873,0.0 -121.82273,37.31905,0.0 -121.82276,37.31909,0.0 -121.82298,37.31931,0.0 -121.82317,37.31951,0.0 -121.82355,37.31991,0.0 -121.82387,37.32024,0.0 -121.82411,37.32047,0.0 -121.82442,37.32079,0.0 -121.82512,37.32153,0.0 -121.82535,37.32176,0.0 -121.82586,37.3223,0.0 -121.82599,37.32244,0.0 -121.8265,37.32297,0.0 -121.82673,37.32319,0.0 -121.82688,37.32335,0.0 -121.82756,37.32405,0.0 -121.82891,37.32542,0.0 -121.82906,37.32557,0.0 -121.82925,37.32576,0.0 -121.8296,37.32611,0.0 -121.83031,37.32683,0.0 -121.83144,37.32797,0.0 -121.83176,37.32828,0.0 -121.83212,37.32864,0.0 -121.8335,37.33004,0.0 -121.83384,37.33038,0.0 -121.83417,37.33073,0.0 -121.83447,37.33104,0.0 -121.83495,37.33152,0.0 -121.835,37.33157,0.0 -121.83586,37.33245,0.0 -121.83664,37.33325,0.0 -121.83698,37.33359,0.0 -121.83741,37.33405,0.0 -121.83775,37.33439,0.0 -121.83927,37.33594,0.0 -121.8396,37.33628,0.0 -121.83991,37.33661,0.0 -121.83999,37.3367,0.0 -121.84023,37.33693,0.0 -121.84035,37.33706,0.0 -121.84072,37.33744,0.0 -121.84096,37.33768,0.0 -121.8412,37.33793,0.0 -121.84121,37.33799,0.0 -121.84122,37.33802,0.0 -121.84124,37.33804,0.0 -121.84126,37.33808,0.0 -121.84137,37.33822,0.0 -121.84138,37.33823,0.0 -121.8416,37.33845,0.0 -121.84198,37.33883,0.0 -121.84248,37.33933,0.0 -121.84271,37.33956,0.0 -121.84303,37.33988,0.0 -121.84311,37.34001,0.0 -121.84369,37.3406,0.0 -121.84423,37.34111,0.0 -121.84437,37.34125,0.0 -121.84464,37.34153,0.0 -121.84499,37.3419,0.0 -121.84547,37.34239,0.0 -121.84557,37.34249,0.0 -121.84599,37.34292,0.0 -121.84612,37.34307,0.0 -121.84622,37.34317,0.0 -121.8463,37.34326,0.0 -121.84644,37.34342,0.0 -121.84648,37.34346,0.0 -121.84659,37.34358,0.0 -121.84682,37.34384,0.0 -121.84704,37.34405,0.0 -121.84728,37.34429,0.0 -121.84742,37.34444,0.0 -121.84763,37.34465,0.0 -121.84789,37.34492,0.0 -121.84794,37.34497,0.0 -121.84802,37.34498,0.0 -121.84824,37.34522,0.0 -121.84859,37.34557,0.0 -121.84865,37.34564,0.0 -121.849,37.346,0.0 -121.84931,37.34631,0.0 -121.8496,37.34661,0.0 -121.84992,37.34693,0.0 -121.85028,37.34728,0.0 -121.8508,37.3478,0.0 -121.85131,37.34833,0.0 -121.85156,37.34864,0.0 -121.85183,37.34896,0.0 -121.85202,37.3492,0.0 -121.85203,37.34926,0.0 -121.85203,37.34927,0.0 -121.85204,37.34928,0.0 -121.85204,37.34929,0.0 -121.85209,37.34935,0.0 -121.85229,37.34957,0.0 -121.85258,37.34989,0.0 -121.85274,37.35004,0.0 -121.85287,37.35017,0.0 -121.85298,37.35027,0.0 -121.85312,37.3504,0.0 -121.8532,37.35048,0.0 -121.85335,37.3506,0.0 -121.8535,37.35067,0.0 -121.8541,37.35129,0.0 -121.85431,37.3515,0.0 -121.85448,37.35168,0.0 -121.85473,37.35196,0.0 -121.85474,37.35206,0.0 -121.85497,37.35231,0.0 -121.85518,37.35252,0.0 -121.85541,37.35277,0.0 -121.85556,37.35293,0.0 -121.85568,37.35305,0.0 -121.85582,37.35319,0.0 -121.85601,37.35337,0.0 -121.85612,37.35347,0.0 -121.8562,37.35353,0.0 -121.85631,37.35362,0.0 -121.85641,37.3537,0.0 -121.85655,37.35382,0.0 -121.85669,37.35395,0.0 -121.8568,37.35399,0.0 -121.85696,37.35414,0.0 -121.85797,37.35519,0.0 -121.85821,37.35542,0.0 -121.85891,37.35613,0.0 -121.8591,37.35633,0.0 -121.85916,37.35639,0.0 -121.86019,37.35746,0.0 -121.86044,37.35772,0.0 -121.86058,37.35786,0.0 -121.86073,37.35802,0.0 -121.86082,37.35811,0.0 -121.86102,37.35832,0.0 -121.86125,37.35855,0.0 -121.86174,37.35903,0.0 -121.86337,37.36068,0.0 -121.86411,37.36144,0.0 -121.86461,37.36196,0.0 -121.86485,37.3622,0.0 -121.86516,37.36251,0.0 -121.86563,37.36297,0.0 -121.86701,37.36437,0.0 -121.86729,37.36467,0.0 -121.8675,37.36489,0.0 -121.86754,37.36498,0.0 -121.86766,37.36511,0.0 -121.86776,37.36521,0.0 -121.86805,37.36551,0.0 -121.86808,37.36554,0.0 -121.86827,37.36572,0.0 -121.86842,37.36585,0.0 -121.86865,37.36605,0.0 -121.86876,37.36614,0.0 -121.86883,37.36622,0.0 -121.86889,37.36628,0.0 -121.86898,37.36638,0.0 -121.86929,37.36679,0.0 -121.86936,37.36688,0.0 -121.86949,37.36711,0.0 -121.87007,37.36797,0.0 -121.87033,37.36834,0.0 -121.87077,37.369,0.0 -121.87078,37.36901,0.0 -121.87079,37.36902,0.0 -121.8709,37.36909,0.0 -121.87104,37.36928,0.0 -121.87119,37.36948,0.0 -121.8715,37.36994,0.0 -121.87157,37.37006,0.0 -121.87169,37.37023,0.0 -121.8718,37.37042,0.0 -121.87195,37.37064,0.0 -121.87204,37.37076,0.0 -121.87216,37.37093,0.0 -121.87217,37.37097,0.0 -121.87218,37.37099,0.0 -121.87219,37.37102,0.0 -121.87222,37.37105,0.0 -121.87243,37.37135,0.0 -121.87253,37.3715,0.0 -121.87259,37.37161,0.0 -121.87262,37.37167,0.0 -121.87264,37.37171,0.0 -121.87266,37.37179,0.0 -121.87267,37.37186,0.0 -121.87267,37.37194,0.0 -121.87267,37.37198,0.0 -121.87267,37.37203,0.0 -121.87265,37.37214,0.0 -121.87263,37.37225,0.0 -121.87252,37.37265,0.0 -121.87251,37.37271,0.0 -121.87251,37.37274,0.0 -121.8725,37.37277,0.0 -121.8725,37.37283,0.0 -121.87252,37.37296,0.0 -121.87253,37.37304,0.0 -121.87254,37.37311,0.0 -121.87256,37.37318,0.0 -121.87259,37.37325,0.0 -121.87263,37.37333,0.0 -121.87267,37.3734,0.0 -121.87279,37.37357,0.0 -121.87297,37.37379,0.0 -121.87305,37.37389,0.0 -121.87327,37.37412,0.0 -121.87332,37.37417,0.0 -121.87351,37.37438,0.0 -121.87384,37.37474,0.0 -121.87384,37.37475,0.0 -121.87398,37.3748,0.0 -121.87404,37.37487,0.0 -121.8742,37.37505,0.0 -121.87432,37.37518,0.0 -121.87465,37.37555,0.0 -121.87522,37.3762,0.0 -121.87554,37.37658,0.0 -121.87561,37.37666,0.0 -121.87583,37.37689,0.0 -121.87586,37.37693,0.0 -121.87589,37.37696,0.0 -121.87593,37.377,0.0 -121.87631,37.37743,0.0 -121.87648,37.37761,0.0 -121.87652,37.37775,0.0 -121.87653,37.37776,0.0 -121.8769,37.37817,0.0 -121.87764,37.37903,0.0 -121.87774,37.37915,0.0 -121.87808,37.37953,0.0 -121.87823,37.3797,0.0 -121.87832,37.3798,0.0 -121.87842,37.37991,0.0 -121.87848,37.37995,0.0 -121.87854,37.38,0.0 -121.87862,37.38008,0.0 -121.8787,37.38016,0.0 -121.87902,37.38047,0.0 -121.8796,37.38099,0.0 -121.88017,37.38152,0.0 -121.88049,37.38182,0.0 -121.8807,37.38201,0.0 -121.88092,37.38222,0.0 -121.88107,37.38235,0.0 -121.88131,37.38257,0.0 -121.88154,37.38279,0.0 -121.88187,37.3831,0.0 -121.8822,37.3834,0.0 -121.88286,37.38401,0.0 -121.88364,37.38474,0.0 -121.88395,37.38504,0.0 -121.88412,37.38523,0.0 -121.88431,37.38544,0.0 -121.88444,37.38559,0.0 -121.88505,37.3863,0.0 -121.88534,37.38663,0.0 -121.88579,37.38713,0.0 -121.88597,37.38735,0.0 -121.88606,37.38748,0.0 -121.88621,37.38765,0.0 -121.88675,37.38829,0.0 -121.88709,37.38867,0.0 -121.88743,37.38906,0.0 -121.88755,37.3892,0.0 -121.88759,37.38924,0.0 -121.88837,37.39018,0.0 -121.88872,37.39059,0.0 -121.88882,37.3907,0.0 -121.88933,37.39131,0.0 -121.88973,37.39179,0.0 -121.88992,37.39202,0.0 -121.89013,37.39229,0.0 -121.89023,37.39244,0.0 -121.89033,37.39262,0.0 -121.89042,37.39279,0.0 -121.89046,37.39292,0.0 -121.89053,37.39314,0.0 -121.89059,37.39341,0.0 -121.89063,37.39363,0.0 -121.89065,37.3939,0.0 -121.89067,37.39432,0.0 -121.89084,37.39654,0.0 -121.89085,37.39666,0.0 -121.89088,37.39706,0.0 -121.89106,37.39943,0.0 -121.89113,37.40039,0.0 -121.89114,37.40063,0.0 -121.89111,37.4009,0.0 -121.89108,37.40115,0.0 -121.89103,37.40131,0.0 -121.89096,37.4015,0.0 -121.89076,37.40202,0.0 -121.89072,37.40214,0.0 -121.89069,37.40223,0.0 -121.89065,37.40242,0.0 -121.8906,37.40295,0.0 -121.89056,37.40326,0.0 -121.89054,37.40337,0.0 -121.89064,37.40338,0.0 -121.89092,37.4034,0.0 -121.89128,37.40342,0.0 -121.8917,37.40344,0.0 -121.89198,37.40339,0.0 -121.8922,37.40341,0.0 -121.89236,37.40343,0.0 -121.89284,37.40346,0.0 -121.89358,37.40352,0.0 -121.89389,37.40355,0.0 -121.89427,37.40358,0.0 -121.89435,37.40361,0.0 -121.89437,37.40362,0.0 -121.89449,37.40367,0.0 -121.89502,37.40372,0.0 -121.89524,37.40374,0.0 -121.8954,37.40376,0.0 -121.89579,37.4038,0.0 -121.89677,37.40388,0.0 -121.89727,37.40392,0.0 -121.89734,37.40392,0.0 -121.89739,37.40393,0.0 -121.89744,37.40394,0.0 -121.89749,37.40395,0.0 -121.89755,37.40397,0.0 -121.89759,37.40399,0.0 -121.89764,37.40404,0.0 -121.89768,37.40407,0.0 -121.89771,37.40413,0.0 -121.89776,37.40427,0.0 -121.8979,37.40457,0.0 -121.89794,37.40466,0.0 -121.89797,37.40473,0.0 -121.89805,37.40483,0.0 -121.89814,37.40493,0.0 -121.89818,37.40497,0.0 -121.89822,37.405,0.0 -121.89826,37.40502,0.0 -121.89835,37.40504,0.0 -121.89853,37.40523,0.0 -121.89872,37.40545,0.0 -121.89884,37.40562,0.0 -121.89894,37.40579,0.0 -121.89906,37.40597,0.0 -121.89916,37.40616,0.0 -121.89927,37.40641,0.0 -121.89936,37.40667,0.0 -121.89939,37.40678,0.0 -121.89945,37.40704,0.0 -121.89953,37.40795,0.0 -121.89955,37.40812,0.0 -121.89955,37.40819,0.0 -121.89956,37.40831,0.0 -121.89962,37.40909,0.0 -121.89969,37.41015,0.0 -121.8997,37.41031,0.0 -121.89973,37.41083,0.0 -121.89971,37.4111,0.0 -121.89969,37.41128,0.0 -121.89969,37.41133,0.0 -121.89966,37.41139,0.0 -121.8996,37.41148,0.0 -121.89953,37.41159,0.0 -121.89943,37.41174,0.0 -121.89932,37.41178,0.0 -121.89887,37.4124,0.0 -121.89869,37.4126,0.0 -121.8982,37.41327,0.0 -121.89826,37.4133,0.0"
