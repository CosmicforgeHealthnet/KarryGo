package supportcase

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
)

const SchemaVersion = 1

type Category string

const (
	WrongItemOrMeal          Category = "WRONG_ITEM_OR_MEAL"
	MissingItem              Category = "MISSING_ITEM"
	UnauthorizedSubstitution Category = "UNAUTHORIZED_SUBSTITUTION"
	DamagedOrSpoiltProduct   Category = "DAMAGED_OR_SPOILT_PRODUCT"
	QualityIssue             Category = "QUALITY_ISSUE"
	VendorDelay              Category = "VENDOR_DELAY"
	DeliveryDelay            Category = "DELIVERY_DELAY"
	PickupDispute            Category = "PICKUP_DISPUTE"
	ReceiverHandoverDispute  Category = "RECEIVER_HANDOVER_DISPUTE"
	RefundProblem            Category = "REFUND_PROBLEM"
	VendorOwnedDeliveryIssue Category = "VENDOR_OWNED_DELIVERY_ISSUE"
)

var categories = map[Category]struct{}{
	WrongItemOrMeal: {}, MissingItem: {}, UnauthorizedSubstitution: {},
	DamagedOrSpoiltProduct: {}, QualityIssue: {}, VendorDelay: {}, DeliveryDelay: {},
	PickupDispute: {}, ReceiverHandoverDispute: {}, RefundProblem: {}, VendorOwnedDeliveryIssue: {},
}

var sensitiveNumber = regexp.MustCompile(`\b[0-9]{4,8}\b`)

// SanitizeDescription removes credential-shaped numeric material from bounded
// free text. Case classification and proof remain typed references; prose is
// never an authority for OTP/QR/challenge evidence.
func SanitizeDescription(value string) string {
	return sensitiveNumber.ReplaceAllString(strings.TrimSpace(value), "[REDACTED]")
}

func (c Category) Valid() bool { _, ok := categories[c]; return ok }

type Context struct {
	CommerceOrderRef   string   `json:"commerce_order_ref"`
	VendorOrderRef     string   `json:"vendor_order_ref"`
	VendorRef          string   `json:"vendor_ref"`
	FulfilmentGroupRef string   `json:"fulfilment_group_ref,omitempty"`
	MissionRef         string   `json:"mission_ref,omitempty"`
	VendorMissionRef   string   `json:"vendor_mission_ref,omitempty"`
	PlatformMissionRef string   `json:"platform_mission_ref,omitempty"`
	ParcelRefs         []string `json:"parcel_refs,omitempty"`
	LineRefs           []string `json:"line_refs,omitempty"`
	EvidenceRefs       []string `json:"evidence_refs,omitempty"`
	MediaRefs          []string `json:"media_refs,omitempty"`
	RefundRef          string   `json:"refund_ref,omitempty"`
	SubstitutionRef    string   `json:"substitution_ref,omitempty"`
}

type CreateRequest struct {
	SchemaVersion int      `json:"schema_version"`
	OperationRef  string   `json:"operation_ref"`
	Category      Category `json:"category"`
	Priority      string   `json:"priority"`
	Subject       string   `json:"subject"`
	Description   string   `json:"description"`
	CustomerRef   string   `json:"customer_ref"`
	Context       Context  `json:"context"`
}

type Case struct {
	CaseRef            string    `json:"case_ref"`
	OperationRef       string    `json:"operation_ref"`
	Category           Category  `json:"category"`
	Status             string    `json:"status"`
	Resolution         string    `json:"resolution,omitempty"`
	ResolutionVersion  int64     `json:"resolution_version"`
	FinancialActionRef string    `json:"financial_action_ref,omitempty"`
	Version            int64     `json:"version"`
	UpdatedAt          time.Time `json:"updated_at"`
}

type ResolutionEvent struct {
	SchemaVersion      int       `json:"schema_version"`
	EventID            string    `json:"event_id"`
	CaseRef            string    `json:"case_ref"`
	OperationRef       string    `json:"operation_ref"`
	Category           Category  `json:"category"`
	Resolution         string    `json:"resolution"`
	ResolutionVersion  int64     `json:"resolution_version"`
	FinancialActionRef string    `json:"financial_action_ref,omitempty"`
	CustomerRef        string    `json:"customer_ref"`
	Context            Context   `json:"context"`
	ResolvedAt         time.Time `json:"resolved_at"`
}

func (r CreateRequest) Validate() error {
	if r.SchemaVersion != SchemaVersion || !r.Category.Valid() {
		return errors.New("invalid commerce support schema or category")
	}
	if !bounded(r.OperationRef, 1, 160) || !bounded(r.Subject, 1, 160) || !bounded(r.Description, 1, 1000) || !bounded(r.CustomerRef, 1, 200) {
		return errors.New("invalid commerce support request bounds")
	}
	if !bounded(r.Context.CommerceOrderRef, 1, 200) || !bounded(r.Context.VendorOrderRef, 1, 200) || !bounded(r.Context.VendorRef, 1, 200) {
		return errors.New("required commerce references are missing")
	}
	for _, refs := range [][]string{r.Context.ParcelRefs, r.Context.LineRefs, r.Context.EvidenceRefs, r.Context.MediaRefs} {
		if len(refs) > 20 {
			return errors.New("too many commerce support references")
		}
		for _, ref := range refs {
			if !bounded(ref, 1, 300) {
				return errors.New("invalid commerce support reference")
			}
		}
	}
	return nil
}

func Fingerprint(r CreateRequest) (string, error) {
	r.Priority = strings.ToLower(strings.TrimSpace(r.Priority))
	r.Subject, r.Description = strings.TrimSpace(r.Subject), strings.TrimSpace(r.Description)
	b, err := json.Marshal(r)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}

func bounded(v string, min, max int) bool {
	n := len(strings.TrimSpace(v))
	return n >= min && n <= max
}
