package server

// Config is the configuration of caas server
type Config struct {
	Port int64 `config:"port,default=8838"`
	//
	Path string `config:"path"`
	//
	Shell       string            `config:"shell"`
	Environment map[string]string `config:"environment"`
	Timeout     int64             `config:"timeout"`
	// Auth
	ClientID     string `config:"client_id"`
	ClientSecret string `config:"client_secret"`
	AuthService  string `config:"auth_service"`
	//
	MetadataDir string `config:"metadatadir"`
	//
	WorkDir string `config:"workdir"`
	//
	IsCommandCancelOnCloseDisabled bool `config:"is_command_cancel_on_close_disabled"`
	//
	IsCleanWorkDirDisabled bool `config:"is_clean_workdir_disabled"`
	//
	IsCleanMetadataDirDisabled bool `config:"is_clean_metadatadir_disabled"`

	// Terminal
	TerminalPath        string `config:"terminal_path"`
	TerminalShell       string `config:"terminal_shell"`
	TerminalDriver      string `config:"terminal_driver"`
	TerminalDriverImage string `config:"terminal_driver_image"`
	TerminalInitCommand string `config:"terminal_init_command"`
	//
	TerminalRelay string `config:"terminal_relay"`

	//
	IsAutoReport bool `config:"is_auto_report"`
}
