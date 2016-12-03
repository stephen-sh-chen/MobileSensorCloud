package main

import (
	"encoding/json"
	"fmt"
	"github.com/pubnub/go/messaging"
	"gopkg.in/mgo.v2"
	"net/http"
)

// Global variables
var my_pubkey = "pub-c-bcc7ac96-ccbe-4577-bd6f-66321585d73a"
var my_subkey = "sub-c-6d08ffd2-a589-11e6-80e1-0619f8945a4f"
var my_channel = "my_channel"
var db_addr = "54.191.90.246:27017"

type SensorSignal struct {
	SignalID  string  `json:"signal_id"`
	SensorID  string  `json:"sensor_id"`
	TimeStamp string  `json:"last_update"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Long      float64 `json:"longitude"`
	Lat       float64 `json:"latitude"`
}

func addSensorSignal2DB(s *mgo.Session, msg interface{}) {
	session := s.Copy()
	//defer session.Close()

	var sig SensorSignal
	err := json.Unmarshal([]byte(msg.(string)), &sig)
	if err != nil {
		fmt.Println("json decode fail!!!")
		return
	}

	c := session.DB("fullstack").C("signal")
	err = c.Insert(sig)
	if err != nil {
		fmt.Println(err)
		return
	}
}

func subscribeSensorInfo(s *mgo.Session) {
	pubnub := messaging.NewPubnub(my_pubkey, my_subkey, "", "", false, "")
	successChannel := make(chan []byte)
	errorChannel := make(chan []byte)

	session := s.Copy()
	//defer session.Close()

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
					//c := session.DB("fullstack").C("signal")
					//err = c.Insert(m[0])
					//if err != nil {
					//    fmt.Print("insert DB fail!")
					//}
					go addSensorSignal2DB(session, m[0])
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

func main() {
	session, err := mgo.Dial(db_addr)
	if err != nil {
		panic(err)
		fmt.Println("cannot connet to the mongo DB!!!!")
	}
	defer session.Close()
	session.SetMode(mgo.Monotonic, true)
	fmt.Println("leave!")

	subscribeSensorInfo(session)
	http.ListenAndServe(":8080", nil)
}
