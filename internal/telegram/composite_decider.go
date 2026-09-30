package telegram

import "errors"

// CompositePurchaseDecider lets one bot update stream serve both Store and
// guest-WiFi purchase IDs without starting competing Telegram pollers.
type CompositePurchaseDecider struct{ Deciders []PurchaseDecider }

func (c CompositePurchaseDecider) ResolveRequestBusiness(id string) (string, error) {
	var last error
	for _, d := range c.Deciders {
		business, err := d.ResolveRequestBusiness(id)
		if err == nil {
			return business, nil
		}
		last = err
	}
	if last == nil {
		last = errors.New("request not found")
	}
	return "", last
}

func (c CompositePurchaseDecider) DecidePurchaseRequest(businessID, id string, approve bool) error {
	for _, d := range c.Deciders {
		resolved, err := d.ResolveRequestBusiness(id)
		if err == nil && resolved == businessID {
			return d.DecidePurchaseRequest(businessID, id, approve)
		}
	}
	return errors.New("request not found")
}
