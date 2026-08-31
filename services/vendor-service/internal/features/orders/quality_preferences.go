package orders

import (
	"errors"
	"strings"
)

type QualityPreferenceType string

const (
	QualityRipeness    QualityPreferenceType = "RIPENESS"
	QualityFirmness    QualityPreferenceType = "FIRMNESS"
	QualityCutSize     QualityPreferenceType = "CUT_SIZE"
	QualityPreparation QualityPreferenceType = "PREPARATION"
	QualityCleaning    QualityPreferenceType = "CLEANING"
	QualitySorting     QualityPreferenceType = "SORTING"
)

type QualityPreferenceSource string

const (
	PreferenceCustomerSaved    QualityPreferenceSource = "CUSTOMER_SAVED"
	PreferenceCheckoutExplicit QualityPreferenceSource = "CHECKOUT_EXPLICIT"
	PreferencePlatformDefault  QualityPreferenceSource = "PLATFORM_DEFAULT"
)

type QualityPreference struct {
	Type          QualityPreferenceType   `json:"type"`
	Value         string                  `json:"value"`
	Source        QualityPreferenceSource `json:"source"`
	SourceVersion int64                   `json:"source_version"`
}

type QualityResult string

const (
	QualityMet          QualityResult = "MET"
	QualityPartial      QualityResult = "PARTIAL"
	QualityNotAvailable QualityResult = "NOT_AVAILABLE"
	QualitySubstitute   QualityResult = "SUBSTITUTE"
)

func (p QualityPreference) Validate() error {
	if strings.TrimSpace(string(p.Type)) == "" || strings.TrimSpace(p.Value) == "" {
		return errors.New("quality preference type and value are required")
	}
	switch p.Source {
	case PreferenceCustomerSaved, PreferenceCheckoutExplicit, PreferencePlatformDefault:
	default:
		return errors.New("invalid quality preference source")
	}
	if p.SourceVersion < 0 {
		return errors.New("quality preference source version cannot be negative")
	}
	return nil
}

// ValidateQualityPreferences rejects contradictory duplicate types while
// allowing several independent preferences on one line.
func ValidateQualityPreferences(values []QualityPreference) error {
	seen := map[QualityPreferenceType]string{}
	for _, p := range values {
		if err := p.Validate(); err != nil {
			return err
		}
		if prior, ok := seen[p.Type]; ok && prior != p.Value {
			return errors.New("contradictory quality preferences for one type")
		}
		seen[p.Type] = p.Value
	}
	return nil
}

func ValidateQualityResult(result QualityResult) error {
	switch result {
	case QualityMet, QualityPartial, QualityNotAvailable, QualitySubstitute:
		return nil
	default:
		return errors.New("invalid quality result")
	}
}
