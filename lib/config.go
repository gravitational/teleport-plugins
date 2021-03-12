package lib

// TeleportConfig stores config options for where
// the Teleport's Auth server is listening, and what certificates to
// use to authenticate in it.
type TeleportConfig struct {
	AuthServer string `toml:"auth_server"`
	ClientKey  string `toml:"client_key"`
	ClientCrt  string `toml:"client_crt"`
	RootCAs    string `toml:"root_cas"`
}
