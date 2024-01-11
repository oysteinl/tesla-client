package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/oysteinl/tesla-client/service"
	"os"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/joho/godotenv"
	log "github.com/sirupsen/logrus"
)

var vehicleUrl string
var vehicleUrlExists bool
var refreshTokenUrl string
var refreshTokenUrlExists bool
var refreshToken string
var refreshTokenExists bool
var mqttUrl string
var mqttUrlExists bool
var mqttAliveTopic string
var mqttAliveTopicExists bool
var mqttUser string
var mqttUserExists bool
var mqttPass string
var mqttPassExists bool

var accessToken string
var mqttClient mqtt.Client

var homeAndAwayTracker HomeAndAwayTracker

type HomeAndAwayTracker struct {
	mu   sync.Mutex
	home bool
}

func (h *HomeAndAwayTracker) SetHome(home bool) {
	h.mu.Lock()
	h.home = home
	h.mu.Unlock()
}

func init() {
	log.SetLevel(log.DebugLevel)
	log.SetFormatter(&log.TextFormatter{
		DisableColors: false,
		FullTimestamp: true,
	})

	envPath := ".env"
	if len(os.Args) == 2 {
		envPath = os.Args[1]
	}
	if err := godotenv.Load(envPath); err != nil {
		panic("No .env file found")
	}
	vehicleUrl, vehicleUrlExists = os.LookupEnv("VEHICLE_URL")
	refreshTokenUrl, refreshTokenUrlExists = os.LookupEnv("REFRESH_TOKEN_URL")
	refreshToken, refreshTokenExists = os.LookupEnv("REFRESH_TOKEN")
	mqttUrl, mqttUrlExists = os.LookupEnv("MQTT_URL")
	mqttAliveTopic, mqttAliveTopicExists = os.LookupEnv("MQTT_ALIVE_TOPIC")
	mqttUser, mqttUserExists = os.LookupEnv("MQTT_USER")
	mqttPass, mqttPassExists = os.LookupEnv("MQTT_PASSWORD")

	if !vehicleUrlExists || !mqttUrlExists || !refreshTokenUrlExists || !mqttUserExists || !mqttPassExists || !refreshTokenExists || !mqttAliveTopicExists {
		panic("Env variables not set")
	}
}

func main() {
	log.Info("Starting tesla-client")
	keepAlive := make(chan os.Signal)
	listen()
	defer mqttClient.Disconnect(500)
	<-keepAlive
}

func listen() {
	mqttClient = service.ConnectMQTT("teslaClient", mqttUser, mqttPass, mqttUrl)
	mqttClient.Subscribe(mqttAliveTopic, 0, mqttAliveCallback)
}

var done = make(chan bool)

func mqttAliveCallback(_ mqtt.Client, msg mqtt.Message) {
	payload := string(msg.Payload())
	log.Infof("* [%s] %s\n", msg.Topic(), payload)

	if payload == "home" {
		if homeAndAwayTracker.home {
			return //Already home
		}
		time.Sleep(10 * time.Second) //Wait to let the vehicle come online
		fetchDataAndPublishState()
		//Start scheduled fetches every 10 min
		ticker := time.NewTicker(10 * time.Minute)
		go func() {
			for {
				select {
				case <-done:
					ticker.Stop()
					log.Debug("Ticker stopped")
					return
				case <-ticker.C:
					log.Debug("Tick received")
					fetchDataAndPublishState()
				}
			}
		}()
		homeAndAwayTracker.SetHome(true)
	} else if payload == "not_home" {
		if !homeAndAwayTracker.home {
			return //Already away
		}
		//Cancel scheduled fetches
		done <- true
		homeAndAwayTracker.SetHome(false)
	}
}

func fetchDataAndPublishState() {
	log.Info("Fetching vehicle status")
	if accessToken == "" {
		token, err := service.FetchAccessToken(refreshToken, refreshTokenUrl)
		if err != nil {
			log.Error(err)
		} else {
			accessToken = token
		}
	}
	var vehicleStatus service.VehicleStatus
	vehicleStatus, err := service.RequestVehicleStatus(accessToken, vehicleUrl)
	if err != nil {
		if errors.Is(err, service.ErrAuth) {
			log.Info("Invalid auth, updating token")
			token, err := service.FetchAccessToken(refreshToken, refreshTokenUrl)
			if err != nil {
				log.Error(err)
				return
			}
			accessToken = token
			vehicleStatus, err = service.RequestVehicleStatus(accessToken, vehicleUrl)
			if err != nil {
				log.Error(err)
				return
			}
		} else if errors.Is(err, service.ErrOffline) {
			log.Info("Vehicle offline, backing off")
			return
		} else {
			log.Error(err)
			return
		}
	}
	publishVehicleStatus(vehicleStatus)
}

type attributes struct {
	Charging bool `json:"charging"`
}

func publishVehicleStatus(vehicleStatus service.VehicleStatus) {
	log.Infof("Publishing to queue: BatteryState=%d, Charging=%s",
		vehicleStatus.Response.ChargeState.UsableBatteryLevel,
		vehicleStatus.Response.ChargeState.ChargingState)
	attributes := attributes{Charging: vehicleStatus.Response.ChargeState.ChargingState == "Charging" || vehicleStatus.Response.ChargeState.ChargingState == "NoPower"}
	attributeAsJson, _ := json.Marshal(attributes)
	mqttClient.Publish("/tesla/state/battery", 1, true, fmt.Sprintf("%d", vehicleStatus.Response.ChargeState.UsableBatteryLevel))
	mqttClient.Publish("/tesla/state/charging", 1, true, attributeAsJson)
}
