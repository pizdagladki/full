package domain

import "time"

// Consent is the domain model for a user's registration consents.
type Consent struct {
	IsAdult          bool
	ConsentRecording bool
	ConsentTos       bool
	AcceptedAt       time.Time
}

// ConsentRequest is the DTO for POST /v1/auth/consent.
// All three flags must be explicitly set to true; a false value or a missing
// field both fail validation and produce HTTP 422.
type ConsentRequest struct {
	IsAdult          bool `json:"is_adult"           validate:"required"`
	ConsentRecording bool `json:"consent_recording"  validate:"required"`
	ConsentTos       bool `json:"consent_tos"        validate:"required"`
}

// ConsentInfo is the JSON representation of a user's consent state returned in
// /me. It mirrors Consent but uses snake_case JSON keys for the API.
type ConsentInfo struct {
	IsAdult          bool      `json:"is_adult"`
	ConsentRecording bool      `json:"consent_recording"`
	ConsentTos       bool      `json:"consent_tos"`
	AcceptedAt       time.Time `json:"accepted_at"`
}
