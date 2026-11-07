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

// ScopedPurchaseDecider lets a platform operator retain their own guest/media
// workflows without routing other customers' guest payments into that bot.
type ScopedPurchaseDecider struct {
	BusinessID string
	Decider    PurchaseDecider
}

func (s ScopedPurchaseDecider) ResolveRequestBusiness(id string) (string, error) {
	business, err := s.Decider.ResolveRequestBusiness(id)
	if err != nil {
		return "", err
	}
	if business != s.BusinessID {
		return "", errors.New("request not found")
	}
	return business, nil
}
func (s ScopedPurchaseDecider) DecidePurchaseRequest(business, id string, approve bool) error {
	if business != s.BusinessID {
		return errors.New("request not found")
	}
	resolved, err := s.ResolveRequestBusiness(id)
	if err != nil || resolved != business {
		return errors.New("request not found")
	}
	return s.Decider.DecidePurchaseRequest(business, id, approve)
}
