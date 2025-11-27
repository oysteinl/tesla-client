package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"github.com/oysteinl/tesla-client/service"

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
var mqttUser string
var mqttUserExists bool
var mqttPass string
var mqttPassExists bool

var accessToken string
var mqttClient mqtt.Client

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
	mqttUser, mqttUserExists = os.LookupEnv("MQTT_USER")
	mqttPass, mqttPassExists = os.LookupEnv("MQTT_PASSWORD")

	if !vehicleUrlExists || !mqttUrlExists || !refreshTokenUrlExists || !mqttUserExists || !mqttPassExists || !refreshTokenExists {
		panic("Env variables not set")
	}
}

func main() {
	log.Info("Starting tesla-client")
	keepAlive := make(chan os.Signal)
	// Channel to signal termination (optional, for a clean exit)
	mqttClient = service.ConnectMQTT("teslaClient", mqttUser, mqttPass, mqttUrl)
	// Start the polling routine
	go poller()
	fetchDataAndPublishState()
	<-keepAlive
}

func poller() {
	interval := 15 * time.Minute
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		<-ticker.C
		if fetchDataAndPublishState() {
			log.Info("Changing interval to 5 minutes.")
			ticker.Reset(5 * time.Minute)
		} else {
			log.Info("Continuing with 15-minute interval.")
			ticker.Reset(15 * time.Minute)
		}

	}
}

func fetchDataAndPublishState() bool {

	log.Info("Fetching vehicle status")
	if accessToken == "" {
		token, err := service.FetchAccessToken(refreshToken, refreshTokenUrl)
		if err != nil {
			log.Error(err)
			return false
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
				return false
			}
			accessToken = token
			vehicleStatus, err = service.RequestVehicleStatus(accessToken, vehicleUrl)
			if err != nil {
				log.Error(err)
				return false
			}
		} else if errors.Is(err, service.ErrOffline) {
			log.Info("Vehicle offline, backing off")
			return false
		} else {
			log.Error(err)
			return false
		}
	}
	publishVehicleStatus(vehicleStatus)
	return true
}

type attributes struct {
	Charging bool `json:"charging"`
	Driving  bool `json:"driving"`
}

func publishVehicleStatus(vehicleStatus service.VehicleStatus) {
	log.Infof("Publishing to queue: BatteryState=%d, Charging=%s ShiftState=%s",
		vehicleStatus.Response.ChargeState.UsableBatteryLevel,
		vehicleStatus.Response.ChargeState.ChargingState,
		vehicleStatus.Response.DriveState.ShiftState)
	attributes := attributes{
		Charging: vehicleStatus.Response.ChargeState.ChargingState == "Charging" || vehicleStatus.Response.ChargeState.ChargingState == "NoPower",
		Driving:  vehicleStatus.Response.DriveState.ShiftState == "D" || vehicleStatus.Response.DriveState.ShiftState == "R"}
	attributeAsJson, _ := json.Marshal(attributes)
	mqttClient.Publish("/tesla/state/battery", 1, true, fmt.Sprintf("%d", vehicleStatus.Response.ChargeState.UsableBatteryLevel))
	mqttClient.Publish("/tesla/state/charging", 1, true, attributeAsJson)
}
