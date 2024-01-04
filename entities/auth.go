package entities

// AuthRequest is the request for authentication
type AuthRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}
