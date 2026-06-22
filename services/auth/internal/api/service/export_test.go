package service

// NewGoogleOAuthWithEndpoints exposes newGoogleOAuthWithEndpoints to the
// external test package (service_test) via the standard Go export_test.go
// idiom. The exported name is visible only during testing.
var NewGoogleOAuthWithEndpoints = newGoogleOAuthWithEndpoints
