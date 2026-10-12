// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PutOnboardingBankProfileRequest extends VendorPayload {
  PutOnboardingBankProfileRequest({
    String? accountNumber,
    String? bankCode,
    String? stepUpGrantId,
    int? expectedVersion,
  }) : super({
         'account_number': ?accountNumber,
         'bank_code': ?bankCode,
         'step_up_grant_id': ?stepUpGrantId,
         'expected_version': ?expectedVersion,
       });
  PutOnboardingBankProfileRequest.fromJson(super.json);
  String? get accountNumber => value('account_number') as String?;
  String? get bankCode => value('bank_code') as String?;
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostOnboardingDocumentUploadsRequest extends VendorPayload {
  PostOnboardingDocumentUploadsRequest({
    String? dimension,
    String? subjectType,
    String? subjectRef,
    String? mimeType,
    int? byteSize,
  }) : super({
         'dimension': ?dimension,
         'subject_type': ?subjectType,
         'subject_ref': ?subjectRef,
         'mime_type': ?mimeType,
         'byte_size': ?byteSize,
       });
  PostOnboardingDocumentUploadsRequest.fromJson(super.json);
  String? get dimension => value('dimension') as String?;
  String? get subjectType => value('subject_type') as String?;
  String? get subjectRef => value('subject_ref') as String?;
  String? get mimeType => value('mime_type') as String?;
  int? get byteSize => value('byte_size') as int?;
}

