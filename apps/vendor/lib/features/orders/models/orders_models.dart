// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostCommerceOrdersVendorOrderIdAcceptRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdAcceptRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdAcceptRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdCancelRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdCancelRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdCancelRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdCompleteRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdCompleteRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdCompleteRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdDeliverRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdDeliverRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdDeliverRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdDeliveredRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdDeliveredRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdDeliveredRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdDeliveringRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdDeliveringRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdDeliveringRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdHandoverRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdHandoverRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdHandoverRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdLinesLineIdActualQuantityRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdLinesLineIdActualQuantityRequest({int? actualQuantity}) : super({'actual_quantity': ?actualQuantity});
  PostCommerceOrdersVendorOrderIdLinesLineIdActualQuantityRequest.fromJson(super.json);
  int? get actualQuantity => value('actual_quantity') as int?;
}

class PostCommerceOrdersVendorOrderIdLinesLineIdAvailabilityRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdLinesLineIdAvailabilityRequest({int? pickedQuantity, int? unavailableQuantity, int? actualMeasureQuantity, String? outcome}) : super({'picked_quantity': ?pickedQuantity, 'unavailable_quantity': ?unavailableQuantity, 'actual_measure_quantity': ?actualMeasureQuantity, 'outcome': ?outcome});
  PostCommerceOrdersVendorOrderIdLinesLineIdAvailabilityRequest.fromJson(super.json);
  int? get pickedQuantity => value('picked_quantity') as int?;
  int? get unavailableQuantity => value('unavailable_quantity') as int?;
  int? get actualMeasureQuantity => value('actual_measure_quantity') as int?;
  String? get outcome => value('outcome') as String?;
}

class PostCommerceOrdersVendorOrderIdLinesLineIdQualityRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdLinesLineIdQualityRequest({String? result, String? observedValue, String? reason}) : super({'result': ?result, 'observed_value': ?observedValue, 'reason': ?reason});
  PostCommerceOrdersVendorOrderIdLinesLineIdQualityRequest.fromJson(super.json);
  String? get result => value('result') as String?;
  String? get observedValue => value('observed_value') as String?;
  String? get reason => value('reason') as String?;
}

class PostCommerceOrdersVendorOrderIdLinesLineIdSubstitutionsRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdLinesLineIdSubstitutionsRequest({String? proposedItemRef, String? proposedVariantRef, String? proposedCatalogueRef, Object? replacementSnapshot, int? proposedQuantity, int? proposedPriceMinor, String? reason, String? expiresAt}) : super({'proposed_item_ref': ?proposedItemRef, 'proposed_variant_ref': ?proposedVariantRef, 'proposed_catalogue_ref': ?proposedCatalogueRef, 'replacement_snapshot': ?replacementSnapshot, 'proposed_quantity': ?proposedQuantity, 'proposed_price_minor': ?proposedPriceMinor, 'reason': ?reason, 'expires_at': ?expiresAt});
  PostCommerceOrdersVendorOrderIdLinesLineIdSubstitutionsRequest.fromJson(super.json);
  String? get proposedItemRef => value('proposed_item_ref') as String?;
  String? get proposedVariantRef => value('proposed_variant_ref') as String?;
  String? get proposedCatalogueRef => value('proposed_catalogue_ref') as String?;
  Object? get replacementSnapshot => value('replacement_snapshot');
  int? get proposedQuantity => value('proposed_quantity') as int?;
  int? get proposedPriceMinor => value('proposed_price_minor') as int?;
  String? get reason => value('reason') as String?;
  String? get expiresAt => value('expires_at') as String?;
}

