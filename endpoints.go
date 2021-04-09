package goblue

type endpoints struct {
	DeviceID    string
	Lang        string
	Login       string
	AccessToken string
	Vehicles    string
	Status      string
}

func defaultEndpoints() endpoints {
	return endpoints{
		DeviceID:    "/api/v1/spa/notifications/register",
		Lang:        "/api/v1/user/language",
		Login:       "/api/v1/user/signin",
		AccessToken: "/api/v1/user/oauth2/token",
		Vehicles:    "/api/v1/spa/vehicles",
		Status:      "/api/v1/spa/vehicles/%s/status",
	}
}
