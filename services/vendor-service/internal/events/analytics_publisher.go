package events

import "cosmicforge/logistics/shared/go/commerceanalytics"

func AnalyticsEventTypes() []string {
	return []string{
		commerceanalytics.EventOrderCompleted,
		commerceanalytics.EventOrderAccepted,
		commerceanalytics.EventOrderRejected,
		commerceanalytics.EventOrderCancelled,
		commerceanalytics.EventOrderRefunded,
		commerceanalytics.EventItemAvailability,
		commerceanalytics.EventInventoryMovement,
		commerceanalytics.EventWasteExpiry,
		commerceanalytics.EventPurchaseReceived,
		commerceanalytics.EventSupplierDelivery,
		commerceanalytics.EventPreparationTiming,
		commerceanalytics.EventPickingTiming,
		commerceanalytics.EventFulfilmentPerformance,
		commerceanalytics.EventProductionCapacity,
		commerceanalytics.EventRFQStateChanged,
		commerceanalytics.EventRFQQuoteSubmitted,
		commerceanalytics.EventRFQAccepted,
		commerceanalytics.EventCRMRelationship,
		commerceanalytics.EventSearchDemand,
		commerceanalytics.EventSupportOutcome,
		commerceanalytics.EventSupportCaseResolved,
		commerceanalytics.EventPriceSnapshot,
		commerceanalytics.EventGuaranteeEvidence,
	}
}
