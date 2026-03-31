package stripe

import "encoding/json"

// parseStripeObject unmarshals the raw JSON from a Stripe event data object.
func parseStripeObject(raw json.RawMessage, target any) error {
	return json.Unmarshal(raw, target)
}
