package cmd

type UserInfo struct {
	account  string
	password string
	schoolID string
	token    string
}

// Global variable to hold user info flags, shared across commands.
var u UserInfo

type signconfig struct {
	address      string
	address_name string
	latitude     string
	longitude    string
	remark       string
	comment      string
	province     string
	city         string
	debug        bool
}
