package service

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/stripe/stripe-go/v81"
	"github.com/stripe/stripe-go/v81/paymentintent"
	"github.com/stripe/stripe-go/v81/webhook"
)

// StripePaymentProvider implements PaymentProvider using the Stripe API.
type StripePaymentProvider struct {
	signingSecret string
}

// NewStripePaymentProvider returns a PaymentProvider backed by Stripe.
// apiKey is set as the package-level stripe.Key global.
func NewStripePaymentProvider(apiKey, signingSecret string) PaymentProvider {
	stripe.Key = apiKey

	return &StripePaymentProvider{signingSecret: signingSecret}
}

// CreatePaymentIntent creates a Stripe PaymentIntent for the given product and amount.
func (p *StripePaymentProvider) CreatePaymentIntent(
	_ context.Context,
	productID int64,
	amountCents int,
) (string, string, error) {
	params := &stripe.PaymentIntentParams{
		Amount:   new(int64),
		Currency: stripe.String(string(stripe.CurrencyUSD)),
		Metadata: map[string]string{
			"product_id": strconv.FormatInt(productID, 10),
		},
	}
	*params.Amount = int64(amountCents)

	pi, err := paymentintent.New(params)
	if err != nil {
		return "", "", fmt.Errorf("create payment intent: %w", err)
	}

	return pi.ClientSecret, pi.ID, nil
}

// VerifyWebhook validates the raw Stripe webhook payload and signature header.
// For non-succeeded events it returns succeeded=false and a nil error.
func (p *StripePaymentProvider) VerifyWebhook(payload []byte, sigHeader string) (string, string, bool, error) {
	event, err := webhook.ConstructEvent(payload, sigHeader, p.signingSecret)
	if err != nil {
		return "", "", false, fmt.Errorf("verify webhook: %w", err)
	}

	if event.Type != stripe.EventTypePaymentIntentSucceeded {
		return event.ID, "", false, nil
	}

	var pi stripe.PaymentIntent

	err = json.Unmarshal(event.Data.Raw, &pi)
	if err != nil {
		return "", "", false, fmt.Errorf("unmarshal payment intent: %w", err)
	}

	return event.ID, pi.ID, true, nil
}
