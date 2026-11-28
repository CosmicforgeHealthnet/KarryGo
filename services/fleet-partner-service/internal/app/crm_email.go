package app

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"cosmicforge/logistics/shared/go/emailbrand"

	crmmodels "cosmicforge/logistics/services/fleet-partner-service/internal/features/crm/models"
)

// crmEmail is the customer-facing content of one CRM notification. These
// emails go to an organization's own customers, so the organization is the
// sender in the copy and questions are directed to it, not to Karryam support.
type crmEmail struct {
	Title  string
	Body   string
	Layout emailbrand.Layout
}

func crmString(payload map[string]any, key string) string {
	switch v := payload[key].(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return v.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(v)
	}
}

func crmMinor(payload map[string]any) (int64, bool) {
	switch v := payload["totalAmountMinor"].(type) {
	case int64:
		return v, true
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	case string:
		n, err := strconv.ParseInt(v, 10, 64)
		return n, err == nil
	default:
		return 0, false
	}
}

// formatMinorAmount renders minor units as "NGN 1,250,000.00".
func formatMinorAmount(currency string, minor int64) string {
	sign := ""
	if minor < 0 {
		sign, minor = "-", -minor
	}
	whole := strconv.FormatInt(minor/100, 10)
	var grouped strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(r)
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s%s.%02d", strings.ToUpper(currency), sign, grouped.String(), minor%100))
}

func crmDate(raw string) string {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t.UTC().Format("2 Jan 2006")
	}
	return raw
}