class PostCommerceOrdersVendorOrderIdPackRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdPackRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdPackRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdPickingRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdPickingRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdPickingRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdPickupRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdPickupRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdPickupRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdPreparingRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdPreparingRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdPreparingRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdReadyRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdReadyRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdReadyRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdReadyForPickupRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdReadyForPickupRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdReadyForPickupRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdReadyForPickupVariant94fe09Request extends VendorPayload {
  PostCommerceOrdersVendorOrderIdReadyForPickupVariant94fe09Request({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdReadyForPickupVariant94fe09Request.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdRejectRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdRejectRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdRejectRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdStartPickingRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdStartPickingRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdStartPickingRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class PostCommerceOrdersVendorOrderIdStartPreparingRequest extends VendorPayload {
  PostCommerceOrdersVendorOrderIdStartPreparingRequest({String? reasonCode}) : super({'reason_code': ?reasonCode});
  PostCommerceOrdersVendorOrderIdStartPreparingRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class OrdersSection17eParentVendorOrdersAllocation extends VendorPayload {
  OrdersSection17eParentVendorOrdersAllocation.fromJson(super.json);
  String? get allocationType => value('allocation_type') as String?;
  int? get amountMinor => value('amount_minor') as int?;
  String? get beneficiaryRef => value('beneficiary_ref') as String?;
  String? get beneficiaryType => value('beneficiary_type') as String?;
  String? get fundingSource => value('funding_source') as String?;
  String? get id => value('id') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get vendorOrderId => value('vendor_order_id') as String?;
}

class OrdersSection17eParentVendorOrdersCreateOrderRequest extends VendorPayload {
  OrdersSection17eParentVendorOrdersCreateOrderRequest.fromJson(super.json);
  String? get quoteFingerprint => value('quote_fingerprint') as String?;
}

class OrdersSection17eParentVendorOrdersDeliveryETA extends VendorPayload {
  OrdersSection17eParentVendorOrdersDeliveryETA.fromJson(super.json);
  String? get actualAt => value('actual_at') as String?;
  List<dynamic>? get components => value('components') as List<dynamic>?;
  String? get confidence => value('confidence') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get delayState => value('delay_state') as String?;
  String? get earliestAt => value('earliest_at') as String?;
  String? get etaId => value('eta_id') as String?;
  int? get etaVersion => value('eta_version') as int?;
  String? get fulfilmentGroupRef => value('fulfilment_group_ref') as String?;
  int? get fulfilmentPlanVersion => value('fulfilment_plan_version') as int?;
  String? get latestAt => value('latest_at') as String?;
  String? get missionRef => value('mission_ref') as String?;
  String? get parentOrderRef => value('parent_order_ref') as String?;
  String? get policyVersion => value('policy_version') as String?;
  String? get promisedLatestAt => value('promised_latest_at') as String?;
  String? get sourceAuthority => value('source_authority') as String?;
  String? get sourceOccurredAt => value('source_occurred_at') as String?;
  int? get sourceVersion => value('source_version') as int?;
  String? get status => value('status') as String?;
  String? get trigger => value('trigger') as String?;
}

class OrdersSection17eParentVendorOrdersDeliveryETAComponent extends VendorPayload {
  OrdersSection17eParentVendorOrdersDeliveryETAComponent.fromJson(super.json);
  int? get earliestSeconds => value('earliest_seconds') as int?;
  String? get evidenceAt => value('evidence_at') as String?;
  String? get evidenceRef => value('evidence_ref') as String?;
  String? get kind => value('kind') as String?;
  int? get latestSeconds => value('latest_seconds') as int?;
  String? get source => value('source') as String?;
  int? get sourceVersion => value('source_version') as int?;
}

class OrdersSection17eParentVendorOrdersOrderLine extends VendorPayload {
  OrdersSection17eParentVendorOrdersOrderLine.fromJson(super.json);
  String? get cartItemId => value('cart_item_id') as String?;
  int? get catalogueRevision => value('catalogue_revision') as int?;
  JsonObject? get deliveryCommitment => value('delivery_commitment') == null ? null : jsonObject(value('delivery_commitment'));
  int? get discountAllocatedMinor => value('discount_allocated_minor') as int?;
  int? get expectedMeasureQuantity => value('expected_measure_quantity') as int?;
  int? get feeAllocatedMinor => value('fee_allocated_minor') as int?;
  String? get id => value('id') as String?;
  String? get inventoryItemRef => value('inventory_item_ref') as String?;
  int? get inventoryRevision => value('inventory_revision') as int?;
  String? get itemName => value('item_name') as String?;
  String? get itemRef => value('item_ref') as String?;
  String? get listingRef => value('listing_ref') as String?;
  int? get maxAuthorizedTotalMinor => value('max_authorized_total_minor') as int?;
  int? get measureOverfillBps => value('measure_overfill_bps') as int?;
  int? get measureUnderfillBps => value('measure_underfill_bps') as int?;
  String? get onlinePriceDisclosure => value('online_price_disclosure') as String?;
  String? get priceReference => value('price_reference') as String?;
  String? get pricingBasis => value('pricing_basis') as String?;
  int? get quantity => value('quantity') as int?;
  String? get quoteLineRef => value('quote_line_ref') as String?;
  int? get quotedTotalMinor => value('quoted_total_minor') as int?;
  String? get requestedMeasureRef => value('requested_measure_ref') as String?;
  String? get sellUnitRef => value('sell_unit_ref') as String?;
  bool? get storePriceParityClaimed => value('store_price_parity_claimed') as bool?;
  int? get taxAllocatedMinor => value('tax_allocated_minor') as int?;
  int? get unitPriceMinor => value('unit_price_minor') as int?;
  String? get variantRef => value('variant_ref') as String?;
}

class OrdersSection17eParentVendorOrdersParentOrder extends VendorPayload {
  OrdersSection17eParentVendorOrdersParentOrder.fromJson(super.json);
  JsonObject? get addressSnapshot => value('address_snapshot') == null ? null : jsonObject(value('address_snapshot'));
  String? get cartRef => value('cart_ref') as String?;
  int? get cartVersion => value('cart_version') as int?;
  String? get checkoutSessionId => value('checkout_session_id') as String?;
  int? get checkoutValidationRevision => value('checkout_validation_revision') as int?;
  String? get createdAt => value('created_at') as String?;
  String? get currency => value('currency') as String?;
  int? get customerApprovedTotalMinor => value('customer_approved_total_minor') as int?;
  int? get deliverySubtotalMinor => value('delivery_subtotal_minor') as int?;
  int? get discountTotalMinor => value('discount_total_minor') as int?;
  String? get domain => value('domain') as String?;
  int? get feeSubtotalMinor => value('fee_subtotal_minor') as int?;
  String? get fulfilmentPlanRef => value('fulfilment_plan_ref') as String?;
  int? get fulfilmentPlanRevision => value('fulfilment_plan_revision') as int?;
  int? get fulfilmentPlanVersion => value('fulfilment_plan_version') as int?;
  JsonObject? get fundingBreakdown => value('funding_breakdown') == null ? null : jsonObject(value('funding_breakdown'));
  bool? get hasReceiver => value('has_receiver') as bool?;
  int? get maxAuthorizedTotalMinor => value('max_authorized_total_minor') as int?;
  String? get parentOrderId => value('parent_order_id') as String?;
  String? get paymentOperationRef => value('payment_operation_ref') as String?;
  List<dynamic>? get platformAllocations => value('platform_allocations') as List<dynamic>?;
  int? get platformFeeTaxMinor => value('platform_fee_tax_minor') as int?;
  JsonObject? get priceDisclosureSnapshot => value('price_disclosure_snapshot') == null ? null : jsonObject(value('price_disclosure_snapshot'));
  int? get productSubtotalMinor => value('product_subtotal_minor') as int?;
  String? get quoteFingerprint => value('quote_fingerprint') as String?;
  String? get quoteRef => value('quote_ref') as String?;
  int? get quoteVersion => value('quote_version') as int?;
  JsonObject? get receiverSnapshot => value('receiver_snapshot') == null ? null : jsonObject(value('receiver_snapshot'));
  int? get refundPreparedMinor => value('refund_prepared_minor') as int?;
  String? get selectedFulfilmentMode => value('selected_fulfilment_mode') as String?;
  int? get settlementPreparedMinor => value('settlement_prepared_minor') as int?;
  String? get state => value('state') as String?;
  int? get subsidyTotalMinor => value('subsidy_total_minor') as int?;
  int? get taxSubtotalMinor => value('tax_subtotal_minor') as int?;
  String? get terminalAt => value('terminal_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  int? get vendorOrderCount => value('vendor_order_count') as int?;
  List<dynamic>? get vendorOrders => value('vendor_orders') as List<dynamic>?;
  int? get version => value('version') as int?;
}

class OrdersSection17eParentVendorOrdersProductionReadiness extends VendorPayload {
  OrdersSection17eParentVendorOrdersProductionReadiness.fromJson(super.json);
  String? get customerEtaEnd => value('customer_eta_end') as String?;
  String? get customerEtaStart => value('customer_eta_start') as String?;
  String? get customerEtaStatus => value('customer_eta_status') as String?;
  String? get expectedWindowEnd => value('expected_window_end') as String?;
  String? get expectedWindowStart => value('expected_window_start') as String?;
  String? get logisticsWindowEnd => value('logistics_window_end') as String?;
  String? get logisticsWindowStart => value('logistics_window_start') as String?;
  String? get parentOrderId => value('parent_order_id') as String?;
  String? get productionJobId => value('production_job_id') as String?;
  String? get promisedReadyAt => value('promised_ready_at') as String?;
  int? get sourceVersion => value('source_version') as int?;
  int? get timingRevision => value('timing_revision') as int?;
  JsonObject? get timingSourceEvidence => value('timing_source_evidence') == null ? null : jsonObject(value('timing_source_evidence'));
  String? get updatedAt => value('updated_at') as String?;
  String? get vendorOrderId => value('vendor_order_id') as String?;
}

class OrdersSection17eParentVendorOrdersTransitionRequest extends VendorPayload {
  OrdersSection17eParentVendorOrdersTransitionRequest.fromJson(super.json);
  String? get reasonCode => value('reason_code') as String?;
}

class OrdersSection17eParentVendorOrdersVendorOrder extends VendorPayload {
  OrdersSection17eParentVendorOrdersVendorOrder.fromJson(super.json);
  List<dynamic>? get allocations => value('allocations') as List<dynamic>?;
  String? get branchId => value('branch_id') as String?;
  String? get businessId => value('business_id') as String?;
  String? get cancelReasonCode => value('cancel_reason_code') as String?;
  String? get cancelledBy => value('cancelled_by') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get currency => value('currency') as String?;
  int? get customerPayableMinor => value('customer_payable_minor') as int?;
  int? get deliveryCustomerMinor => value('delivery_customer_minor') as int?;
  String? get deliveryFeeBeneficiary => value('delivery_fee_beneficiary') as String?;
  int? get deliveryGrossMinor => value('delivery_gross_minor') as int?;
  int? get deliveryOtherFundedMinor => value('delivery_other_funded_minor') as int?;
  int? get deliveryPlatformFundedMinor => value('delivery_platform_funded_minor') as int?;
  String? get deliveryResponsibility => value('delivery_responsibility') as String?;
  int? get deliveryVendorFundedMinor => value('delivery_vendor_funded_minor') as int?;
  String? get domain => value('domain') as String?;
  String? get fulfilmentMode => value('fulfilment_mode') as String?;
  String? get inventoryReservationRef => value('inventory_reservation_ref') as String?;
  String? get inventoryReservationSourceRef => value('inventory_reservation_source_ref') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  String? get logisticsMissionRef => value('logistics_mission_ref') as String?;
  String? get logisticsPlanGroupRef => value('logistics_plan_group_ref') as String?;
  int? get maxProductSubtotalMinor => value('max_product_subtotal_minor') as int?;
  int? get orderOtherFundedMinor => value('order_other_funded_minor') as int?;
  int? get orderPlatformFundedMinor => value('order_platform_funded_minor') as int?;
  int? get orderVendorFundedMinor => value('order_vendor_funded_minor') as int?;
  String? get parentOrderId => value('parent_order_id') as String?;
  int? get productOtherFundedMinor => value('product_other_funded_minor') as int?;
  int? get productPlatformFundedMinor => value('product_platform_funded_minor') as int?;
  int? get productSubtotalMinor => value('product_subtotal_minor') as int?;
  int? get productVendorFundedMinor => value('product_vendor_funded_minor') as int?;
  String? get quoteGroupRef => value('quote_group_ref') as String?;
  int? get refundAllocatedMinor => value('refund_allocated_minor') as int?;
  bool? get reservationRequired => value('reservation_required') as bool?;
  int? get settlementAllocatedMinor => value('settlement_allocated_minor') as int?;
  String? get status => value('status') as String?;
  String? get terminalAt => value('terminal_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get vendorOrderId => value('vendor_order_id') as String?;
  String? get vendorOwnDeliveryState => value('vendor_own_delivery_state') as String?;
  int? get vendorPayableMinor => value('vendor_payable_minor') as int?;
  int? get version => value('version') as int?;
}
