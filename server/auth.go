package server

import (
	"fmt"

	caas "github.com/go-idp/agent"
	"github.com/go-zoox/fetch"
)

func createAuthenticator(cfg *Config) func(clientID, clientSecret string) (err error) {
	return func(clientID, clientSecret string) (err error) {
		// static auth
		if cfg.ClientID != "" && cfg.ClientSecret != "" {
			if clientID != cfg.ClientID || clientSecret != cfg.ClientSecret {
				return fmt.Errorf("invalid client id or secret")
			}

			return nil
		}

		if cfg.AuthService != "" {
			// Protocol:
			// Request:
			//   POST <AuthService>
			//		Header:
			//   		Content-Type: application/json
			//   		X-Client-ID: <ClientID>
			//   		X-Client-Secret: <ClientSecret>
			//
			//		Body:
			//   	{
			//     	"client_id": <ClientID>,
			//     	"client_secret": <ClientSecret>
			//   	}
			//
			// Response:
			//   Status: 200
			//   Body:
			//   	{
			//			"code": 200,
			//     	"message": "ok"
			//   	}
			//
			response, err := fetch.Post(cfg.AuthService, &fetch.Config{
				Headers: fetch.Headers{
					"Content-Type":    "application/json",
					"User-Agent":      fmt.Sprintf("caas/%s", caas.Version),
					"X-Client-ID":     clientID,
					"X-Client-Secret": clientSecret,
				},
				Body: map[string]string{
					"client_id":     clientID,
					"client_secret": clientSecret,
				},
			})
			if err != nil {
				return fmt.Errorf("failed to communicate with auth service(%s): %s", cfg.AuthService, err)
			}

			if response.Status != 200 {
				return fmt.Errorf("failed to authenticate by response status(%d): %s", response.Status, response.String())
			}

			code := response.Get("code").Int()
			if code != 200 {
				message := response.Get("message").String()
				if message == "" {
					message = fmt.Sprintf("unknown error (%s)", response.String())
				}

				return fmt.Errorf("[%d] %s", code, message)
			}

			return nil
		}

		return nil
	}
}
