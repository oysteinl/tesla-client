package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

var ErrAuth = errors.New("invalid auth")
var ErrOffline = errors.New("vehicle is offline")

func FetchAccessToken(refreshToken string, url string) (string, error) {
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
	request, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
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

func RequestVehicleStatus(accessToken string, url string) (VehicleStatus, error) {
	// Create a new request with the GET method
	request, err := http.NewRequest("GET", url, nil)
	if err != nil {
		fmt.Println("Error creating request:", err)
		return VehicleStatus{}, err
	}
	request.Header.Set("Authorization", "Bearer "+accessToken)
	client := &http.Client{}

	// Send the request
	response, err := client.Do(request)
	if err != nil {
		fmt.Println("Error making VehicleStatus request:", err)
		return VehicleStatus{}, err
	}
	defer response.Body.Close()

	if response.StatusCode == http.StatusUnauthorized {
		return VehicleStatus{}, ErrAuth
	} else if response.StatusCode == http.StatusRequestTimeout {
		return VehicleStatus{}, ErrOffline
	} else if response.StatusCode != http.StatusOK {
		return VehicleStatus{}, errors.New("Error: Unexpected status code " + response.Status)
	}

	// Decode the response body into the VehicleStatus struct
	var vehicleStatusResponse VehicleStatus
	if err := json.NewDecoder(response.Body).Decode(&vehicleStatusResponse); err != nil {
		fmt.Println("Error decoding response body:", err)
		return VehicleStatus{}, err
	}
	return vehicleStatusResponse, nil
}
