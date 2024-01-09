package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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

var ErrAuth = errors.New("invalid auth")
var ErrOffline = errors.New("vehicle is offline")

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
	<-keepAlive
}

func listen() {
	mqttClient = connectMQTT("teslaClient")
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
					log.Debugf("Tick received")
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
		log.Debugf("Stopping ticker")
		done <- true
		log.Debugf("Done sent")
		homeAndAwayTracker.SetHome(false)
	}
}

func fetchDataAndPublishState() {
	log.Info("Fetching vehicle status")
	if accessToken == "" {
		token, err := fetchAccessToken()
		if err != nil {
			log.Error(err)
		} else {
			accessToken = token
		}
	}
	var vehicleStatus vehicleStatus
	vehicleStatus, err := requestVehicleStatus()
	if err != nil {
		if errors.Is(err, ErrAuth) {
			token, err := fetchAccessToken()
			if err != nil {
				log.Error(err)
				return
			}
			accessToken = token
			vehicleStatus, err = requestVehicleStatus()
			if err != nil {
				log.Error(err)
				return
			}
		} else if errors.Is(err, ErrOffline) {
			log.Info("Vehicle offline")
			return
		} else {
			log.Error(err)
			return
		}
	}
	publishVehicleStatus(vehicleStatus)
}

func fetchAccessToken() (string, error) {
	body := body{
		GrantType:    "refresh_token",
		ClientId:     "ownerapi",
		RefreshToken: refreshToken,
		Scope:        "openid email offline_access",
	}
	jsonData, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	request, err := http.NewRequest("POST", refreshTokenUrl, bytes.NewBuffer(jsonData))
	if err != nil {
		return "", err
	}
	request.Header.Set("Content-Type", "application/json")
	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", errors.New("Error: Unexpected status code: " + response.Status)
	}
	var result responseToken
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		return "", err
	}
	return result.AccessToken, nil
}

func requestVehicleStatus() (vehicleStatus, error) {
	// Create a new request with the GET method
	request, err := http.NewRequest("GET", vehicleUrl, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return vehicleStatus{}, err
	}

	// Set the Authorization header with the bearer token
	request.Header.Set("Authorization", "Bearer "+accessToken)

	// Create an HTTP client
	client := &http.Client{}

	// Send the request
	response, err := client.Do(request)
	if err != nil {
		fmt.Println("Error making VehicleStatus request:", err)
		return vehicleStatus{}, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		return vehicleStatus{}, ErrAuth
	} else if response.StatusCode == http.StatusRequestTimeout {
		return vehicleStatus{}, ErrOffline
	} else if response.StatusCode != http.StatusOK {
		return vehicleStatus{}, errors.New("Error: Unexpected status code " + response.Status)
	}

	// Decode the response body into the VehicleStatus struct
	var vehicleStatusResponse vehicleStatus
	if err := json.NewDecoder(response.Body).Decode(&vehicleStatusResponse); err != nil {
		fmt.Println("Error decoding response body:", err)
		return vehicleStatus{}, err
	}
	return vehicleStatusResponse, nil
}

type responseToken struct {
	AccessToken string `json:"access_token"`
}

type attributes struct {
	Charging bool `json:"charging"`
}

func publishVehicleStatus(vehicleStatus vehicleStatus) {
	attributes := attributes{Charging: vehicleStatus.Response.ChargeState.ChargingState == "Charging"}
	attributeAsJson, _ := json.Marshal(attributes)
	mqttClient.Publish("/tesla/state/battery", 1, true, fmt.Sprintf("%d", vehicleStatus.Response.ChargeState.UsableBatteryLevel))
	mqttClient.Publish("/tesla/state/charging", 1, true, attributeAsJson)
}

type body struct {
	GrantType    string `json:"grant_type"`
	ClientId     string `json:"client_id"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}

type chargeState struct {
	UsableBatteryLevel int    `json:"usable_battery_level"`
	ChargingState      string `json:"charging_state"`
}

type response struct {
	ChargeState chargeState `json:"charge_state"`
}

type vehicleStatus struct {
	Response response `json:"response"`
}

func connectMQTT(clientId string) mqtt.Client {
	opts := createClientOptions(clientId)
	client := mqtt.NewClient(opts)
	token := client.Connect()
	for !token.WaitTimeout(3 * time.Second) {
	}
	if err := token.Error(); err != nil {
		log.Fatal(err)
	}
	return client
}

func createClientOptions(clientId string) *mqtt.ClientOptions {
	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", mqttUrl, 1883))
	opts.SetUsername(mqttUser)
	opts.SetPassword(mqttPass)
	opts.SetClientID(clientId)
	return opts
}
