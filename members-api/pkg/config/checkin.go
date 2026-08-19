package config

import "os"

// CheckinSecret is the HMAC key used to sign/validate daily check-in QR tokens.
// Set via the CHECKIN_SECRET environment variable.
var CheckinSecret = os.Getenv("CHECKIN_SECRET")
