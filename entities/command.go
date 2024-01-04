package entities

// Command is the request for command
type Command struct {
	ID          string            `json:"id"`
	Script      string            `json:"script"`
	Environment map[string]string `json:"environment"`
	WorkDirBase string            `json:"workdirbase"`
	//
	User string `json:"user"`
	//
	Engine     string  `json:"engine"`
	Image      string  `json:"image"`
	CPU        float64 `json:"cpu"`
	Memory     int64   `json:"memory"`
	Platform   string  `json:"platform"`
	Network    string  `json:"network"`
	Privileged bool    `json:"privileged"`
}
