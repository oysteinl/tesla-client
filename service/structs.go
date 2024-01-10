package service

type responseToken struct {
	AccessToken string `json:"access_token"`
}

type VehicleStatus struct {
	Response Response `json:"response"`
}

type Response struct {
	ChargeState ChargeState `json:"charge_state"`
}

type ChargeState struct {
	UsableBatteryLevel int    `json:"usable_battery_level"`
	ChargingState      string `json:"charging_state"`
}

type body struct {
	GrantType    string `json:"grant_type"`
	ClientId     string `json:"client_id"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
}