class PostOnboardingDocumentsRequest extends VendorPayload {
  PostOnboardingDocumentsRequest({
    String? representativeId,
    String? branchId,
    String? brandId,
    String? dimension,
    String? mediaRef,
    String? documentType,
    int? expectedVersion,
  }) : super({
         'representative_id': ?representativeId,
         'branch_id': ?branchId,
         'brand_id': ?brandId,
         'dimension': ?dimension,
         'media_ref': ?mediaRef,
         'document_type': ?documentType,
         'expected_version': ?expectedVersion,
       });
  PostOnboardingDocumentsRequest.fromJson(super.json);
  String? get representativeId => value('representative_id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get brandId => value('brand_id') as String?;
  String? get dimension => value('dimension') as String?;
  String? get mediaRef => value('media_ref') as String?;
  String? get documentType => value('document_type') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class DeleteOnboardingDocumentsDocumentIdRequest extends VendorPayload {
  DeleteOnboardingDocumentsDocumentIdRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  DeleteOnboardingDocumentsDocumentIdRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchOnboardingDocumentsDocumentIdRequest extends VendorPayload {
  PatchOnboardingDocumentsDocumentIdRequest({
    String? representativeId,
    String? branchId,
    String? brandId,
    String? dimension,
    String? mediaRef,
    String? documentType,
    int? expectedVersion,
  }) : super({
         'representative_id': ?representativeId,
         'branch_id': ?branchId,
         'brand_id': ?brandId,
         'dimension': ?dimension,
         'media_ref': ?mediaRef,
         'document_type': ?documentType,
         'expected_version': ?expectedVersion,
       });
  PatchOnboardingDocumentsDocumentIdRequest.fromJson(super.json);
  String? get representativeId => value('representative_id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get brandId => value('brand_id') as String?;
  String? get dimension => value('dimension') as String?;
  String? get mediaRef => value('media_ref') as String?;
  String? get documentType => value('document_type') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PutOnboardingRepresentativeRequest extends VendorPayload {
  PutOnboardingRepresentativeRequest({
    String? capacity,
    String? firstName,
    String? lastName,
    String? email,
    String? phone,
    num? ownershipPercent,
    bool? authorized,
    int? expectedVersion,
    String? stepUpGrantId,
  }) : super({
         'capacity': ?capacity,
         'first_name': ?firstName,
         'last_name': ?lastName,
         'email': ?email,
         'phone': ?phone,
         'ownership_percent': ?ownershipPercent,
         'authorized': ?authorized,
         'expected_version': ?expectedVersion,
         'step_up_grant_id': ?stepUpGrantId,
       });
  PutOnboardingRepresentativeRequest.fromJson(super.json);
  String? get capacity => value('capacity') as String?;
  String? get firstName => value('first_name') as String?;
  String? get lastName => value('last_name') as String?;
  String? get email => value('email') as String?;
  String? get phone => value('phone') as String?;
  num? get ownershipPercent => value('ownership_percent') as num?;
  bool? get authorized => value('authorized') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
}

class PostOnboardingVerificationsDimensionResubmitRequest
    extends VendorPayload {
  PostOnboardingVerificationsDimensionResubmitRequest({
    String? subjectType,
    String? subjectRef,
    List<dynamic>? documentRefs,
  }) : super({
         'subject_type': ?subjectType,
         'subject_ref': ?subjectRef,
         'document_refs': ?documentRefs,
       });
  PostOnboardingVerificationsDimensionResubmitRequest.fromJson(super.json);
  String? get subjectType => value('subject_type') as String?;
  String? get subjectRef => value('subject_ref') as String?;
  List<dynamic>? get documentRefs => value('document_refs') as List<dynamic>?;
}

class PostOnboardingVerificationsDimensionSubmitRequest extends VendorPayload {
  PostOnboardingVerificationsDimensionSubmitRequest({
    String? subjectType,
    String? subjectRef,
    List<dynamic>? documentRefs,
  }) : super({
         'subject_type': ?subjectType,
         'subject_ref': ?subjectRef,
         'document_refs': ?documentRefs,
       });
  PostOnboardingVerificationsDimensionSubmitRequest.fromJson(super.json);
  String? get subjectType => value('subject_type') as String?;
  String? get subjectRef => value('subject_ref') as String?;
  List<dynamic>? get documentRefs => value('document_refs') as List<dynamic>?;
}

class OnboardingSection7OnboardingVerificationActivationReadiness
    extends VendorPayload {
  OnboardingSection7OnboardingVerificationActivationReadiness.fromJson(
    super.json,
  );
  List<dynamic>? get blockers => value('blockers') as List<dynamic>?;
  int? get progressPercent => value('progress_percent') as int?;
  bool? get ready => value('ready') as bool?;
  List<dynamic>? get requirements => value('requirements') as List<dynamic>?;
}

class OnboardingSection7OnboardingVerificationErrorEnvelope
    extends VendorPayload {
  OnboardingSection7OnboardingVerificationErrorEnvelope.fromJson(super.json);
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
  bool? get success => value('success') as bool?;
}

class OnboardingSection7OnboardingVerificationMaskedBankProfile
    extends VendorPayload {
  OnboardingSection7OnboardingVerificationMaskedBankProfile.fromJson(
    super.json,
  );
  String? get bankCode => value('bank_code') as String?;
  String? get businessId => value('business_id') as String?;
  String? get externalRef => value('external_ref') as String?;
  String? get maskedAccount => value('masked_account') as String?;
  String? get observedAt => value('observed_at') as String?;
  String? get resolvedAccountName => value('resolved_account_name') as String?;
  int? get sourceVersion => value('source_version') as int?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
}

class OnboardingSection7OnboardingVerificationOnboardingDocument
    extends VendorPayload {
  OnboardingSection7OnboardingVerificationOnboardingDocument.fromJson(
    super.json,
  );
  String? get businessId => value('business_id') as String?;
  String? get dimension => value('dimension') as String?;
  String? get documentType => value('document_type') as String?;
  String? get id => value('id') as String?;
  String? get mediaRef => value('media_ref') as String?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
}

class OnboardingSection7OnboardingVerificationRepresentative
    extends VendorPayload {
  OnboardingSection7OnboardingVerificationRepresentative.fromJson(super.json);
  bool? get active => value('active') as bool?;
  bool? get authorized => value('authorized') as bool?;
  String? get businessId => value('business_id') as String?;
  String? get capacity => value('capacity') as String?;
  String? get email => value('email') as String?;
  String? get firstName => value('first_name') as String?;
  String? get id => value('id') as String?;
  String? get lastName => value('last_name') as String?;
  String? get phone => value('phone') as String?;
  int? get version => value('version') as int?;
}

class OnboardingSection7OnboardingVerificationSuccessEnvelope
    extends VendorPayload {
  OnboardingSection7OnboardingVerificationSuccessEnvelope.fromJson(super.json);
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
  bool? get success => value('success') as bool?;
}

class OnboardingSection7OnboardingVerificationVerificationProjection
    extends VendorPayload {
  OnboardingSection7OnboardingVerificationVerificationProjection.fromJson(
    super.json,
  );
  String? get dimension => value('dimension') as String?;
  String? get expiresAt => value('expires_at') as String?;
  String? get observedAt => value('observed_at') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get remediation => value('remediation') as String?;
  String? get sourceRef => value('source_ref') as String?;
  String? get sourceService => value('source_service') as String?;
  int? get sourceVersion => value('source_version') as int?;
  String? get status => value('status') as String?;
  String? get subjectType => value('subject_type') as String?;
  int? get version => value('version') as int?;
}