func buildCRMEmail(intentType string, payload map[string]any, contactName, organization string) crmEmail {
	organization = strings.TrimSpace(organization)
	if organization == "" {
		organization = "Your logistics provider"
	}
	greeting := "Hello,"
	if fields := strings.Fields(contactName); len(fields) > 0 {
		greeting = "Hello " + fields[0] + ","
	}
	layout := emailbrand.Layout{Product: "Karryam Fleet", Greeting: greeting,
		Outro: []string{fmt.Sprintf("If you have any questions, please contact %s directly.", organization)}}
	quotation := crmString(payload, "quotationNumber")
	contract := crmString(payload, "contractNumber")
	rows := func(pairs ...string) *emailbrand.Card {
		card := &emailbrand.Card{Title: "Details"}
		for i := 0; i+1 < len(pairs); i += 2 {
			if pairs[i+1] != "" {
				card.Rows = append(card.Rows, emailbrand.Row{Label: pairs[i], Value: pairs[i+1]})
			}
		}
		return card
	}
	set := func(eyebrow, heading, tone, status, intro string) {
		layout.Eyebrow, layout.Heading, layout.Intro = eyebrow, heading, []string{intro}
		layout.Status = &emailbrand.Status{Tone: tone, Label: status}
	}

	switch intentType {
	case crmmodels.IntentQuotationIssued:
		set("Quotation", "Your quotation from "+organization, emailbrand.ToneInfo, "Awaiting your response",
			fmt.Sprintf("%s has prepared quotation %s for you. Please review the summary below.", organization, quotation))
		layout.Details = rows("Quotation", quotation, "Valid until", crmDate(crmString(payload, "validUntil")))
		layout.Details.Title = "Quotation summary"
		if minor, ok := crmMinor(payload); ok {
			layout.Details.Total = &emailbrand.Row{Label: "Total", Value: formatMinorAmount(crmString(payload, "currency"), minor)}
		}
	case crmmodels.IntentQuotationAccepted:
		set("Quotation", "Quotation accepted", emailbrand.ToneSuccess, "Accepted",
			fmt.Sprintf("Thank you. %s has recorded your acceptance of quotation %s.", organization, quotation))
		layout.Details = rows("Quotation", quotation, "Provider", organization)
	case crmmodels.IntentQuotationRejected:
		set("Quotation", "Quotation declined", emailbrand.ToneNeutral, "Declined",
			fmt.Sprintf("Quotation %s from %s has been marked as declined.", quotation, organization))
		layout.Details = rows("Quotation", quotation, "Provider", organization)
	case crmmodels.IntentQuotationExpired:
		set("Quotation", "Your quotation has expired", emailbrand.ToneWarning, "Expired",
			fmt.Sprintf("Quotation %s from %s is no longer valid. Contact %s if you would like an updated quotation.", quotation, organization, organization))
		layout.Details = rows("Quotation", quotation, "Provider", organization)
	case crmmodels.IntentContractActivated:
		set("Contract", "Your contract is active", emailbrand.ToneSuccess, "Active",
			fmt.Sprintf("Your contract %s with %s is now active.", contract, organization))
		layout.Details = rows("Contract", contract, "Provider", organization)
	case crmmodels.IntentContractSuspended:
		set("Contract", "Your contract has been suspended", emailbrand.ToneWarning, "Suspended",
			fmt.Sprintf("%s has suspended contract %s. Services under this contract may be paused until it is reinstated.", organization, contract))
		layout.Details = rows("Contract", contract, "Provider", organization)
	case crmmodels.IntentContractTerminated:
		set("Contract", "Your contract has ended", emailbrand.ToneDanger, "Terminated",
			fmt.Sprintf("%s has terminated contract %s.", organization, contract))
		layout.Details = rows("Contract", contract, "Provider", organization)
	case crmmodels.IntentContractExpired:
		set("Contract", "Your contract has expired", emailbrand.ToneNeutral, "Expired",
			fmt.Sprintf("Contract %s with %s has reached its end date.", contract, organization))
		layout.Details = rows("Contract", contract, "Provider", organization)
	case crmmodels.IntentContractRenewalDue:
		set("Contract", "Your contract is due for renewal", emailbrand.ToneInfo, "Renewal due",
			fmt.Sprintf("Contract %s with %s is approaching its renewal date. Contact %s to review and renew it.", contract, organization, organization))
		layout.Details = rows("Contract", contract, "Provider", organization)
	case crmmodels.IntentSLAEscalation:
		set("Service level", "A service level issue has been escalated", emailbrand.ToneWarning, "Escalated",
			fmt.Sprintf("%s has escalated a service level measurement that did not meet its agreed target and is following up.", organization))
		layout.Details = rows("Measure", crmString(payload, "metricCode"), "Outcome", strings.ReplaceAll(strings.ToLower(crmString(payload, "outcome")), "_", " "))
	case crmmodels.IntentCreditHold:
		set("Account", "Your credit account is on hold", emailbrand.ToneWarning, "On hold",
			fmt.Sprintf("%s has placed your credit account on hold. Orders on credit may be paused until the hold is released.", organization))
		layout.Details = rows("Provider", organization)
	case crmmodels.IntentCreditHoldReleased:
		set("Account", "Your credit hold has been released", emailbrand.ToneSuccess, "Released",
			fmt.Sprintf("%s has released the hold on your credit account.", organization))
		layout.Details = rows("Provider", organization)
	default:
		set("Update", "An update from "+organization, emailbrand.ToneInfo, "Update",
			fmt.Sprintf("%s has an update about your account.", organization))
	}
	if layout.Details != nil && len(layout.Details.Rows) == 0 && layout.Details.Total == nil {
		layout.Details = nil
	}
	layout.Preheader = layout.Intro[0]
	return crmEmail{Title: layout.Heading, Body: crmPlainBody(layout), Layout: layout}
}

// crmPlainBody is the stored plain-text body: notification-service requires
// one, and it is what any non-HTML channel shows.
func crmPlainBody(layout emailbrand.Layout) string {
	parts := append([]string{}, layout.Intro...)
	if layout.Details != nil {
		for _, row := range layout.Details.Rows {
			parts = append(parts, row.Label+": "+row.Value)
		}
		if layout.Details.Total != nil {
			parts = append(parts, layout.Details.Total.Label+": "+layout.Details.Total.Value)
		}
	}
	parts = append(parts, layout.Outro...)
	return strings.Join(parts, "\n")
}
