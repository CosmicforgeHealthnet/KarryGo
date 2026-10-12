// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostPurchasingAlertsAlertIdAcknowledgeRequest extends VendorPayload {
  PostPurchasingAlertsAlertIdAcknowledgeRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPurchasingAlertsAlertIdAcknowledgeRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingAlertsAlertIdResolveRequest extends VendorPayload {
  PostPurchasingAlertsAlertIdResolveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPurchasingAlertsAlertIdResolveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutPurchasingApprovalPolicyRequest extends VendorPayload {
  PutPurchasingApprovalPolicyRequest({
    String? name,
    bool? allowSelfApproval,
    int? expectedVersion,
    List<dynamic>? rules,
  }) : super({
         'name': ?name,
         'allow_self_approval': ?allowSelfApproval,
         'expected_version': ?expectedVersion,
         'rules': ?rules,
       });
  PutPurchasingApprovalPolicyRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  bool? get allowSelfApproval => value('allow_self_approval') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
  List<dynamic>? get rules => value('rules') as List<dynamic>?;
}

class PostPurchasingGoodsReceiptsRequest extends VendorPayload {
  PostPurchasingGoodsReceiptsRequest({
    String? purchaseOrderId,
    String? supplierDeliveryReference,
    String? invoiceReference,
    int? invoiceTotalMinor,
    String? receivedAt,
    String? notes,
  }) : super({
         'purchase_order_id': ?purchaseOrderId,
         'supplier_delivery_reference': ?supplierDeliveryReference,
         'invoice_reference': ?invoiceReference,
         'invoice_total_minor': ?invoiceTotalMinor,
         'received_at': ?receivedAt,
         'notes': ?notes,
       });
  PostPurchasingGoodsReceiptsRequest.fromJson(super.json);
  String? get purchaseOrderId => value('purchase_order_id') as String?;
  String? get supplierDeliveryReference =>
      value('supplier_delivery_reference') as String?;
  String? get invoiceReference => value('invoice_reference') as String?;
  int? get invoiceTotalMinor => value('invoice_total_minor') as int?;
  String? get receivedAt => value('received_at') as String?;
  String? get notes => value('notes') as String?;
}

class PostPurchasingGoodsReceiptsReceiptIdConfirmRequest extends VendorPayload {
  PostPurchasingGoodsReceiptsReceiptIdConfirmRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPurchasingGoodsReceiptsReceiptIdConfirmRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingGoodsReceiptsReceiptIdDocumentsRequest
    extends VendorPayload {
  PostPurchasingGoodsReceiptsReceiptIdDocumentsRequest({
    String? documentType,
    String? mediaRef,
    String? externalReference,
  }) : super({
         'document_type': ?documentType,
         'media_ref': ?mediaRef,
         'external_reference': ?externalReference,
       });
  PostPurchasingGoodsReceiptsReceiptIdDocumentsRequest.fromJson(super.json);
  String? get documentType => value('document_type') as String?;
  String? get mediaRef => value('media_ref') as String?;
  String? get externalReference => value('external_reference') as String?;
}

class PutPurchasingGoodsReceiptsReceiptIdInspectionRequest
    extends VendorPayload {
  PutPurchasingGoodsReceiptsReceiptIdInspectionRequest({List<dynamic>? lines})
    : super({'lines': ?lines});
  PutPurchasingGoodsReceiptsReceiptIdInspectionRequest.fromJson(super.json);
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class PostPurchasingGoodsReceiptsReceiptIdReverseRequest extends VendorPayload {
  PostPurchasingGoodsReceiptsReceiptIdReverseRequest({String? reason})
    : super({'reason': ?reason});
  PostPurchasingGoodsReceiptsReceiptIdReverseRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
}

class PostPurchasingPurchaseOrdersRequest extends VendorPayload {
  PostPurchasingPurchaseOrdersRequest({
    String? branchId,
    String? supplierId,
    String? currencyCode,
    String? expectedDeliveryDate,
    String? notes,
    List<dynamic>? lines,
  }) : super({
         'branch_id': ?branchId,
         'supplier_id': ?supplierId,
         'currency_code': ?currencyCode,
         'expected_delivery_date': ?expectedDeliveryDate,
         'notes': ?notes,
         'lines': ?lines,
       });
  PostPurchasingPurchaseOrdersRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get supplierId => value('supplier_id') as String?;
  String? get currencyCode => value('currency_code') as String?;
  String? get expectedDeliveryDate =>
      value('expected_delivery_date') as String?;
  String? get notes => value('notes') as String?;
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
}

class PatchPurchasingPurchaseOrdersPurchaseOrderIdRequest
    extends VendorPayload {
  PatchPurchasingPurchaseOrdersPurchaseOrderIdRequest({
    String? expectedDeliveryDate,
    String? notes,
    int? expectedVersion,
  }) : super({
         'expected_delivery_date': ?expectedDeliveryDate,
         'notes': ?notes,
         'expected_version': ?expectedVersion,
       });
  PatchPurchasingPurchaseOrdersPurchaseOrderIdRequest.fromJson(super.json);
  String? get expectedDeliveryDate =>
      value('expected_delivery_date') as String?;
  String? get notes => value('notes') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingPurchaseOrdersPurchaseOrderIdApproveRequest
    extends VendorPayload {
  PostPurchasingPurchaseOrdersPurchaseOrderIdApproveRequest({
    int? expectedVersion,
    String? stepUpGrantId,
    String? note,
  }) : super({
         'expected_version': ?expectedVersion,
         'step_up_grant_id': ?stepUpGrantId,
         'note': ?note,
       });
  PostPurchasingPurchaseOrdersPurchaseOrderIdApproveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
  String? get note => value('note') as String?;
}

class PostPurchasingPurchaseOrdersPurchaseOrderIdCancelRequest
    extends VendorPayload {
  PostPurchasingPurchaseOrdersPurchaseOrderIdCancelRequest({
    int? expectedVersion,
    String? note,
  }) : super({'expected_version': ?expectedVersion, 'note': ?note});
  PostPurchasingPurchaseOrdersPurchaseOrderIdCancelRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get note => value('note') as String?;
}

class PostPurchasingPurchaseOrdersPurchaseOrderIdCloseRequest
    extends VendorPayload {
  PostPurchasingPurchaseOrdersPurchaseOrderIdCloseRequest({
    int? expectedVersion,
    String? note,
  }) : super({'expected_version': ?expectedVersion, 'note': ?note});
  PostPurchasingPurchaseOrdersPurchaseOrderIdCloseRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get note => value('note') as String?;
}

class PutPurchasingPurchaseOrdersPurchaseOrderIdLinesRequest
    extends VendorPayload {
  PutPurchasingPurchaseOrdersPurchaseOrderIdLinesRequest({
    List<dynamic>? lines,
    int? expectedVersion,
  }) : super({'lines': ?lines, 'expected_version': ?expectedVersion});
  PutPurchasingPurchaseOrdersPurchaseOrderIdLinesRequest.fromJson(super.json);
  List<dynamic>? get lines => value('lines') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingPurchaseOrdersPurchaseOrderIdOrderRequest
    extends VendorPayload {
  PostPurchasingPurchaseOrdersPurchaseOrderIdOrderRequest({
    int? expectedVersion,
    String? note,
  }) : super({'expected_version': ?expectedVersion, 'note': ?note});
  PostPurchasingPurchaseOrdersPurchaseOrderIdOrderRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get note => value('note') as String?;
}

class PostPurchasingPurchaseOrdersPurchaseOrderIdRejectRequest
    extends VendorPayload {
  PostPurchasingPurchaseOrdersPurchaseOrderIdRejectRequest({
    int? expectedVersion,
    String? stepUpGrantId,
    String? note,
  }) : super({
         'expected_version': ?expectedVersion,
         'step_up_grant_id': ?stepUpGrantId,
         'note': ?note,
       });
  PostPurchasingPurchaseOrdersPurchaseOrderIdRejectRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
  String? get note => value('note') as String?;
}

class PostPurchasingPurchaseOrdersPurchaseOrderIdSubmitRequest
    extends VendorPayload {
  PostPurchasingPurchaseOrdersPurchaseOrderIdSubmitRequest({
    int? expectedVersion,
    String? note,
  }) : super({'expected_version': ?expectedVersion, 'note': ?note});
  PostPurchasingPurchaseOrdersPurchaseOrderIdSubmitRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
  String? get note => value('note') as String?;
}

class PutPurchasingReceivingPoliciesBranchesBranchIdRequest
    extends VendorPayload {
  PutPurchasingReceivingPoliciesBranchesBranchIdRequest({
    String? overDeliveryMode,
    int? overDeliveryToleranceBps,
    int? costVarianceApprovalBps,
    int? costVarianceApprovalMinor,
    bool? requireCostVarianceApproval,
  }) : super({
         'over_delivery_mode': ?overDeliveryMode,
         'over_delivery_tolerance_bps': ?overDeliveryToleranceBps,
         'cost_variance_approval_bps': ?costVarianceApprovalBps,
         'cost_variance_approval_minor': ?costVarianceApprovalMinor,
         'require_cost_variance_approval': ?requireCostVarianceApproval,
       });
  PutPurchasingReceivingPoliciesBranchesBranchIdRequest.fromJson(super.json);
  String? get overDeliveryMode => value('over_delivery_mode') as String?;
  int? get overDeliveryToleranceBps =>
      value('over_delivery_tolerance_bps') as int?;
  int? get costVarianceApprovalBps =>
      value('cost_variance_approval_bps') as int?;
  int? get costVarianceApprovalMinor =>
      value('cost_variance_approval_minor') as int?;
  bool? get requireCostVarianceApproval =>
      value('require_cost_variance_approval') as bool?;
}

class PostPurchasingSupplierCategoriesRequest extends VendorPayload {
  PostPurchasingSupplierCategoriesRequest({
    String? code,
    String? name,
    String? description,
  }) : super({'code': ?code, 'name': ?name, 'description': ?description});
  PostPurchasingSupplierCategoriesRequest.fromJson(super.json);
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
}

class PatchPurchasingSupplierCategoriesCategoryIdRequest extends VendorPayload {
  PatchPurchasingSupplierCategoriesCategoryIdRequest({
    String? code,
    String? name,
    String? description,
    int? expectedVersion,
  }) : super({
         'code': ?code,
         'name': ?name,
         'description': ?description,
         'expected_version': ?expectedVersion,
       });
  PatchPurchasingSupplierCategoriesCategoryIdRequest.fromJson(super.json);
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingSupplierCategoriesCategoryIdArchiveRequest
    extends VendorPayload {
  PostPurchasingSupplierCategoriesCategoryIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostPurchasingSupplierCategoriesCategoryIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingSupplierDiscrepanciesDiscrepancyIdApproveRequest
    extends VendorPayload {
  PostPurchasingSupplierDiscrepanciesDiscrepancyIdApproveRequest({
    String? notes,
  }) : super({'notes': ?notes});
  PostPurchasingSupplierDiscrepanciesDiscrepancyIdApproveRequest.fromJson(
    super.json,
  );
  String? get notes => value('notes') as String?;
}

class PostPurchasingSupplierDiscrepanciesDiscrepancyIdRejectRequest
    extends VendorPayload {
  PostPurchasingSupplierDiscrepanciesDiscrepancyIdRejectRequest({String? notes})
    : super({'notes': ?notes});
  PostPurchasingSupplierDiscrepanciesDiscrepancyIdRejectRequest.fromJson(
    super.json,
  );
  String? get notes => value('notes') as String?;
}

class PostPurchasingSupplierDiscrepanciesDiscrepancyIdResolveRequest
    extends VendorPayload {
  PostPurchasingSupplierDiscrepanciesDiscrepancyIdResolveRequest({
    String? notes,
  }) : super({'notes': ?notes});
  PostPurchasingSupplierDiscrepanciesDiscrepancyIdResolveRequest.fromJson(
    super.json,
  );
  String? get notes => value('notes') as String?;
}

class PostPurchasingSuppliersRequest extends VendorPayload {
  PostPurchasingSuppliersRequest({
    String? categoryId,
    String? code,
    String? name,
    String? legalName,
    int? defaultLeadTimeHours,
    String? status,
    String? notes,
  }) : super({
         'category_id': ?categoryId,
         'code': ?code,
         'name': ?name,
         'legal_name': ?legalName,
         'default_lead_time_hours': ?defaultLeadTimeHours,
         'status': ?status,
         'notes': ?notes,
       });
  PostPurchasingSuppliersRequest.fromJson(super.json);
  String? get categoryId => value('category_id') as String?;
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get legalName => value('legal_name') as String?;
  int? get defaultLeadTimeHours => value('default_lead_time_hours') as int?;
  String? get status => value('status') as String?;
  String? get notes => value('notes') as String?;
}

class PatchPurchasingSuppliersSupplierIdRequest extends VendorPayload {
  PatchPurchasingSuppliersSupplierIdRequest({
    String? categoryId,
    String? code,
    String? name,
    String? legalName,
    int? defaultLeadTimeHours,
    String? status,
    String? notes,
    int? expectedVersion,
  }) : super({
         'category_id': ?categoryId,
         'code': ?code,
         'name': ?name,
         'legal_name': ?legalName,
         'default_lead_time_hours': ?defaultLeadTimeHours,
         'status': ?status,
         'notes': ?notes,
         'expected_version': ?expectedVersion,
       });
  PatchPurchasingSuppliersSupplierIdRequest.fromJson(super.json);
  String? get categoryId => value('category_id') as String?;
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get legalName => value('legal_name') as String?;
  int? get defaultLeadTimeHours => value('default_lead_time_hours') as int?;
  String? get status => value('status') as String?;
  String? get notes => value('notes') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingSuppliersSupplierIdArchiveRequest extends VendorPayload {
  PostPurchasingSuppliersSupplierIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostPurchasingSuppliersSupplierIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingSuppliersSupplierIdContactsRequest extends VendorPayload {
  PostPurchasingSuppliersSupplierIdContactsRequest({
    String? contactRole,
    String? name,
    String? email,
    String? phone,
    bool? isPrimary,
  }) : super({
         'contact_role': ?contactRole,
         'name': ?name,
         'email': ?email,
         'phone': ?phone,
         'is_primary': ?isPrimary,
       });
  PostPurchasingSuppliersSupplierIdContactsRequest.fromJson(super.json);
  String? get contactRole => value('contact_role') as String?;
  String? get name => value('name') as String?;
  String? get email => value('email') as String?;
  String? get phone => value('phone') as String?;
  bool? get isPrimary => value('is_primary') as bool?;
}

class PatchPurchasingSuppliersSupplierIdContactsContactIdRequest
    extends VendorPayload {
  PatchPurchasingSuppliersSupplierIdContactsContactIdRequest({
    String? contactRole,
    String? name,
    String? email,
    String? phone,
    bool? isPrimary,
    int? expectedVersion,
  }) : super({
         'contact_role': ?contactRole,
         'name': ?name,
         'email': ?email,
         'phone': ?phone,
         'is_primary': ?isPrimary,
         'expected_version': ?expectedVersion,
       });
  PatchPurchasingSuppliersSupplierIdContactsContactIdRequest.fromJson(
    super.json,
  );
  String? get contactRole => value('contact_role') as String?;
  String? get name => value('name') as String?;
  String? get email => value('email') as String?;
  String? get phone => value('phone') as String?;
  bool? get isPrimary => value('is_primary') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingSuppliersSupplierIdContactsContactIdArchiveRequest
    extends VendorPayload {
  PostPurchasingSuppliersSupplierIdContactsContactIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostPurchasingSuppliersSupplierIdContactsContactIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingSuppliersSupplierIdProductsRequest extends VendorPayload {
  PostPurchasingSuppliersSupplierIdProductsRequest({
    String? branchId,
    String? inventoryItemId,
    String? supplierSku,
    String? purchaseUnitCode,
    String? purchaseUnitName,
    String? conversionMode,
    int? inventoryUnitsPerPurchaseUnit,
    int? minimumOrderQuantity,
    int? purchaseMultiple,
    int? typicalCostMinor,
    String? currencyCode,
    int? leadTimeHours,
  }) : super({
         'branch_id': ?branchId,
         'inventory_item_id': ?inventoryItemId,
         'supplier_sku': ?supplierSku,
         'purchase_unit_code': ?purchaseUnitCode,
         'purchase_unit_name': ?purchaseUnitName,
         'conversion_mode': ?conversionMode,
         'inventory_units_per_purchase_unit': ?inventoryUnitsPerPurchaseUnit,
         'minimum_order_quantity': ?minimumOrderQuantity,
         'purchase_multiple': ?purchaseMultiple,
         'typical_cost_minor': ?typicalCostMinor,
         'currency_code': ?currencyCode,
         'lead_time_hours': ?leadTimeHours,
       });
  PostPurchasingSuppliersSupplierIdProductsRequest.fromJson(super.json);
  String? get branchId => value('branch_id') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
  String? get supplierSku => value('supplier_sku') as String?;
  String? get purchaseUnitCode => value('purchase_unit_code') as String?;
  String? get purchaseUnitName => value('purchase_unit_name') as String?;
  String? get conversionMode => value('conversion_mode') as String?;
  int? get inventoryUnitsPerPurchaseUnit =>
      value('inventory_units_per_purchase_unit') as int?;
  int? get minimumOrderQuantity => value('minimum_order_quantity') as int?;
  int? get purchaseMultiple => value('purchase_multiple') as int?;
  int? get typicalCostMinor => value('typical_cost_minor') as int?;
  String? get currencyCode => value('currency_code') as String?;
  int? get leadTimeHours => value('lead_time_hours') as int?;
}

class PatchPurchasingSuppliersSupplierIdProductsSupplierProductIdRequest
    extends VendorPayload {
  PatchPurchasingSuppliersSupplierIdProductsSupplierProductIdRequest({
    String? supplierSku,
    int? minimumOrderQuantity,
    int? purchaseMultiple,
    int? typicalCostMinor,
    String? currencyCode,
    int? leadTimeHours,
    int? expectedVersion,
  }) : super({
         'supplier_sku': ?supplierSku,
         'minimum_order_quantity': ?minimumOrderQuantity,
         'purchase_multiple': ?purchaseMultiple,
         'typical_cost_minor': ?typicalCostMinor,
         'currency_code': ?currencyCode,
         'lead_time_hours': ?leadTimeHours,
         'expected_version': ?expectedVersion,
       });
  PatchPurchasingSuppliersSupplierIdProductsSupplierProductIdRequest.fromJson(
    super.json,
  );
  String? get supplierSku => value('supplier_sku') as String?;
  int? get minimumOrderQuantity => value('minimum_order_quantity') as int?;
  int? get purchaseMultiple => value('purchase_multiple') as int?;
  int? get typicalCostMinor => value('typical_cost_minor') as int?;
  String? get currencyCode => value('currency_code') as String?;
  int? get leadTimeHours => value('lead_time_hours') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostPurchasingSuppliersSupplierIdProductsSupplierProductIdArchiveRequest
    extends VendorPayload {
  PostPurchasingSuppliersSupplierIdProductsSupplierProductIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostPurchasingSuppliersSupplierIdProductsSupplierProductIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PutPurchasingSuppliersSupplierIdTermsRequest extends VendorPayload {
  PutPurchasingSuppliersSupplierIdTermsRequest({
    int? paymentTermsDays,
    String? currencyCode,
    String? deliveryTerms,
    int? expectedVersion,
  }) : super({
         'payment_terms_days': ?paymentTermsDays,
         'currency_code': ?currencyCode,
         'delivery_terms': ?deliveryTerms,
         'expected_version': ?expectedVersion,
       });
  PutPurchasingSuppliersSupplierIdTermsRequest.fromJson(super.json);
  int? get paymentTermsDays => value('payment_terms_days') as int?;
  String? get currencyCode => value('currency_code') as String?;
  String? get deliveryTerms => value('delivery_terms') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PurchasingSection15VendorPurchasingErrorEnvelope extends VendorPayload {
  PurchasingSection15VendorPurchasingErrorEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}

class PurchasingSection15VendorPurchasingPurchaseCostObservation
    extends VendorPayload {
  PurchasingSection15VendorPurchasingPurchaseCostObservation.fromJson(
    super.json,
  );
  String? get id => value('id') as String?;
  String? get supplierId => value('supplier_id') as String?;
  String? get supplierProductId => value('supplier_product_id') as String?;
  String? get inventoryItemId => value('inventory_item_id') as String?;
  String? get purchaseOrderId => value('purchase_order_id') as String?;
  String? get goodsReceiptId => value('goods_receipt_id') as String?;
  String? get purchaseUnitCode => value('purchase_unit_code') as String?;
  int? get inventoryUnitsPerPurchaseUnit =>
      value('inventory_units_per_purchase_unit') as int?;
  int? get purchaseUnitCostMinor => value('purchase_unit_cost_minor') as int?;
  int? get totalCostMinor => value('total_cost_minor') as int?;
  String? get currency => value('currency') as String?;
  String? get observedAt => value('observed_at') as String?;
}

class PurchasingSection15VendorPurchasingPurchasingCommand
    extends VendorPayload {
  PurchasingSection15VendorPurchasingPurchasingCommand.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PurchasingSection15VendorPurchasingRatio extends VendorPayload {
  PurchasingSection15VendorPurchasingRatio.fromJson(super.json);
  num? get valueValue => value('value') as num?;
  String? get metricStatus => value('metric_status') as String?;
  int? get sampleCount => value('sample_count') as int?;
}

class PurchasingSection15VendorPurchasingSuccessEnvelope extends VendorPayload {
  PurchasingSection15VendorPurchasingSuccessEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class PurchasingSection15VendorPurchasingSupplierPerformance
    extends VendorPayload {
  PurchasingSection15VendorPurchasingSupplierPerformance.fromJson(super.json);
  String? get supplierId => value('supplier_id') as String?;
  String? get observedFrom => value('observed_from') as String?;
  String? get observedTo => value('observed_to') as String?;
  JsonObject? get onTimeRate =>
      value('on_time_rate') == null ? null : jsonObject(value('on_time_rate'));
  JsonObject? get shortDeliveryRate => value('short_delivery_rate') == null
      ? null
      : jsonObject(value('short_delivery_rate'));
  JsonObject? get damageRate =>
      value('damage_rate') == null ? null : jsonObject(value('damage_rate'));
  JsonObject? get rejectionRate => value('rejection_rate') == null
      ? null
      : jsonObject(value('rejection_rate'));
  JsonObject? get acceptanceRate => value('acceptance_rate') == null
      ? null
      : jsonObject(value('acceptance_rate'));
  JsonObject? get costVarianceRate => value('cost_variance_rate') == null
      ? null
      : jsonObject(value('cost_variance_rate'));
  int? get latestObservedUnitCostMinor =>
      value('latest_observed_unit_cost_minor') as int?;
}
