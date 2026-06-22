// Package delivery holds the signaling service WebSocket and HTTP handlers
// (transport layer: request parse/validate, status codes, serialization). Handler
// interfaces are added here by downstream resource slices via the new-resource
// skill; the scaffold ships only the liveness probe and the /ws ping-ack handler
// wired in the app layer.
package delivery
