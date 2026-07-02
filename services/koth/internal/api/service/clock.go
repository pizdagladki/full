package service

import "time"

// realClock is the production Clock implementation.
type realClock struct{}

func (realClock) Now() time.Time { return time.Now() }

// RealClock is the production clock.
var RealClock Clock = realClock{}
