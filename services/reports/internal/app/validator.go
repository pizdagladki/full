package app

import "github.com/go-playground/validator/v10"

// echoValidator adapts validator/v10 to Echo's Validator interface so handlers
// can call c.Validate(payload) on bound request DTOs.
type echoValidator struct {
	validate *validator.Validate
}

// Validate validates i and is invoked by Echo on bound request payloads.
func (e *echoValidator) Validate(i any) error {
	return e.validate.Struct(i)
}

func (a *App) initValidator() {
	a.validator = &echoValidator{validate: validator.New()}
}
