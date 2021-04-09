package goblue

type Config struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Pin      string `json:"pin"`
	Brand    Brand  `json:"brand"`
	Region   Region `json:"region"`
}
