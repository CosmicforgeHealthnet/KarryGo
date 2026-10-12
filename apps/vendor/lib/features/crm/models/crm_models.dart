// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostCrmCampaignsRequest extends VendorPayload {
  PostCrmCampaignsRequest({
    String? name,
    String? objective,
    String? scopeType,
    String? branchRef,
    List<dynamic>? channels,
    String? sourceSegmentRef,
    List<dynamic>? exclusionSegmentRefs,
    int? frequencyCapCount,
    int? frequencyCapWindowMinutes,
    bool? quietHoursEnabled,
    String? quietHoursStart,
    String? quietHoursEnd,
    String? quietHoursTimezone,
    bool? serviceRecoverySuppressionEnabled,
    bool? experimentEnabled,
    List<dynamic>? experimentVariants,
  }) : super({
         'name': ?name,
         'objective': ?objective,
         'scope_type': ?scopeType,
         'branch_ref': ?branchRef,
         'channels': ?channels,
         'source_segment_ref': ?sourceSegmentRef,
         'exclusion_segment_refs': ?exclusionSegmentRefs,
         'frequency_cap_count': ?frequencyCapCount,
         'frequency_cap_window_minutes': ?frequencyCapWindowMinutes,
         'quiet_hours_enabled': ?quietHoursEnabled,
         'quiet_hours_start': ?quietHoursStart,
         'quiet_hours_end': ?quietHoursEnd,
         'quiet_hours_timezone': ?quietHoursTimezone,
         'service_recovery_suppression_enabled':
             ?serviceRecoverySuppressionEnabled,
         'experiment_enabled': ?experimentEnabled,
         'experiment_variants': ?experimentVariants,
       });
  PostCrmCampaignsRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get objective => value('objective') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  List<dynamic>? get channels => value('channels') as List<dynamic>?;
  String? get sourceSegmentRef => value('source_segment_ref') as String?;
  List<dynamic>? get exclusionSegmentRefs =>
      value('exclusion_segment_refs') as List<dynamic>?;
  int? get frequencyCapCount => value('frequency_cap_count') as int?;
  int? get frequencyCapWindowMinutes =>
      value('frequency_cap_window_minutes') as int?;
  bool? get quietHoursEnabled => value('quiet_hours_enabled') as bool?;
  String? get quietHoursStart => value('quiet_hours_start') as String?;
  String? get quietHoursEnd => value('quiet_hours_end') as String?;
  String? get quietHoursTimezone => value('quiet_hours_timezone') as String?;
  bool? get serviceRecoverySuppressionEnabled =>
      value('service_recovery_suppression_enabled') as bool?;
  bool? get experimentEnabled => value('experiment_enabled') as bool?;
  List<dynamic>? get experimentVariants =>
      value('experiment_variants') as List<dynamic>?;
}

class PatchCrmCampaignsCampaignIdRequest extends VendorPayload {
  PatchCrmCampaignsCampaignIdRequest({
    String? name,
    String? objective,
    String? scopeType,
    String? branchRef,
    List<dynamic>? channels,
    String? sourceSegmentRef,
    List<dynamic>? exclusionSegmentRefs,
    int? frequencyCapCount,
    int? frequencyCapWindowMinutes,
    bool? quietHoursEnabled,
    String? quietHoursStart,
    String? quietHoursEnd,
    String? quietHoursTimezone,
    bool? serviceRecoverySuppressionEnabled,
    bool? experimentEnabled,
    List<dynamic>? experimentVariants,
    String? changeReason,
    int? expectedVersion,
  }) : super({
         'name': ?name,
         'objective': ?objective,
         'scope_type': ?scopeType,
         'branch_ref': ?branchRef,
         'channels': ?channels,
         'source_segment_ref': ?sourceSegmentRef,
         'exclusion_segment_refs': ?exclusionSegmentRefs,
         'frequency_cap_count': ?frequencyCapCount,
         'frequency_cap_window_minutes': ?frequencyCapWindowMinutes,
         'quiet_hours_enabled': ?quietHoursEnabled,
         'quiet_hours_start': ?quietHoursStart,
         'quiet_hours_end': ?quietHoursEnd,
         'quiet_hours_timezone': ?quietHoursTimezone,
         'service_recovery_suppression_enabled':
             ?serviceRecoverySuppressionEnabled,
         'experiment_enabled': ?experimentEnabled,
         'experiment_variants': ?experimentVariants,
         'change_reason': ?changeReason,
         'expected_version': ?expectedVersion,
       });
  PatchCrmCampaignsCampaignIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get objective => value('objective') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  List<dynamic>? get channels => value('channels') as List<dynamic>?;
  String? get sourceSegmentRef => value('source_segment_ref') as String?;
  List<dynamic>? get exclusionSegmentRefs =>
      value('exclusion_segment_refs') as List<dynamic>?;
  int? get frequencyCapCount => value('frequency_cap_count') as int?;
  int? get frequencyCapWindowMinutes =>
      value('frequency_cap_window_minutes') as int?;
  bool? get quietHoursEnabled => value('quiet_hours_enabled') as bool?;
  String? get quietHoursStart => value('quiet_hours_start') as String?;
  String? get quietHoursEnd => value('quiet_hours_end') as String?;
  String? get quietHoursTimezone => value('quiet_hours_timezone') as String?;
  bool? get serviceRecoverySuppressionEnabled =>
      value('service_recovery_suppression_enabled') as bool?;
  bool? get experimentEnabled => value('experiment_enabled') as bool?;
  List<dynamic>? get experimentVariants =>
      value('experiment_variants') as List<dynamic>?;
  String? get changeReason => value('change_reason') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCampaignsCampaignIdArchiveRequest extends VendorPayload {
  PostCrmCampaignsCampaignIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostCrmCampaignsCampaignIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCampaignsCampaignIdPreviewRequest extends VendorPayload {
  PostCrmCampaignsCampaignIdPreviewRequest({
    int? sampleLimit,
    String? hypotheticalSendAt,
  }) : super({
         'sample_limit': ?sampleLimit,
         'hypothetical_send_at': ?hypotheticalSendAt,
       });
  PostCrmCampaignsCampaignIdPreviewRequest.fromJson(super.json);
  int? get sampleLimit => value('sample_limit') as int?;
  String? get hypotheticalSendAt => value('hypothetical_send_at') as String?;
}

class PostCrmCampaignsCampaignIdRunsRequest extends VendorPayload {
  PostCrmCampaignsCampaignIdRunsRequest({
    String? snapshotRef,
    String? mode,
    String? scheduledFor,
    int? expectedVersion,
    String? templateKey,
    String? locale,
    String? title,
    String? body,
  }) : super({
         'snapshot_ref': ?snapshotRef,
         'mode': ?mode,
         'scheduled_for': ?scheduledFor,
         'expected_version': ?expectedVersion,
         'template_key': ?templateKey,
         'locale': ?locale,
         'title': ?title,
         'body': ?body,
       });
  PostCrmCampaignsCampaignIdRunsRequest.fromJson(super.json);
  String? get snapshotRef => value('snapshot_ref') as String?;
  String? get mode => value('mode') as String?;
  String? get scheduledFor => value('scheduled_for') as String?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get templateKey => value('template_key') as String?;
  String? get locale => value('locale') as String?;
  String? get title => value('title') as String?;
  String? get body => value('body') as String?;
}

class PostCrmCampaignsCampaignIdRunsRunIdCancelRequest extends VendorPayload {
  PostCrmCampaignsCampaignIdRunsRunIdCancelRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostCrmCampaignsCampaignIdRunsRunIdCancelRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCampaignsCampaignIdRunsRunIdPauseRequest extends VendorPayload {
  PostCrmCampaignsCampaignIdRunsRunIdPauseRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostCrmCampaignsCampaignIdRunsRunIdPauseRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCampaignsCampaignIdRunsRunIdResumeRequest extends VendorPayload {
  PostCrmCampaignsCampaignIdRunsRunIdResumeRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostCrmCampaignsCampaignIdRunsRunIdResumeRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersRequest extends VendorPayload {
  PostCrmCorporateCustomersRequest({
    String? organizationName,
    String? displayName,
    String? organizationType,
    String? scopeMode,
    List<dynamic>? branchRefs,
    String? accountManagerStaffRef,
    bool? clearAccountManager,
    String? relationshipStartedAt,
    String? externalOrganizationRef,
    int? expectedVersion,
  }) : super({
         'organization_name': ?organizationName,
         'display_name': ?displayName,
         'organization_type': ?organizationType,
         'scope_mode': ?scopeMode,
         'branch_refs': ?branchRefs,
         'account_manager_staff_ref': ?accountManagerStaffRef,
         'clear_account_manager': ?clearAccountManager,
         'relationship_started_at': ?relationshipStartedAt,
         'external_organization_ref': ?externalOrganizationRef,
         'expected_version': ?expectedVersion,
       });
  PostCrmCorporateCustomersRequest.fromJson(super.json);
  String? get organizationName => value('organization_name') as String?;
  String? get displayName => value('display_name') as String?;
  String? get organizationType => value('organization_type') as String?;
  String? get scopeMode => value('scope_mode') as String?;
  List<dynamic>? get branchRefs => value('branch_refs') as List<dynamic>?;
  String? get accountManagerStaffRef =>
      value('account_manager_staff_ref') as String?;
  bool? get clearAccountManager => value('clear_account_manager') as bool?;
  String? get relationshipStartedAt =>
      value('relationship_started_at') as String?;
  String? get externalOrganizationRef =>
      value('external_organization_ref') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchCrmCorporateCustomersCorporateIdRequest extends VendorPayload {
  PatchCrmCorporateCustomersCorporateIdRequest({
    String? organizationName,
    String? displayName,
    String? organizationType,
    String? scopeMode,
    List<dynamic>? branchRefs,
    String? accountManagerStaffRef,
    bool? clearAccountManager,
    String? relationshipStartedAt,
    String? externalOrganizationRef,
    int? expectedVersion,
  }) : super({
         'organization_name': ?organizationName,
         'display_name': ?displayName,
         'organization_type': ?organizationType,
         'scope_mode': ?scopeMode,
         'branch_refs': ?branchRefs,
         'account_manager_staff_ref': ?accountManagerStaffRef,
         'clear_account_manager': ?clearAccountManager,
         'relationship_started_at': ?relationshipStartedAt,
         'external_organization_ref': ?externalOrganizationRef,
         'expected_version': ?expectedVersion,
       });
  PatchCrmCorporateCustomersCorporateIdRequest.fromJson(super.json);
  String? get organizationName => value('organization_name') as String?;
  String? get displayName => value('display_name') as String?;
  String? get organizationType => value('organization_type') as String?;
  String? get scopeMode => value('scope_mode') as String?;
  List<dynamic>? get branchRefs => value('branch_refs') as List<dynamic>?;
  String? get accountManagerStaffRef =>
      value('account_manager_staff_ref') as String?;
  bool? get clearAccountManager => value('clear_account_manager') as bool?;
  String? get relationshipStartedAt =>
      value('relationship_started_at') as String?;
  String? get externalOrganizationRef =>
      value('external_organization_ref') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdActivateRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdActivateRequest({
    required int expectedVersion,
  }) : super({'expected_version': expectedVersion});
  PostCrmCorporateCustomersCorporateIdActivateRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdArchiveRequest extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdArchiveRequest({
    required int expectedVersion,
  }) : super({'expected_version': expectedVersion});
  PostCrmCorporateCustomersCorporateIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutCrmCorporateCustomersCorporateIdBranchesRequest extends VendorPayload {
  PutCrmCorporateCustomersCorporateIdBranchesRequest({
    String? scopeMode,
    List<dynamic>? branchRefs,
    int? expectedVersion,
  }) : super({
         'scope_mode': ?scopeMode,
         'branch_refs': ?branchRefs,
         'expected_version': ?expectedVersion,
       });
  PutCrmCorporateCustomersCorporateIdBranchesRequest.fromJson(super.json);
  String? get scopeMode => value('scope_mode') as String?;
  List<dynamic>? get branchRefs => value('branch_refs') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdContactsRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdContactsRequest({
    String? customerProfileRef,
    String? relationshipRole,
    bool? isPrimaryContact,
    String? status,
    int? expectedVersion,
  }) : super({
         'customer_profile_ref': ?customerProfileRef,
         'relationship_role': ?relationshipRole,
         'is_primary_contact': ?isPrimaryContact,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PostCrmCorporateCustomersCorporateIdContactsRequest.fromJson(super.json);
  String? get customerProfileRef => value('customer_profile_ref') as String?;
  String? get relationshipRole => value('relationship_role') as String?;
  bool? get isPrimaryContact => value('is_primary_contact') as bool?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchCrmCorporateCustomersCorporateIdContactsContactIdRequest
    extends VendorPayload {
  PatchCrmCorporateCustomersCorporateIdContactsContactIdRequest({
    String? customerProfileRef,
    String? relationshipRole,
    bool? isPrimaryContact,
    String? status,
    int? expectedVersion,
  }) : super({
         'customer_profile_ref': ?customerProfileRef,
         'relationship_role': ?relationshipRole,
         'is_primary_contact': ?isPrimaryContact,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PatchCrmCorporateCustomersCorporateIdContactsContactIdRequest.fromJson(
    super.json,
  );
  String? get customerProfileRef => value('customer_profile_ref') as String?;
  String? get relationshipRole => value('relationship_role') as String?;
  bool? get isPrimaryContact => value('is_primary_contact') as bool?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdContactsContactIdArchiveRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdContactsContactIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostCrmCorporateCustomersCorporateIdContactsContactIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdCostCentersRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdCostCentersRequest({
    String? code,
    String? name,
    String? status,
    String? locationRef,
    bool? clearLocation,
    String? externalReference,
    int? expectedVersion,
  }) : super({
         'code': ?code,
         'name': ?name,
         'status': ?status,
         'location_ref': ?locationRef,
         'clear_location': ?clearLocation,
         'external_reference': ?externalReference,
         'expected_version': ?expectedVersion,
       });
  PostCrmCorporateCustomersCorporateIdCostCentersRequest.fromJson(super.json);
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get status => value('status') as String?;
  String? get locationRef => value('location_ref') as String?;
  bool? get clearLocation => value('clear_location') as bool?;
  String? get externalReference => value('external_reference') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchCrmCorporateCustomersCorporateIdCostCentersCostCenterIdRequest
    extends VendorPayload {
  PatchCrmCorporateCustomersCorporateIdCostCentersCostCenterIdRequest({
    String? code,
    String? name,
    String? status,
    String? locationRef,
    bool? clearLocation,
    String? externalReference,
    int? expectedVersion,
  }) : super({
         'code': ?code,
         'name': ?name,
         'status': ?status,
         'location_ref': ?locationRef,
         'clear_location': ?clearLocation,
         'external_reference': ?externalReference,
         'expected_version': ?expectedVersion,
       });
  PatchCrmCorporateCustomersCorporateIdCostCentersCostCenterIdRequest.fromJson(
    super.json,
  );
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get status => value('status') as String?;
  String? get locationRef => value('location_ref') as String?;
  bool? get clearLocation => value('clear_location') as bool?;
  String? get externalReference => value('external_reference') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdCostCentersCostCenterIdArchiveRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdCostCentersCostCenterIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostCrmCorporateCustomersCorporateIdCostCentersCostCenterIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdDeactivateRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdDeactivateRequest({
    required int expectedVersion,
  }) : super({'expected_version': expectedVersion});
  PostCrmCorporateCustomersCorporateIdDeactivateRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdHoldRequest extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdHoldRequest({
    required int expectedVersion,
  }) : super({'expected_version': expectedVersion});
  PostCrmCorporateCustomersCorporateIdHoldRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdInteractionsRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdInteractionsRequest({
    String? type,
    String? direction,
    String? outcome,
    String? subject,
    String? note,
    String? occurredAt,
    String? contactRef,
    String? locationRef,
    String? costCenterRef,
  }) : super({
         'type': ?type,
         'direction': ?direction,
         'outcome': ?outcome,
         'subject': ?subject,
         'note': ?note,
         'occurred_at': ?occurredAt,
         'contact_ref': ?contactRef,
         'location_ref': ?locationRef,
         'cost_center_ref': ?costCenterRef,
       });
  PostCrmCorporateCustomersCorporateIdInteractionsRequest.fromJson(super.json);
  String? get type => value('type') as String?;
  String? get direction => value('direction') as String?;
  String? get outcome => value('outcome') as String?;
  String? get subject => value('subject') as String?;
  String? get note => value('note') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
}

class PostCrmCorporateCustomersCorporateIdInteractionsInteractionIdCorrectRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdInteractionsInteractionIdCorrectRequest({
    String? subject,
    String? note,
    String? outcome,
    String? occurredAt,
    String? correctionReason,
    int? expectedVersion,
  }) : super({
         'subject': ?subject,
         'note': ?note,
         'outcome': ?outcome,
         'occurred_at': ?occurredAt,
         'correction_reason': ?correctionReason,
         'expected_version': ?expectedVersion,
       });
  PostCrmCorporateCustomersCorporateIdInteractionsInteractionIdCorrectRequest.fromJson(
    super.json,
  );
  String? get subject => value('subject') as String?;
  String? get note => value('note') as String?;
  String? get outcome => value('outcome') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get correctionReason => value('correction_reason') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdLocationsRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdLocationsRequest({
    String? locationCode,
    String? name,
    String? locationType,
    String? addressLine1,
    String? addressLine2,
    String? city,
    String? stateRegion,
    String? postalCode,
    String? countryCode,
    String? timezone,
    String? externalLocationRef,
    String? status,
    int? expectedVersion,
  }) : super({
         'location_code': ?locationCode,
         'name': ?name,
         'location_type': ?locationType,
         'address_line1': ?addressLine1,
         'address_line2': ?addressLine2,
         'city': ?city,
         'state_region': ?stateRegion,
         'postal_code': ?postalCode,
         'country_code': ?countryCode,
         'timezone': ?timezone,
         'external_location_ref': ?externalLocationRef,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PostCrmCorporateCustomersCorporateIdLocationsRequest.fromJson(super.json);
  String? get locationCode => value('location_code') as String?;
  String? get name => value('name') as String?;
  String? get locationType => value('location_type') as String?;
  String? get addressLine1 => value('address_line1') as String?;
  String? get addressLine2 => value('address_line2') as String?;
  String? get city => value('city') as String?;
  String? get stateRegion => value('state_region') as String?;
  String? get postalCode => value('postal_code') as String?;
  String? get countryCode => value('country_code') as String?;
  String? get timezone => value('timezone') as String?;
  String? get externalLocationRef => value('external_location_ref') as String?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchCrmCorporateCustomersCorporateIdLocationsLocationIdRequest
    extends VendorPayload {
  PatchCrmCorporateCustomersCorporateIdLocationsLocationIdRequest({
    String? locationCode,
    String? name,
    String? locationType,
    String? addressLine1,
    String? addressLine2,
    String? city,
    String? stateRegion,
    String? postalCode,
    String? countryCode,
    String? timezone,
    String? externalLocationRef,
    String? status,
    int? expectedVersion,
  }) : super({
         'location_code': ?locationCode,
         'name': ?name,
         'location_type': ?locationType,
         'address_line1': ?addressLine1,
         'address_line2': ?addressLine2,
         'city': ?city,
         'state_region': ?stateRegion,
         'postal_code': ?postalCode,
         'country_code': ?countryCode,
         'timezone': ?timezone,
         'external_location_ref': ?externalLocationRef,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PatchCrmCorporateCustomersCorporateIdLocationsLocationIdRequest.fromJson(
    super.json,
  );
  String? get locationCode => value('location_code') as String?;
  String? get name => value('name') as String?;
  String? get locationType => value('location_type') as String?;
  String? get addressLine1 => value('address_line1') as String?;
  String? get addressLine2 => value('address_line2') as String?;
  String? get city => value('city') as String?;
  String? get stateRegion => value('state_region') as String?;
  String? get postalCode => value('postal_code') as String?;
  String? get countryCode => value('country_code') as String?;
  String? get timezone => value('timezone') as String?;
  String? get externalLocationRef => value('external_location_ref') as String?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdLocationsLocationIdArchiveRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdLocationsLocationIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostCrmCorporateCustomersCorporateIdLocationsLocationIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdMembershipsRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdMembershipsRequest({
    String? contactRef,
    String? locationRef,
    String? costCenterRef,
    bool? clearLocation,
    bool? clearCostCenter,
    String? membershipRole,
    String? status,
    int? expectedVersion,
  }) : super({
         'contact_ref': ?contactRef,
         'location_ref': ?locationRef,
         'cost_center_ref': ?costCenterRef,
         'clear_location': ?clearLocation,
         'clear_cost_center': ?clearCostCenter,
         'membership_role': ?membershipRole,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PostCrmCorporateCustomersCorporateIdMembershipsRequest.fromJson(super.json);
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
  bool? get clearLocation => value('clear_location') as bool?;
  bool? get clearCostCenter => value('clear_cost_center') as bool?;
  String? get membershipRole => value('membership_role') as String?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchCrmCorporateCustomersCorporateIdMembershipsMembershipIdRequest
    extends VendorPayload {
  PatchCrmCorporateCustomersCorporateIdMembershipsMembershipIdRequest({
    String? contactRef,
    String? locationRef,
    String? costCenterRef,
    bool? clearLocation,
    bool? clearCostCenter,
    String? membershipRole,
    String? status,
    int? expectedVersion,
  }) : super({
         'contact_ref': ?contactRef,
         'location_ref': ?locationRef,
         'cost_center_ref': ?costCenterRef,
         'clear_location': ?clearLocation,
         'clear_cost_center': ?clearCostCenter,
         'membership_role': ?membershipRole,
         'status': ?status,
         'expected_version': ?expectedVersion,
       });
  PatchCrmCorporateCustomersCorporateIdMembershipsMembershipIdRequest.fromJson(
    super.json,
  );
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
  bool? get clearLocation => value('clear_location') as bool?;
  bool? get clearCostCenter => value('clear_cost_center') as bool?;
  String? get membershipRole => value('membership_role') as String?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdMembershipsMembershipIdArchiveRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdMembershipsMembershipIdArchiveRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostCrmCorporateCustomersCorporateIdMembershipsMembershipIdArchiveRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdTasksRequest extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdTasksRequest({
    String? title,
    String? description,
    String? priority,
    String? assignedStaffRef,
    String? dueAt,
    bool? clearDueAt,
    String? contactRef,
    String? locationRef,
    String? costCenterRef,
    String? sourceInteractionRef,
    int? expectedVersion,
  }) : super({
         'title': ?title,
         'description': ?description,
         'priority': ?priority,
         'assigned_staff_ref': ?assignedStaffRef,
         'due_at': ?dueAt,
         'clear_due_at': ?clearDueAt,
         'contact_ref': ?contactRef,
         'location_ref': ?locationRef,
         'cost_center_ref': ?costCenterRef,
         'source_interaction_ref': ?sourceInteractionRef,
         'expected_version': ?expectedVersion,
       });
  PostCrmCorporateCustomersCorporateIdTasksRequest.fromJson(super.json);
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  String? get priority => value('priority') as String?;
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  String? get dueAt => value('due_at') as String?;
  bool? get clearDueAt => value('clear_due_at') as bool?;
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
  String? get sourceInteractionRef =>
      value('source_interaction_ref') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PatchCrmCorporateCustomersCorporateIdTasksTaskIdRequest
    extends VendorPayload {
  PatchCrmCorporateCustomersCorporateIdTasksTaskIdRequest({
    String? title,
    String? description,
    String? priority,
    String? assignedStaffRef,
    String? dueAt,
    bool? clearDueAt,
    String? contactRef,
    String? locationRef,
    String? costCenterRef,
    String? sourceInteractionRef,
    int? expectedVersion,
  }) : super({
         'title': ?title,
         'description': ?description,
         'priority': ?priority,
         'assigned_staff_ref': ?assignedStaffRef,
         'due_at': ?dueAt,
         'clear_due_at': ?clearDueAt,
         'contact_ref': ?contactRef,
         'location_ref': ?locationRef,
         'cost_center_ref': ?costCenterRef,
         'source_interaction_ref': ?sourceInteractionRef,
         'expected_version': ?expectedVersion,
       });
  PatchCrmCorporateCustomersCorporateIdTasksTaskIdRequest.fromJson(super.json);
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  String? get priority => value('priority') as String?;
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  String? get dueAt => value('due_at') as String?;
  bool? get clearDueAt => value('clear_due_at') as bool?;
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
  String? get sourceInteractionRef =>
      value('source_interaction_ref') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdTasksTaskIdCancelRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdTasksTaskIdCancelRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostCrmCorporateCustomersCorporateIdTasksTaskIdCancelRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdTasksTaskIdCompleteRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdTasksTaskIdCompleteRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostCrmCorporateCustomersCorporateIdTasksTaskIdCompleteRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdTasksTaskIdReassignRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdTasksTaskIdReassignRequest({
    String? title,
    String? description,
    String? priority,
    String? assignedStaffRef,
    String? dueAt,
    bool? clearDueAt,
    String? contactRef,
    String? locationRef,
    String? costCenterRef,
    String? sourceInteractionRef,
    int? expectedVersion,
  }) : super({
         'title': ?title,
         'description': ?description,
         'priority': ?priority,
         'assigned_staff_ref': ?assignedStaffRef,
         'due_at': ?dueAt,
         'clear_due_at': ?clearDueAt,
         'contact_ref': ?contactRef,
         'location_ref': ?locationRef,
         'cost_center_ref': ?costCenterRef,
         'source_interaction_ref': ?sourceInteractionRef,
         'expected_version': ?expectedVersion,
       });
  PostCrmCorporateCustomersCorporateIdTasksTaskIdReassignRequest.fromJson(
    super.json,
  );
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  String? get priority => value('priority') as String?;
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  String? get dueAt => value('due_at') as String?;
  bool? get clearDueAt => value('clear_due_at') as bool?;
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
  String? get sourceInteractionRef =>
      value('source_interaction_ref') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCorporateCustomersCorporateIdTasksTaskIdStartRequest
    extends VendorPayload {
  PostCrmCorporateCustomersCorporateIdTasksTaskIdStartRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PostCrmCorporateCustomersCorporateIdTasksTaskIdStartRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmCustomersProfileIdLoyaltyAdjustmentsRequest extends VendorPayload {
  PostCrmCustomersProfileIdLoyaltyAdjustmentsRequest({
    String? direction,
    int? points,
    String? reason,
  }) : super({'direction': ?direction, 'points': ?points, 'reason': ?reason});
  PostCrmCustomersProfileIdLoyaltyAdjustmentsRequest.fromJson(super.json);
  String? get direction => value('direction') as String?;
  int? get points => value('points') as int?;
  String? get reason => value('reason') as String?;
}

class PostCrmExportsRequest extends VendorPayload {
  PostCrmExportsRequest({
    String? exportType,
    String? format,
    String? scopeType,
    String? branchRef,
    int? rowLimit,
    JsonObject? filter,
    String? stepUpGrantId,
  }) : super({
         'export_type': ?exportType,
         'format': ?format,
         'scope_type': ?scopeType,
         'branch_ref': ?branchRef,
         'row_limit': ?rowLimit,
         'filter': ?filter,
         'step_up_grant_id': ?stepUpGrantId,
       });
  PostCrmExportsRequest.fromJson(super.json);
  String? get exportType => value('export_type') as String?;
  String? get format => value('format') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  int? get rowLimit => value('row_limit') as int?;
  JsonObject? get filter =>
      value('filter') == null ? null : jsonObject(value('filter'));
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
}

class PostCrmExportsExportIdCancelRequest extends VendorPayload {
  PostCrmExportsExportIdCancelRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostCrmExportsExportIdCancelRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmExportsExportIdDownloadRequest extends VendorPayload {
  PostCrmExportsExportIdDownloadRequest({String? stepUpGrantId})
    : super({'step_up_grant_id': ?stepUpGrantId});
  PostCrmExportsExportIdDownloadRequest.fromJson(super.json);
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
}

class PostCrmIntelligenceRebuildRequest extends VendorPayload {
  PostCrmIntelligenceRebuildRequest({
    String? modelKey,
    String? scopeType,
    String? branchRef,
  }) : super({
         'model_key': ?modelKey,
         'scope_type': ?scopeType,
         'branch_ref': ?branchRef,
       });
  PostCrmIntelligenceRebuildRequest.fromJson(super.json);
  String? get modelKey => value('model_key') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
}

class PostCrmInteractionsRequest extends VendorPayload {
  PostCrmInteractionsRequest({
    String? customerProfileRef,
    String? branchRef,
    String? type,
    String? direction,
    String? outcome,
    String? subject,
    String? note,
    String? visibility,
    String? occurredAt,
    String? relatedOrderRef,
    String? relatedRfqRef,
    String? relatedSupportCaseRef,
  }) : super({
         'customer_profile_ref': ?customerProfileRef,
         'branch_ref': ?branchRef,
         'type': ?type,
         'direction': ?direction,
         'outcome': ?outcome,
         'subject': ?subject,
         'note': ?note,
         'visibility': ?visibility,
         'occurred_at': ?occurredAt,
         'related_order_ref': ?relatedOrderRef,
         'related_rfq_ref': ?relatedRfqRef,
         'related_support_case_ref': ?relatedSupportCaseRef,
       });
  PostCrmInteractionsRequest.fromJson(super.json);
  String? get customerProfileRef => value('customer_profile_ref') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get type => value('type') as String?;
  String? get direction => value('direction') as String?;
  String? get outcome => value('outcome') as String?;
  String? get subject => value('subject') as String?;
  String? get note => value('note') as String?;
  String? get visibility => value('visibility') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get relatedOrderRef => value('related_order_ref') as String?;
  String? get relatedRfqRef => value('related_rfq_ref') as String?;
  String? get relatedSupportCaseRef =>
      value('related_support_case_ref') as String?;
}

class PostCrmInteractionsInteractionIdCorrectRequest extends VendorPayload {
  PostCrmInteractionsInteractionIdCorrectRequest({
    String? subject,
    String? note,
    String? outcome,
    String? occurredAt,
    String? correctionReason,
    int? expectedVersion,
  }) : super({
         'subject': ?subject,
         'note': ?note,
         'outcome': ?outcome,
         'occurred_at': ?occurredAt,
         'correction_reason': ?correctionReason,
         'expected_version': ?expectedVersion,
       });
  PostCrmInteractionsInteractionIdCorrectRequest.fromJson(super.json);
  String? get subject => value('subject') as String?;
  String? get note => value('note') as String?;
  String? get outcome => value('outcome') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get correctionReason => value('correction_reason') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmLoyaltyEntriesEntryIdReverseRequest extends VendorPayload {
  PostCrmLoyaltyEntriesEntryIdReverseRequest({String? reason})
    : super({'reason': ?reason});
  PostCrmLoyaltyEntriesEntryIdReverseRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
}

class PatchCrmLoyaltyProgramRequest extends VendorPayload {
  PatchCrmLoyaltyProgramRequest({
    String? status,
    String? pointsNameSingular,
    String? pointsNamePlural,
    String? expiryPolicy,
    int? expiryDays,
    bool? clearExpiryDays,
    int? minimumRedemptionPoints,
    int? redemptionIncrementPoints,
    int? manualAdjustmentLimit,
    int? expectedVersion,
  }) : super({
         'status': ?status,
         'points_name_singular': ?pointsNameSingular,
         'points_name_plural': ?pointsNamePlural,
         'expiry_policy': ?expiryPolicy,
         'expiry_days': ?expiryDays,
         'clear_expiry_days': ?clearExpiryDays,
         'minimum_redemption_points': ?minimumRedemptionPoints,
         'redemption_increment_points': ?redemptionIncrementPoints,
         'manual_adjustment_limit': ?manualAdjustmentLimit,
         'expected_version': ?expectedVersion,
       });
  PatchCrmLoyaltyProgramRequest.fromJson(super.json);
  String? get status => value('status') as String?;
  String? get pointsNameSingular => value('points_name_singular') as String?;
  String? get pointsNamePlural => value('points_name_plural') as String?;
  String? get expiryPolicy => value('expiry_policy') as String?;
  int? get expiryDays => value('expiry_days') as int?;
  bool? get clearExpiryDays => value('clear_expiry_days') as bool?;
  int? get minimumRedemptionPoints =>
      value('minimum_redemption_points') as int?;
  int? get redemptionIncrementPoints =>
      value('redemption_increment_points') as int?;
  int? get manualAdjustmentLimit => value('manual_adjustment_limit') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PutCrmLoyaltyTiersRequest extends VendorPayload {
  PutCrmLoyaltyTiersRequest({
    required List<JsonObject> tiers,
    int? expectedVersion,
  }) : super({'tiers': tiers, 'expected_version': ?expectedVersion});
  PutCrmLoyaltyTiersRequest.fromJson(super.json);
  List<JsonObject> get tiers =>
      (value('tiers') as List).map(jsonObject).toList();
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmSegmentsRequest extends VendorPayload {
  PostCrmSegmentsRequest({
    String? name,
    String? description,
    String? segmentType,
    JsonObject? scope,
    Object? rule,
  }) : super({
         'name': ?name,
         'description': ?description,
         'segment_type': ?segmentType,
         'scope': ?scope,
         'rule': ?rule,
       });
  PostCrmSegmentsRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get segmentType => value('segment_type') as String?;
  JsonObject? get scope =>
      value('scope') == null ? null : jsonObject(value('scope'));
  Object? get rule => value('rule');
}

class PostCrmSegmentsPreviewRequest extends VendorPayload {
  PostCrmSegmentsPreviewRequest({
    JsonObject? scope,
    Object? rule,
    int? sampleLimit,
  }) : super({'scope': ?scope, 'rule': ?rule, 'sample_limit': ?sampleLimit});
  PostCrmSegmentsPreviewRequest.fromJson(super.json);
  JsonObject? get scope =>
      value('scope') == null ? null : jsonObject(value('scope'));
  Object? get rule => value('rule');
  int? get sampleLimit => value('sample_limit') as int?;
}

class PatchCrmSegmentsSegmentIdRequest extends VendorPayload {
  PatchCrmSegmentsSegmentIdRequest({
    String? name,
    String? description,
    Object? rule,
    String? changeReason,
    int? expectedVersion,
  }) : super({
         'name': ?name,
         'description': ?description,
         'rule': ?rule,
         'change_reason': ?changeReason,
         'expected_version': ?expectedVersion,
       });
  PatchCrmSegmentsSegmentIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  Object? get rule => value('rule');
  String? get changeReason => value('change_reason') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmSegmentsSegmentIdActivateRequest extends VendorPayload {
  PostCrmSegmentsSegmentIdActivateRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostCrmSegmentsSegmentIdActivateRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmSegmentsSegmentIdArchiveRequest extends VendorPayload {
  PostCrmSegmentsSegmentIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostCrmSegmentsSegmentIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmSegmentsSegmentIdExclusionsBulkAddRequest extends VendorPayload {
  PostCrmSegmentsSegmentIdExclusionsBulkAddRequest({List<dynamic>? profileRefs})
    : super({'profile_refs': ?profileRefs});
  PostCrmSegmentsSegmentIdExclusionsBulkAddRequest.fromJson(super.json);
  List<dynamic>? get profileRefs => value('profile_refs') as List<dynamic>?;
}

class PostCrmSegmentsSegmentIdExclusionsBulkRemoveRequest
    extends VendorPayload {
  PostCrmSegmentsSegmentIdExclusionsBulkRemoveRequest({
    List<dynamic>? profileRefs,
  }) : super({'profile_refs': ?profileRefs});
  PostCrmSegmentsSegmentIdExclusionsBulkRemoveRequest.fromJson(super.json);
  List<dynamic>? get profileRefs => value('profile_refs') as List<dynamic>?;
}

class PostCrmSegmentsSegmentIdMembersBulkAddRequest extends VendorPayload {
  PostCrmSegmentsSegmentIdMembersBulkAddRequest({List<dynamic>? profileRefs})
    : super({'profile_refs': ?profileRefs});
  PostCrmSegmentsSegmentIdMembersBulkAddRequest.fromJson(super.json);
  List<dynamic>? get profileRefs => value('profile_refs') as List<dynamic>?;
}

class PostCrmSegmentsSegmentIdMembersBulkRemoveRequest extends VendorPayload {
  PostCrmSegmentsSegmentIdMembersBulkRemoveRequest({List<dynamic>? profileRefs})
    : super({'profile_refs': ?profileRefs});
  PostCrmSegmentsSegmentIdMembersBulkRemoveRequest.fromJson(super.json);
  List<dynamic>? get profileRefs => value('profile_refs') as List<dynamic>?;
}

class PostCrmTagsRequest extends VendorPayload {
  PostCrmTagsRequest({String? name, String? description, JsonObject? scope})
    : super({'name': ?name, 'description': ?description, 'scope': ?scope});
  PostCrmTagsRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  JsonObject? get scope =>
      value('scope') == null ? null : jsonObject(value('scope'));
}

class PostCrmTagsBulkAssignRequest extends VendorPayload {
  PostCrmTagsBulkAssignRequest({String? tagRef, List<dynamic>? profileRefs})
    : super({'tag_ref': ?tagRef, 'profile_refs': ?profileRefs});
  PostCrmTagsBulkAssignRequest.fromJson(super.json);
  String? get tagRef => value('tag_ref') as String?;
  List<dynamic>? get profileRefs => value('profile_refs') as List<dynamic>?;
}

class PostCrmTagsBulkRemoveRequest extends VendorPayload {
  PostCrmTagsBulkRemoveRequest({String? tagRef, List<dynamic>? profileRefs})
    : super({'tag_ref': ?tagRef, 'profile_refs': ?profileRefs});
  PostCrmTagsBulkRemoveRequest.fromJson(super.json);
  String? get tagRef => value('tag_ref') as String?;
  List<dynamic>? get profileRefs => value('profile_refs') as List<dynamic>?;
}

class PatchCrmTagsTagIdRequest extends VendorPayload {
  PatchCrmTagsTagIdRequest({
    String? name,
    String? description,
    int? expectedVersion,
  }) : super({
         'name': ?name,
         'description': ?description,
         'expected_version': ?expectedVersion,
       });
  PatchCrmTagsTagIdRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmTagsTagIdArchiveRequest extends VendorPayload {
  PostCrmTagsTagIdArchiveRequest({int? expectedVersion})
    : super({'expected_version': ?expectedVersion});
  PostCrmTagsTagIdArchiveRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmTasksRequest extends VendorPayload {
  PostCrmTasksRequest({
    String? customerProfileRef,
    String? branchRef,
    String? visibility,
    String? title,
    String? description,
    String? priority,
    String? assignedStaffRef,
    String? dueAt,
    String? sourceInteractionRef,
    String? relatedOrderRef,
    String? relatedRfqRef,
    String? relatedSupportCaseRef,
  }) : super({
         'customer_profile_ref': ?customerProfileRef,
         'branch_ref': ?branchRef,
         'visibility': ?visibility,
         'title': ?title,
         'description': ?description,
         'priority': ?priority,
         'assigned_staff_ref': ?assignedStaffRef,
         'due_at': ?dueAt,
         'source_interaction_ref': ?sourceInteractionRef,
         'related_order_ref': ?relatedOrderRef,
         'related_rfq_ref': ?relatedRfqRef,
         'related_support_case_ref': ?relatedSupportCaseRef,
       });
  PostCrmTasksRequest.fromJson(super.json);
  String? get customerProfileRef => value('customer_profile_ref') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get visibility => value('visibility') as String?;
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  String? get priority => value('priority') as String?;
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  String? get dueAt => value('due_at') as String?;
  String? get sourceInteractionRef =>
      value('source_interaction_ref') as String?;
  String? get relatedOrderRef => value('related_order_ref') as String?;
  String? get relatedRfqRef => value('related_rfq_ref') as String?;
  String? get relatedSupportCaseRef =>
      value('related_support_case_ref') as String?;
}

class PatchCrmTasksTaskIdRequest extends VendorPayload {
  PatchCrmTasksTaskIdRequest({
    String? title,
    String? description,
    String? priority,
    String? dueAt,
    bool? clearDueAt,
    int? expectedVersion,
  }) : super({
         'title': ?title,
         'description': ?description,
         'priority': ?priority,
         'due_at': ?dueAt,
         'clear_due_at': ?clearDueAt,
         'expected_version': ?expectedVersion,
       });
  PatchCrmTasksTaskIdRequest.fromJson(super.json);
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  String? get priority => value('priority') as String?;
  String? get dueAt => value('due_at') as String?;
  bool? get clearDueAt => value('clear_due_at') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmTasksTaskIdCancelRequest extends VendorPayload {
  PostCrmTasksTaskIdCancelRequest({required int expectedVersion})
    : super({'expected_version': expectedVersion});
  PostCrmTasksTaskIdCancelRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmTasksTaskIdCompleteRequest extends VendorPayload {
  PostCrmTasksTaskIdCompleteRequest({required int expectedVersion})
    : super({'expected_version': expectedVersion});
  PostCrmTasksTaskIdCompleteRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmTasksTaskIdReassignRequest extends VendorPayload {
  PostCrmTasksTaskIdReassignRequest({
    String? assignedStaffRef,
    int? expectedVersion,
  }) : super({
         'assigned_staff_ref': ?assignedStaffRef,
         'expected_version': ?expectedVersion,
       });
  PostCrmTasksTaskIdReassignRequest.fromJson(super.json);
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostCrmTasksTaskIdStartRequest extends VendorPayload {
  PostCrmTasksTaskIdStartRequest({required int expectedVersion})
    : super({'expected_version': expectedVersion});
  PostCrmTasksTaskIdStartRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8CompletionCrmErrorEnvelope extends VendorPayload {
  CrmSection8CompletionCrmErrorEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}

class CrmSection8CompletionCrmLoyaltyTierDefinition extends VendorPayload {
  CrmSection8CompletionCrmLoyaltyTierDefinition.fromJson(super.json);
  String? get id => value('id') as String?;
  String? get tierKey => value('tier_key') as String?;
  String? get displayName => value('display_name') as String?;
  int? get minimumLifetimePoints => value('minimum_lifetime_points') as int?;
  int? get rank => value('rank') as int?;
  String? get status => value('status') as String?;
  int? get version => value('version') as int?;
  String? get updatedAt => value('updated_at') as String?;
}

class CrmSection8CompletionCrmLoyaltyTierSet extends VendorPayload {
  CrmSection8CompletionCrmLoyaltyTierSet.fromJson(super.json);
  List<dynamic>? get tiers => value('tiers') as List<dynamic>?;
  int? get version => value('version') as int?;
  String? get updatedAt => value('updated_at') as String?;
}

class CrmSection8CompletionCrmReplaceLoyaltyTiersRequest extends VendorPayload {
  CrmSection8CompletionCrmReplaceLoyaltyTiersRequest.fromJson(super.json);
  List<dynamic>? get tiers => value('tiers') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8CompletionCrmCustomerLoyaltyTier extends VendorPayload {
  CrmSection8CompletionCrmCustomerLoyaltyTier.fromJson(super.json);
  String? get state => value('state') as String?;
  String? get tierKey => value('tier_key') as String?;
  String? get displayName => value('display_name') as String?;
  int? get rank => value('rank') as int?;
  int? get lifetimePoints => value('lifetime_points') as int?;
  String? get nextTierKey => value('next_tier_key') as String?;
  int? get pointsToNextTier => value('points_to_next_tier') as int?;
  int? get configuredTierCount => value('configured_tier_count') as int?;
  int? get definitionSetVersion => value('definition_set_version') as int?;
}

class CrmSection8aCrmFoundationPagination extends VendorPayload {
  CrmSection8aCrmFoundationPagination.fromJson(super.json);
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8aCrmFoundationProfileView extends VendorPayload {
  CrmSection8aCrmFoundationProfileView.fromJson(super.json);
  String? get profileRef => value('profile_ref') as String?;
  String? get subjectType => value('subject_type') as String?;
  String? get displayLabel => value('display_label') as String?;
  String? get status => value('status') as String?;
  String? get privacyState => value('privacy_state') as String?;
  String? get mergeState => value('merge_state') as String?;
  String? get acquisitionSourceType =>
      value('acquisition_source_type') as String?;
  String? get createdAt => value('created_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8aCrmFoundationCommercialSummary extends VendorPayload {
  CrmSection8aCrmFoundationCommercialSummary.fromJson(super.json);
  String? get state => value('state') as String?;
  String? get scope => value('scope') as String?;
  String? get currency => value('currency') as String?;
  int? get completedOrderCount => value('completed_order_count') as int?;
  int? get grossSpendMinor => value('gross_spend_minor') as int?;
  int? get netSpendMinor => value('net_spend_minor') as int?;
  int? get averageOrderValueMinor => value('average_order_value_minor') as int?;
  String? get firstCompletedOrderAt =>
      value('first_completed_order_at') as String?;
  String? get lastCompletedOrderAt =>
      value('last_completed_order_at') as String?;
  String? get lastOrderRef => value('last_order_ref') as String?;
  int? get recencyDays => value('recency_days') as int?;
  num? get orderFrequencyPerYear => value('order_frequency_per_year') as num?;
  String? get observedAt => value('observed_at') as String?;
  int? get projectionVersion => value('projection_version') as int?;
  bool? get stale => value('stale') as bool?;
}

class CrmSection8aCrmFoundationBranchSummary extends VendorPayload {
  CrmSection8aCrmFoundationBranchSummary.fromJson(super.json);
  String? get branchRef => value('branch_ref') as String?;
  int? get completedOrderCount => value('completed_order_count') as int?;
  int? get grossSpendMinor => value('gross_spend_minor') as int?;
  int? get netSpendMinor => value('net_spend_minor') as int?;
  String? get currency => value('currency') as String?;
  String? get firstRelationshipAt => value('first_relationship_at') as String?;
  String? get lastRelationshipAt => value('last_relationship_at') as String?;
  String? get lastCapabilityType => value('last_capability_type') as String?;
  String? get projectionState => value('projection_state') as String?;
}

class CrmSection8aCrmFoundationCapabilitySummary extends VendorPayload {
  CrmSection8aCrmFoundationCapabilitySummary.fromJson(super.json);
  String? get lastCapabilityType => value('last_capability_type') as String?;
  String? get categoryAffinity => value('category_affinity') as String?;
  String? get itemAffinity => value('item_affinity') as String?;
}

class CrmSection8aCrmFoundationChannelConsent extends VendorPayload {
  CrmSection8aCrmFoundationChannelConsent.fromJson(super.json);
  String? get channel => value('channel') as String?;
  String? get decision => value('decision') as String?;
  bool? get eligible => value('eligible') as bool?;
  String? get reason => value('reason') as String?;
  bool? get stale => value('stale') as bool?;
  String? get observedAt => value('observed_at') as String?;
}

class CrmSection8aCrmFoundationConsentSummary extends VendorPayload {
  CrmSection8aCrmFoundationConsentSummary.fromJson(super.json);
  bool? get integrationAvailable => value('integration_available') as bool?;
  JsonObject? get marketing =>
      value('marketing') == null ? null : jsonObject(value('marketing'));
}

class CrmSection8aCrmFoundationPrivacySummary extends VendorPayload {
  CrmSection8aCrmFoundationPrivacySummary.fromJson(super.json);
  String? get state => value('state') as String?;
  bool? get marketingSuppressed => value('marketing_suppressed') as bool?;
  bool? get redacted => value('redacted') as bool?;
  String? get redactedAt => value('redacted_at') as String?;
  bool? get integrationAvailable => value('integration_available') as bool?;
  List<dynamic>? get coveredDomains =>
      value('covered_domains') as List<dynamic>?;
}

class CrmSection8aCrmFoundationIdentityLinkSummary extends VendorPayload {
  CrmSection8aCrmFoundationIdentityLinkSummary.fromJson(super.json);
  String? get mergeState => value('merge_state') as String?;
  String? get mergedIntoProfileRef =>
      value('merged_into_profile_ref') as String?;
  int? get linkedProfileCount => value('linked_profile_count') as int?;
}

class CrmSection8aCrmFoundationMarketingEligibilitySummary
    extends VendorPayload {
  CrmSection8aCrmFoundationMarketingEligibilitySummary.fromJson(super.json);
  bool? get integrationAvailable => value('integration_available') as bool?;
  List<dynamic>? get eligibleChannels =>
      value('eligible_channels') as List<dynamic>?;
  bool? get anyChannelEligible => value('any_channel_eligible') as bool?;
}

class CrmSection8aCrmFoundationCustomerListItem extends VendorPayload {
  CrmSection8aCrmFoundationCustomerListItem.fromJson(super.json);
  JsonObject? get profile =>
      value('profile') == null ? null : jsonObject(value('profile'));
  JsonObject? get commercial =>
      value('commercial') == null ? null : jsonObject(value('commercial'));
  JsonObject? get marketingEligibility => value('marketing_eligibility') == null
      ? null
      : jsonObject(value('marketing_eligibility'));
}

class CrmSection8aCrmFoundationCustomer360 extends VendorPayload {
  CrmSection8aCrmFoundationCustomer360.fromJson(super.json);
  JsonObject? get profile =>
      value('profile') == null ? null : jsonObject(value('profile'));
  JsonObject? get commercial =>
      value('commercial') == null ? null : jsonObject(value('commercial'));
  List<dynamic>? get branches => value('branches') as List<dynamic>?;
  JsonObject? get capability =>
      value('capability') == null ? null : jsonObject(value('capability'));
  JsonObject? get consent =>
      value('consent') == null ? null : jsonObject(value('consent'));
  JsonObject? get privacy =>
      value('privacy') == null ? null : jsonObject(value('privacy'));
  JsonObject? get identityLink => value('identity_link') == null
      ? null
      : jsonObject(value('identity_link'));
  JsonObject? get modules =>
      value('modules') == null ? null : jsonObject(value('modules'));
}

class CrmSection8aCrmFoundationCrmEventSubject extends VendorPayload {
  CrmSection8aCrmFoundationCrmEventSubject.fromJson(super.json);
  String? get type => value('type') as String?;
  String? get ref => value('ref') as String?;
  String? get privacySubjectRef => value('privacy_subject_ref') as String?;
}

class CrmSection8aCrmFoundationCrmEventEnvelope extends VendorPayload {
  CrmSection8aCrmFoundationCrmEventEnvelope.fromJson(super.json);
  String? get eventId => value('event_id') as String?;
  String? get eventType => value('event_type') as String?;
  String? get sourceService => value('source_service') as String?;
  int? get sourceVersion => value('source_version') as int?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get correlationId => value('correlation_id') as String?;
  String? get causationId => value('causation_id') as String?;
  String? get businessRef => value('business_ref') as String?;
  JsonObject? get subject =>
      value('subject') == null ? null : jsonObject(value('subject'));
  JsonObject? get payload =>
      value('payload') == null ? null : jsonObject(value('payload'));
}

class CrmSection8aCrmFoundationCrmEventPayload extends VendorPayload {
  CrmSection8aCrmFoundationCrmEventPayload.fromJson(super.json);
  String? get purpose => value('purpose') as String?;
  String? get channel => value('channel') as String?;
  String? get decision => value('decision') as String?;
  String? get effectiveAt => value('effective_at') as String?;
  String? get expiresAt => value('expires_at') as String?;
  String? get action => value('action') as String?;
  String? get retentionPolicyRef => value('retention_policy_ref') as String?;
  JsonObject? get sourceSubject => value('source_subject') == null
      ? null
      : jsonObject(value('source_subject'));
  JsonObject? get targetSubject => value('target_subject') == null
      ? null
      : jsonObject(value('target_subject'));
  String? get authorityReference => value('authority_reference') as String?;
  String? get sourceOrderRef => value('source_order_ref') as String?;
  String? get orderState => value('order_state') as String?;
  String? get completedAt => value('completed_at') as String?;
  String? get currency => value('currency') as String?;
  int? get grossAmountMinor => value('gross_amount_minor') as int?;
  int? get netAmountMinor => value('net_amount_minor') as int?;
  int? get refundedAmountMinor => value('refunded_amount_minor') as int?;
  String? get branchRef => value('branch_ref') as String?;
  String? get capabilityType => value('capability_type') as String?;
  String? get acquisitionSourceType =>
      value('acquisition_source_type') as String?;
  String? get acquisitionSourceRef =>
      value('acquisition_source_ref') as String?;
}

class CrmSection8aCrmFoundationErrorEnvelope extends VendorPayload {
  CrmSection8aCrmFoundationErrorEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}

class CrmSection8bCrmTagsSegmentationPagination extends VendorPayload {
  CrmSection8bCrmTagsSegmentationPagination.fromJson(super.json);
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8bCrmTagsSegmentationScopeSelector extends VendorPayload {
  CrmSection8bCrmTagsSegmentationScopeSelector.fromJson(super.json);
  String? get type => value('type') as String?;
  String? get branchRef => value('branch_ref') as String?;
}

class CrmSection8bCrmTagsSegmentationTag extends VendorPayload {
  CrmSection8bCrmTagsSegmentationTag.fromJson(super.json);
  String? get tagRef => value('tag_ref') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get status => value('status') as String?;
  int? get memberCount => value('member_count') as int?;
  String? get createdAt => value('created_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8bCrmTagsSegmentationCreateTagRequest extends VendorPayload {
  CrmSection8bCrmTagsSegmentationCreateTagRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  JsonObject? get scope =>
      value('scope') == null ? null : jsonObject(value('scope'));
}

class CrmSection8bCrmTagsSegmentationUpdateTagRequest extends VendorPayload {
  CrmSection8bCrmTagsSegmentationUpdateTagRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8bCrmTagsSegmentationExpectedVersionRequest
    extends VendorPayload {
  CrmSection8bCrmTagsSegmentationExpectedVersionRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8bCrmTagsSegmentationBulkTagRequest extends VendorPayload {
  CrmSection8bCrmTagsSegmentationBulkTagRequest.fromJson(super.json);
  String? get tagRef => value('tag_ref') as String?;
  List<dynamic>? get profileRefs => value('profile_refs') as List<dynamic>?;
}

class CrmSection8bCrmTagsSegmentationBulkProfileRequest extends VendorPayload {
  CrmSection8bCrmTagsSegmentationBulkProfileRequest.fromJson(super.json);
  List<dynamic>? get profileRefs => value('profile_refs') as List<dynamic>?;
}

class CrmSection8bCrmTagsSegmentationBulkResult extends VendorPayload {
  CrmSection8bCrmTagsSegmentationBulkResult.fromJson(super.json);
  String? get tagRef => value('tag_ref') as String?;
  int? get requested => value('requested') as int?;
  int? get applied => value('applied') as int?;
  int? get skipped => value('skipped') as int?;
}

class CrmSection8bCrmTagsSegmentationSegmentField extends VendorPayload {
  CrmSection8bCrmTagsSegmentationSegmentField.fromJson(super.json);
  String? get key => value('key') as String?;
  String? get label => value('label') as String?;
  String? get description => value('description') as String?;
  String? get type => value('type') as String?;
  String? get source => value('source') as String?;
  int? get version => value('version') as int?;
  List<dynamic>? get scopes => value('scopes') as List<dynamic>?;
  List<dynamic>? get operators => value('operators') as List<dynamic>?;
  List<dynamic>? get values => value('values') as List<dynamic>?;
  bool? get available => value('available') as bool?;
  String? get nullableBehavior => value('nullable_behavior') as String?;
  String? get branchBehavior => value('branch_behavior') as String?;
  String? get privacyBehavior => value('privacy_behavior') as String?;
  bool? get branchOnly => value('branch_only') as bool?;
}

class CrmSection8bCrmTagsSegmentationRuleLimits extends VendorPayload {
  CrmSection8bCrmTagsSegmentationRuleLimits.fromJson(super.json);
  int? get maxDepth => value('max_depth') as int?;
  int? get maxNodes => value('max_nodes') as int?;
  int? get maxOrChildren => value('max_or_children') as int?;
  int? get maxInValues => value('max_in_values') as int?;
  int? get maxNotDepth => value('max_not_depth') as int?;
  int? get maxTagReferences => value('max_tag_references') as int?;
}

class CrmSection8bCrmTagsSegmentationRuleDocument extends VendorPayload {
  CrmSection8bCrmTagsSegmentationRuleDocument.fromJson(super.json);
  int? get schemaVersion => value('schema_version') as int?;
  Object? get root => value('root');
  Object? get exclusion => value('exclusion');
}

class CrmSection8bCrmTagsSegmentationRuleGroup extends VendorPayload {
  CrmSection8bCrmTagsSegmentationRuleGroup.fromJson(super.json);
  String? get op => value('op') as String?;
  List<dynamic>? get children => value('children') as List<dynamic>?;
}

class CrmSection8bCrmTagsSegmentationRuleCondition extends VendorPayload {
  CrmSection8bCrmTagsSegmentationRuleCondition.fromJson(super.json);
  String? get field => value('field') as String?;
  String? get operator => value('operator') as String?;
  Object? get valueValue => value('value');
}

class CrmSection8bCrmTagsSegmentationMoneyValue extends VendorPayload {
  CrmSection8bCrmTagsSegmentationMoneyValue.fromJson(super.json);
  String? get currency => value('currency') as String?;
  int? get minor => value('minor') as int?;
}

class CrmSection8bCrmTagsSegmentationDurationWindow extends VendorPayload {
  CrmSection8bCrmTagsSegmentationDurationWindow.fromJson(super.json);
  int? get amount => value('amount') as int?;
  String? get unit => value('unit') as String?;
}

class CrmSection8bCrmTagsSegmentationTagReferenceValue extends VendorPayload {
  CrmSection8bCrmTagsSegmentationTagReferenceValue.fromJson(super.json);
  String? get tagId => value('tag_id') as String?;
}

class CrmSection8bCrmTagsSegmentationSegment extends VendorPayload {
  CrmSection8bCrmTagsSegmentationSegment.fromJson(super.json);
  String? get segmentRef => value('segment_ref') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get segmentType => value('segment_type') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get status => value('status') as String?;
  int? get definitionVersion => value('definition_version') as int?;
  String? get definitionHash => value('definition_hash') as String?;
  String? get evaluationStatus => value('evaluation_status') as String?;
  int? get memberCount => value('member_count') as int?;
  int? get materializedDefinitionVersion =>
      value('materialized_definition_version') as int?;
  String? get lastEvaluatedAt => value('last_evaluated_at') as String?;
  String? get staleSince => value('stale_since') as String?;
  String? get lastEvaluationErrorCode =>
      value('last_evaluation_error_code') as String?;
  String? get ruleExplanation => value('rule_explanation') as String?;
  Object? get rule => value('rule');
  Object? get exclusionRule => value('exclusion_rule');
  String? get createdAt => value('created_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8bCrmTagsSegmentationCreateSegmentRequest
    extends VendorPayload {
  CrmSection8bCrmTagsSegmentationCreateSegmentRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  String? get segmentType => value('segment_type') as String?;
  JsonObject? get scope =>
      value('scope') == null ? null : jsonObject(value('scope'));
  JsonObject? get rule =>
      value('rule') == null ? null : jsonObject(value('rule'));
}

class CrmSection8bCrmTagsSegmentationUpdateSegmentRequest
    extends VendorPayload {
  CrmSection8bCrmTagsSegmentationUpdateSegmentRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  JsonObject? get rule =>
      value('rule') == null ? null : jsonObject(value('rule'));
  String? get changeReason => value('change_reason') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8bCrmTagsSegmentationPreviewRequest extends VendorPayload {
  CrmSection8bCrmTagsSegmentationPreviewRequest.fromJson(super.json);
  JsonObject? get scope =>
      value('scope') == null ? null : jsonObject(value('scope'));
  JsonObject? get rule =>
      value('rule') == null ? null : jsonObject(value('rule'));
  int? get sampleLimit => value('sample_limit') as int?;
}

class CrmSection8bCrmTagsSegmentationPreviewResult extends VendorPayload {
  CrmSection8bCrmTagsSegmentationPreviewResult.fromJson(super.json);
  String? get definitionHash => value('definition_hash') as String?;
  String? get evaluationAsOf => value('evaluation_as_of') as String?;
  int? get matchCount => value('match_count') as int?;
  String? get countState => value('count_state') as String?;
  String? get scope => value('scope') as String?;
  String? get branchRef => value('branch_ref') as String?;
  List<dynamic>? get sample => value('sample') as List<dynamic>?;
  String? get ruleExplanation => value('rule_explanation') as String?;
  List<dynamic>? get unavailableFields =>
      value('unavailable_fields') as List<dynamic>?;
  List<dynamic>? get warnings => value('warnings') as List<dynamic>?;
}

class CrmSection8bCrmTagsSegmentationPreviewSample extends VendorPayload {
  CrmSection8bCrmTagsSegmentationPreviewSample.fromJson(super.json);
  String? get profileRef => value('profile_ref') as String?;
  String? get displayLabel => value('display_label') as String?;
  String? get commercialProjectionState =>
      value('commercial_projection_state') as String?;
  List<dynamic>? get matchedConditions =>
      value('matched_conditions') as List<dynamic>?;
  List<dynamic>? get unavailableConditions =>
      value('unavailable_conditions') as List<dynamic>?;
}

class CrmSection8bCrmTagsSegmentationSegmentMember extends VendorPayload {
  CrmSection8bCrmTagsSegmentationSegmentMember.fromJson(super.json);
  String? get profileRef => value('profile_ref') as String?;
  String? get displayLabel => value('display_label') as String?;
  String? get subjectType => value('subject_type') as String?;
  String? get commercialProjectionState =>
      value('commercial_projection_state') as String?;
  String? get membershipSource => value('membership_source') as String?;
  int? get generation => value('generation') as int?;
  int? get definitionVersion => value('definition_version') as int?;
  String? get matchedAt => value('matched_at') as String?;
}

class CrmSection8bCrmTagsSegmentationMembershipState extends VendorPayload {
  CrmSection8bCrmTagsSegmentationMembershipState.fromJson(super.json);
  String? get evaluationStatus => value('evaluation_status') as String?;
  int? get memberCount => value('member_count') as int?;
  int? get definitionVersion => value('definition_version') as int?;
  String? get lastEvaluatedAt => value('last_evaluated_at') as String?;
  String? get staleSince => value('stale_since') as String?;
}

class CrmSection8bCrmTagsSegmentationEvaluation extends VendorPayload {
  CrmSection8bCrmTagsSegmentationEvaluation.fromJson(super.json);
  String? get evaluationRef => value('evaluation_ref') as String?;
  String? get segmentRef => value('segment_ref') as String?;
  int? get definitionVersion => value('definition_version') as int?;
  int? get generation => value('generation') as int?;
  String? get status => value('status') as String?;
  String? get evaluationAsOf => value('evaluation_as_of') as String?;
  int? get memberCount => value('member_count') as int?;
  String? get errorCode => value('error_code') as String?;
  String? get completedAt => value('completed_at') as String?;
  int? get attemptCount => value('attempt_count') as int?;
}

class CrmSection8bCrmTagsSegmentationErrorEnvelope extends VendorPayload {
  CrmSection8bCrmTagsSegmentationErrorEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}

class CrmSection8cCrmInteractionsTasksInteraction extends VendorPayload {
  CrmSection8cCrmInteractionsTasksInteraction.fromJson(super.json);
  String? get interactionRef => value('interaction_ref') as String?;
  String? get customerProfileRef => value('customer_profile_ref') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get type => value('type') as String?;
  String? get direction => value('direction') as String?;
  String? get outcome => value('outcome') as String?;
  String? get subject => value('subject') as String?;
  String? get note => value('note') as String?;
  String? get visibility => value('visibility') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get recordedByStaffRef => value('recorded_by_staff_ref') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8cCrmInteractionsTasksInteractionRevision
    extends VendorPayload {
  CrmSection8cCrmInteractionsTasksInteractionRevision.fromJson(super.json);
  String? get revisionRef => value('revision_ref') as String?;
  int? get version => value('version') as int?;
  String? get subject => value('subject') as String?;
  String? get note => value('note') as String?;
  String? get outcome => value('outcome') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get correctionReason => value('correction_reason') as String?;
  String? get changedByStaffRef => value('changed_by_staff_ref') as String?;
  String? get createdAt => value('created_at') as String?;
}

class CrmSection8cCrmInteractionsTasksTask extends VendorPayload {
  CrmSection8cCrmInteractionsTasksTask.fromJson(super.json);
  String? get taskRef => value('task_ref') as String?;
  String? get customerProfileRef => value('customer_profile_ref') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get visibility => value('visibility') as String?;
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  String? get status => value('status') as String?;
  String? get priority => value('priority') as String?;
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  bool? get assigneeActive => value('assignee_active') as bool?;
  String? get createdByStaffRef => value('created_by_staff_ref') as String?;
  String? get dueAt => value('due_at') as String?;
  String? get completedAt => value('completed_at') as String?;
  String? get cancelledAt => value('cancelled_at') as String?;
  String? get sourceInteractionRef =>
      value('source_interaction_ref') as String?;
  bool? get isOverdue => value('is_overdue') as bool?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8cCrmInteractionsTasksTaskHistory extends VendorPayload {
  CrmSection8cCrmInteractionsTasksTaskHistory.fromJson(super.json);
  String? get historyRef => value('history_ref') as String?;
  String? get action => value('action') as String?;
  String? get status => value('status') as String?;
  String? get priority => value('priority') as String?;
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  String? get dueAt => value('due_at') as String?;
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  int? get version => value('version') as int?;
  String? get changedByStaffRef => value('changed_by_staff_ref') as String?;
  String? get occurredAt => value('occurred_at') as String?;
}

class CrmSection8cCrmInteractionsTasksCreateInteractionRequest
    extends VendorPayload {
  CrmSection8cCrmInteractionsTasksCreateInteractionRequest.fromJson(super.json);
  String? get customerProfileRef => value('customer_profile_ref') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get type => value('type') as String?;
  String? get direction => value('direction') as String?;
  String? get outcome => value('outcome') as String?;
  String? get subject => value('subject') as String?;
  String? get note => value('note') as String?;
  String? get visibility => value('visibility') as String?;
  String? get occurredAt => value('occurred_at') as String?;
}

class CrmSection8cCrmInteractionsTasksCorrectInteractionRequest
    extends VendorPayload {
  CrmSection8cCrmInteractionsTasksCorrectInteractionRequest.fromJson(
    super.json,
  );
  String? get subject => value('subject') as String?;
  String? get note => value('note') as String?;
  String? get outcome => value('outcome') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get correctionReason => value('correction_reason') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8cCrmInteractionsTasksCreateTaskRequest extends VendorPayload {
  CrmSection8cCrmInteractionsTasksCreateTaskRequest.fromJson(super.json);
  String? get customerProfileRef => value('customer_profile_ref') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get visibility => value('visibility') as String?;
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  String? get priority => value('priority') as String?;
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  String? get dueAt => value('due_at') as String?;
  String? get sourceInteractionRef =>
      value('source_interaction_ref') as String?;
}

class CrmSection8cCrmInteractionsTasksUpdateTaskRequest extends VendorPayload {
  CrmSection8cCrmInteractionsTasksUpdateTaskRequest.fromJson(super.json);
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  String? get priority => value('priority') as String?;
  String? get dueAt => value('due_at') as String?;
  bool? get clearDueAt => value('clear_due_at') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8cCrmInteractionsTasksVersionCommandRequest
    extends VendorPayload {
  CrmSection8cCrmInteractionsTasksVersionCommandRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8cCrmInteractionsTasksReassignTaskRequest
    extends VendorPayload {
  CrmSection8cCrmInteractionsTasksReassignTaskRequest.fromJson(super.json);
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8cCrmInteractionsTasksPagination extends VendorPayload {
  CrmSection8cCrmInteractionsTasksPagination.fromJson(super.json);
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8cCrmInteractionsTasksInteractionEnvelope
    extends VendorPayload {
  CrmSection8cCrmInteractionsTasksInteractionEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class CrmSection8cCrmInteractionsTasksInteractionListEnvelope
    extends VendorPayload {
  CrmSection8cCrmInteractionsTasksInteractionListEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class CrmSection8cCrmInteractionsTasksInteractionRevisionListEnvelope
    extends VendorPayload {
  CrmSection8cCrmInteractionsTasksInteractionRevisionListEnvelope.fromJson(
    super.json,
  );
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class CrmSection8cCrmInteractionsTasksTaskEnvelope extends VendorPayload {
  CrmSection8cCrmInteractionsTasksTaskEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class CrmSection8cCrmInteractionsTasksTaskListEnvelope extends VendorPayload {
  CrmSection8cCrmInteractionsTasksTaskListEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class CrmSection8cCrmInteractionsTasksTaskHistoryListEnvelope
    extends VendorPayload {
  CrmSection8cCrmInteractionsTasksTaskHistoryListEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class CrmSection8cCrmInteractionsTasksErrorEnvelope extends VendorPayload {
  CrmSection8cCrmInteractionsTasksErrorEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}

class CrmSection8dCrmLoyaltyLoyaltyProgram extends VendorPayload {
  CrmSection8dCrmLoyaltyLoyaltyProgram.fromJson(super.json);
  String? get status => value('status') as String?;
  String? get pointsNameSingular => value('points_name_singular') as String?;
  String? get pointsNamePlural => value('points_name_plural') as String?;
  String? get expiryPolicy => value('expiry_policy') as String?;
  int? get expiryDays => value('expiry_days') as int?;
  int? get minimumRedemptionPoints =>
      value('minimum_redemption_points') as int?;
  int? get redemptionIncrementPoints =>
      value('redemption_increment_points') as int?;
  int? get manualAdjustmentLimit => value('manual_adjustment_limit') as int?;
  bool? get configured => value('configured') as bool?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8dCrmLoyaltyUpdateLoyaltyProgramRequest extends VendorPayload {
  CrmSection8dCrmLoyaltyUpdateLoyaltyProgramRequest.fromJson(super.json);
  String? get status => value('status') as String?;
  String? get pointsNameSingular => value('points_name_singular') as String?;
  String? get pointsNamePlural => value('points_name_plural') as String?;
  String? get expiryPolicy => value('expiry_policy') as String?;
  int? get expiryDays => value('expiry_days') as int?;
  bool? get clearExpiryDays => value('clear_expiry_days') as bool?;
  int? get minimumRedemptionPoints =>
      value('minimum_redemption_points') as int?;
  int? get redemptionIncrementPoints =>
      value('redemption_increment_points') as int?;
  int? get manualAdjustmentLimit => value('manual_adjustment_limit') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8dCrmLoyaltyLoyaltySummary extends VendorPayload {
  CrmSection8dCrmLoyaltyLoyaltySummary.fromJson(super.json);
  String? get programStatus => value('program_status') as String?;
  String? get enrollment => value('enrollment') as String?;
  String? get accountStatus => value('account_status') as String?;
  String? get pointsNameSingular => value('points_name_singular') as String?;
  String? get pointsNamePlural => value('points_name_plural') as String?;
  int? get availablePoints => value('available_points') as int?;
  int? get lifetimeCreditedPoints => value('lifetime_credited_points') as int?;
  int? get lifetimeDebitedPoints => value('lifetime_debited_points') as int?;
  int? get pendingExpiryPoints => value('pending_expiry_points') as int?;
  String? get nextExpiryAt => value('next_expiry_at') as String?;
  int? get pointsExpiringNext => value('points_expiring_next') as int?;
  int? get version => value('version') as int?;
}

class CrmSection8dCrmLoyaltyLoyaltyEntry extends VendorPayload {
  CrmSection8dCrmLoyaltyLoyaltyEntry.fromJson(super.json);
  String? get entryRef => value('entry_ref') as String?;
  int? get sequence => value('sequence') as int?;
  String? get entryType => value('entry_type') as String?;
  int? get deltaPoints => value('delta_points') as int?;
  int? get balanceAfter => value('balance_after') as int?;
  String? get sourceType => value('source_type') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get reversesEntryRef => value('reverses_entry_ref') as String?;
  String? get reversedByEntryRef => value('reversed_by_entry_ref') as String?;
  String? get createdByStaffRef => value('created_by_staff_ref') as String?;
  String? get createdAt => value('created_at') as String?;
}

class CrmSection8dCrmLoyaltyLoyaltyEntryPage extends VendorPayload {
  CrmSection8dCrmLoyaltyLoyaltyEntryPage.fromJson(super.json);
  List<dynamic>? get entries => value('entries') as List<dynamic>?;
  bool? get hasMore => value('has_more') as bool?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
}

class CrmSection8dCrmLoyaltyAdjustLoyaltyRequest extends VendorPayload {
  CrmSection8dCrmLoyaltyAdjustLoyaltyRequest.fromJson(super.json);
  String? get direction => value('direction') as String?;
  int? get points => value('points') as int?;
  String? get reason => value('reason') as String?;
}

class CrmSection8dCrmLoyaltyAdjustLoyaltyResponse extends VendorPayload {
  CrmSection8dCrmLoyaltyAdjustLoyaltyResponse.fromJson(super.json);
  JsonObject? get entry =>
      value('entry') == null ? null : jsonObject(value('entry'));
  JsonObject? get summary =>
      value('summary') == null ? null : jsonObject(value('summary'));
}

class CrmSection8dCrmLoyaltyReverseLoyaltyEntryRequest extends VendorPayload {
  CrmSection8dCrmLoyaltyReverseLoyaltyEntryRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
}

class CrmSection8dCrmLoyaltyErrorEnvelope extends VendorPayload {
  CrmSection8dCrmLoyaltyErrorEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}

class CrmSection8eCrmCampaignIntelligenceExperimentVariant
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceExperimentVariant.fromJson(super.json);
  String? get variantKey => value('variant_key') as String?;
  String? get displayName => value('display_name') as String?;
  int? get allocationBps => value('allocation_bps') as int?;
  bool? get isControl => value('is_control') as bool?;
}

class CrmSection8eCrmCampaignIntelligenceFrequencyPolicy extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceFrequencyPolicy.fromJson(super.json);
  bool? get enabled => value('enabled') as bool?;
  int? get cap => value('cap') as int?;
  int? get windowMinutes => value('window_minutes') as int?;
  String? get evaluationState => value('evaluation_state') as String?;
  bool? get sendTimeRecheckRequired =>
      value('send_time_recheck_required') as bool?;
}

class CrmSection8eCrmCampaignIntelligenceQuietHoursPolicy
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceQuietHoursPolicy.fromJson(super.json);
  bool? get enabled => value('enabled') as bool?;
  String? get start => value('start') as String?;
  String? get end => value('end') as String?;
  String? get timezone => value('timezone') as String?;
  bool? get crossesMidnight => value('crosses_midnight') as bool?;
  String? get hypotheticalSendAt => value('hypothetical_send_at') as String?;
  bool? get wouldBlock => value('would_block') as bool?;
  bool? get sendTimeRecheckRequired =>
      value('send_time_recheck_required') as bool?;
}

class CrmSection8eCrmCampaignIntelligenceCampaign extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaign.fromJson(super.json);
  String? get campaignRef => value('campaign_ref') as String?;
  String? get name => value('name') as String?;
  String? get objective => value('objective') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get status => value('status') as String?;
  int? get definitionVersion => value('definition_version') as int?;
  String? get currentSnapshotRef => value('current_snapshot_ref') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignDefinition
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignDefinition.fromJson(super.json);
  int? get definitionVersion => value('definition_version') as int?;
  String? get definitionHash => value('definition_hash') as String?;
  String? get name => value('name') as String?;
  String? get objective => value('objective') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  List<dynamic>? get channels => value('channels') as List<dynamic>?;
  String? get sourceSegmentRef => value('source_segment_ref') as String?;
  List<dynamic>? get exclusionSegmentRefs =>
      value('exclusion_segment_refs') as List<dynamic>?;
  JsonObject? get frequencyPolicy => value('frequency_policy') == null
      ? null
      : jsonObject(value('frequency_policy'));
  JsonObject? get quietHoursPolicy => value('quiet_hours_policy') == null
      ? null
      : jsonObject(value('quiet_hours_policy'));
  bool? get serviceRecoverySuppressionEnabled =>
      value('service_recovery_suppression_enabled') as bool?;
  bool? get experimentEnabled => value('experiment_enabled') as bool?;
  List<dynamic>? get experimentVariants =>
      value('experiment_variants') as List<dynamic>?;
  String? get createdAt => value('created_at') as String?;
  String? get changeReason => value('change_reason') as String?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignWithDefinition
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignWithDefinition.fromJson(
    super.json,
  );
  JsonObject? get campaign =>
      value('campaign') == null ? null : jsonObject(value('campaign'));
  JsonObject? get definition =>
      value('definition') == null ? null : jsonObject(value('definition'));
}

class CrmSection8eCrmCampaignIntelligenceCampaignPage extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignPage.fromJson(super.json);
  List<dynamic>? get campaigns => value('campaigns') as List<dynamic>?;
  bool? get hasMore => value('has_more') as bool?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignDefinitionRequest
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignDefinitionRequest.fromJson(
    super.json,
  );
  String? get name => value('name') as String?;
  String? get objective => value('objective') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  List<dynamic>? get channels => value('channels') as List<dynamic>?;
  String? get sourceSegmentRef => value('source_segment_ref') as String?;
  List<dynamic>? get exclusionSegmentRefs =>
      value('exclusion_segment_refs') as List<dynamic>?;
  int? get frequencyCapCount => value('frequency_cap_count') as int?;
  int? get frequencyCapWindowMinutes =>
      value('frequency_cap_window_minutes') as int?;
  bool? get quietHoursEnabled => value('quiet_hours_enabled') as bool?;
  String? get quietHoursStart => value('quiet_hours_start') as String?;
  String? get quietHoursEnd => value('quiet_hours_end') as String?;
  String? get quietHoursTimezone => value('quiet_hours_timezone') as String?;
  bool? get serviceRecoverySuppressionEnabled =>
      value('service_recovery_suppression_enabled') as bool?;
  bool? get experimentEnabled => value('experiment_enabled') as bool?;
  List<dynamic>? get experimentVariants =>
      value('experiment_variants') as List<dynamic>?;
}

class CrmSection8eCrmCampaignIntelligenceUpdateCampaignRequest
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceUpdateCampaignRequest.fromJson(super.json);
  String? get name => value('name') as String?;
  String? get objective => value('objective') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  List<dynamic>? get channels => value('channels') as List<dynamic>?;
  String? get sourceSegmentRef => value('source_segment_ref') as String?;
  List<dynamic>? get exclusionSegmentRefs =>
      value('exclusion_segment_refs') as List<dynamic>?;
  int? get frequencyCapCount => value('frequency_cap_count') as int?;
  int? get frequencyCapWindowMinutes =>
      value('frequency_cap_window_minutes') as int?;
  bool? get quietHoursEnabled => value('quiet_hours_enabled') as bool?;
  String? get quietHoursStart => value('quiet_hours_start') as String?;
  String? get quietHoursEnd => value('quiet_hours_end') as String?;
  String? get quietHoursTimezone => value('quiet_hours_timezone') as String?;
  bool? get serviceRecoverySuppressionEnabled =>
      value('service_recovery_suppression_enabled') as bool?;
  bool? get experimentEnabled => value('experiment_enabled') as bool?;
  List<dynamic>? get experimentVariants =>
      value('experiment_variants') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get changeReason => value('change_reason') as String?;
}

class CrmSection8eCrmCampaignIntelligenceArchiveCampaignRequest
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceArchiveCampaignRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8eCrmCampaignIntelligencePreviewCampaignRequest
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligencePreviewCampaignRequest.fromJson(
    super.json,
  );
  int? get sampleLimit => value('sample_limit') as int?;
  String? get hypotheticalSendAt => value('hypothetical_send_at') as String?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignChannelProjection
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignChannelProjection.fromJson(
    super.json,
  );
  String? get channel => value('channel') as String?;
  int? get consentEligibleCount => value('consent_eligible_count') as int?;
  int? get consentIneligibleCount => value('consent_ineligible_count') as int?;
  JsonObject? get consentIneligibleReasons =>
      value('consent_ineligible_reasons') == null
      ? null
      : jsonObject(value('consent_ineligible_reasons'));
  bool? get sendTimeRecheckRequired =>
      value('send_time_recheck_required') as bool?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignPreviewSample
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignPreviewSample.fromJson(super.json);
  String? get profileRef => value('profile_ref') as String?;
  String? get displayLabel => value('display_label') as String?;
  String? get reasonCode => value('reason_code') as String?;
  String? get experimentVariantKey =>
      value('experiment_variant_key') as String?;
  bool? get isControl => value('is_control') as bool?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignPreview extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignPreview.fromJson(super.json);
  String? get campaignRef => value('campaign_ref') as String?;
  int? get definitionVersion => value('definition_version') as int?;
  String? get definitionHash => value('definition_hash') as String?;
  String? get evaluationAsOf => value('evaluation_as_of') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get sourceSegmentRef => value('source_segment_ref') as String?;
  String? get sourceSegmentEvaluationStatus =>
      value('source_segment_evaluation_status') as String?;
  int? get sourceSegmentDefinitionVersion =>
      value('source_segment_definition_version') as int?;
  int? get sourceSegmentGeneration =>
      value('source_segment_generation') as int?;
  List<dynamic>? get exclusionSegmentRefs =>
      value('exclusion_segment_refs') as List<dynamic>?;
  String? get countState => value('count_state') as String?;
  int? get sourceCount => value('source_count') as int?;
  int? get segmentExclusionCount => value('segment_exclusion_count') as int?;
  int? get privacySuppressedCount => value('privacy_suppressed_count') as int?;
  int? get serviceRecoverySuppressedCount =>
      value('service_recovery_suppressed_count') as int?;
  int? get consentSuppressedCount => value('consent_suppressed_count') as int?;
  int? get targetCount => value('target_count') as int?;
  int? get controlCount => value('control_count') as int?;
  int? get treatmentCount => value('treatment_count') as int?;
  List<dynamic>? get channels => value('channels') as List<dynamic>?;
  JsonObject? get experimentVariantCounts =>
      value('experiment_variant_counts') == null
      ? null
      : jsonObject(value('experiment_variant_counts'));
  JsonObject? get frequencyPolicy => value('frequency_policy') == null
      ? null
      : jsonObject(value('frequency_policy'));
  JsonObject? get quietHoursPolicy => value('quiet_hours_policy') == null
      ? null
      : jsonObject(value('quiet_hours_policy'));
  bool? get serviceRecoverySuppressionEnabled =>
      value('service_recovery_suppression_enabled') as bool?;
  List<dynamic>? get sample => value('sample') as List<dynamic>?;
  List<dynamic>? get warnings => value('warnings') as List<dynamic>?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignSnapshot
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignSnapshot.fromJson(super.json);
  String? get snapshotRef => value('snapshot_ref') as String?;
  String? get campaignRef => value('campaign_ref') as String?;
  int? get campaignDefinitionVersion =>
      value('campaign_definition_version') as int?;
  String? get definitionHash => value('definition_hash') as String?;
  String? get audienceHash => value('audience_hash') as String?;
  String? get status => value('status') as String?;
  String? get evaluationAsOf => value('evaluation_as_of') as String?;
  String? get sourceSegmentRef => value('source_segment_ref') as String?;
  int? get sourceSegmentDefinitionVersion =>
      value('source_segment_definition_version') as int?;
  int? get sourceSegmentGeneration =>
      value('source_segment_generation') as int?;
  int? get sourceCount => value('source_count') as int?;
  int? get segmentExclusionCount => value('segment_exclusion_count') as int?;
  int? get privacySuppressedCount => value('privacy_suppressed_count') as int?;
  int? get serviceRecoverySuppressedCount =>
      value('service_recovery_suppressed_count') as int?;
  int? get consentSuppressedCount => value('consent_suppressed_count') as int?;
  int? get targetCount => value('target_count') as int?;
  int? get controlCount => value('control_count') as int?;
  int? get treatmentCount => value('treatment_count') as int?;
  List<dynamic>? get channels => value('channels') as List<dynamic>?;
  bool? get sendTimeConsentRecheckRequired =>
      value('send_time_consent_recheck_required') as bool?;
  bool? get sendTimeFrequencyRecheckRequired =>
      value('send_time_frequency_recheck_required') as bool?;
  bool? get sendTimeQuietHoursRecheckRequired =>
      value('send_time_quiet_hours_recheck_required') as bool?;
  int? get attemptCount => value('attempt_count') as int?;
  bool? get deadLettered => value('dead_lettered') as bool?;
  String? get errorCode => value('error_code') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get completedAt => value('completed_at') as String?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignSnapshotPage
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignSnapshotPage.fromJson(super.json);
  List<dynamic>? get snapshots => value('snapshots') as List<dynamic>?;
  bool? get hasMore => value('has_more') as bool?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignTargetChannel
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignTargetChannel.fromJson(super.json);
  String? get channel => value('channel') as String?;
  String? get snapshotConsentDecision =>
      value('snapshot_consent_decision') as String?;
  bool? get snapshotConsentEligible =>
      value('snapshot_consent_eligible') as bool?;
  String? get consentAsOf => value('consent_as_of') as String?;
  int? get consentSourceVersion => value('consent_source_version') as int?;
  String? get snapshotEligibilityState =>
      value('snapshot_eligibility_state') as String?;
  String? get reasonCode => value('reason_code') as String?;
  bool? get sendTimeRecheckRequired =>
      value('send_time_recheck_required') as bool?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignTarget extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignTarget.fromJson(super.json);
  String? get targetRef => value('target_ref') as String?;
  String? get profileRef => value('profile_ref') as String?;
  String? get displayLabel => value('display_label') as String?;
  String? get experimentVariantKey =>
      value('experiment_variant_key') as String?;
  bool? get isControl => value('is_control') as bool?;
  int? get experimentBucket => value('experiment_bucket') as int?;
  bool? get currentlyActionable => value('currently_actionable') as bool?;
  String? get suppressionReasonCode =>
      value('suppression_reason_code') as String?;
  bool? get serviceRecoveryActive => value('service_recovery_active') as bool?;
  List<dynamic>? get channels => value('channels') as List<dynamic>?;
}

class CrmSection8eCrmCampaignIntelligenceCampaignTargetPage
    extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceCampaignTargetPage.fromJson(super.json);
  List<dynamic>? get targets => value('targets') as List<dynamic>?;
  bool? get hasMore => value('has_more') as bool?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get sendTimeRecheckRequired =>
      value('send_time_recheck_required') as bool?;
}

class CrmSection8eCrmCampaignIntelligenceErrorEnvelope extends VendorPayload {
  CrmSection8eCrmCampaignIntelligenceErrorEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}

class CrmSection8fCampaignExecutionCreateRun extends VendorPayload {
  CrmSection8fCampaignExecutionCreateRun.fromJson(super.json);
  String? get snapshotRef => value('snapshot_ref') as String?;
  String? get mode => value('mode') as String?;
  String? get scheduledFor => value('scheduled_for') as String?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get templateKey => value('template_key') as String?;
  String? get locale => value('locale') as String?;
  String? get title => value('title') as String?;
  String? get body => value('body') as String?;
}

class CrmSection8fCampaignExecutionRun extends VendorPayload {
  CrmSection8fCampaignExecutionRun.fromJson(super.json);
  String? get runRef => value('run_ref') as String?;
  String? get campaignRef => value('campaign_ref') as String?;
  String? get campaignVersionRef => value('campaign_version_ref') as String?;
  String? get snapshotRef => value('snapshot_ref') as String?;
  String? get status => value('status') as String?;
  String? get scheduledFor => value('scheduled_for') as String?;
  int? get totalDeliveries => value('total_deliveries') as int?;
  int? get handedOffCount => value('handed_off_count') as int?;
  int? get deliveredCount => value('delivered_count') as int?;
  int? get suppressedCount => value('suppressed_count') as int?;
  int? get failedCount => value('failed_count') as int?;
  int? get version => value('version') as int?;
}

class CrmSection8fCampaignExecutionDelivery extends VendorPayload {
  CrmSection8fCampaignExecutionDelivery.fromJson(super.json);
  String? get deliveryRef => value('delivery_ref') as String?;
  String? get runRef => value('run_ref') as String?;
  String? get targetRef => value('target_ref') as String?;
  String? get channel => value('channel') as String?;
  String? get status => value('status') as String?;
  String? get suppressionReason => value('suppression_reason') as String?;
  String? get attributionRef => value('attribution_ref') as String?;
  String? get consentDecisionAtHandoff =>
      value('consent_decision_at_handoff') as String?;
  bool? get consentEligibleAtHandoff =>
      value('consent_eligible_at_handoff') as bool?;
  String? get consentCheckedAt => value('consent_checked_at') as String?;
  String? get consentAsOf => value('consent_as_of') as String?;
  int? get consentSourceVersion => value('consent_source_version') as int?;
  String? get policyCheckedAt => value('policy_checked_at') as String?;
}

class CrmSection8fCampaignExecutionDeliveryEvent extends VendorPayload {
  CrmSection8fCampaignExecutionDeliveryEvent.fromJson(super.json);
  String? get eventId => value('event_id') as String?;
  String? get deliveryRef => value('delivery_ref') as String?;
  String? get status => value('status') as String?;
  String? get reasonCode => value('reason_code') as String?;
  int? get sourceVersion => value('source_version') as int?;
  String? get occurredAt => value('occurred_at') as String?;
}

class CrmSection8fCampaignExecutionRunEnvelope extends VendorPayload {
  CrmSection8fCampaignExecutionRunEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class CrmSection8fCampaignExecutionDeliveryEnvelope extends VendorPayload {
  CrmSection8fCampaignExecutionDeliveryEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get data =>
      value('data') == null ? null : jsonObject(value('data'));
}

class CrmSection8gCrmCorporateCustomersCorporateCustomer extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateCustomer.fromJson(super.json);
  String? get corporateRef => value('corporate_ref') as String?;
  String? get corporateCustomerCode =>
      value('corporate_customer_code') as String?;
  String? get organizationName => value('organization_name') as String?;
  String? get displayName => value('display_name') as String?;
  String? get organizationType => value('organization_type') as String?;
  String? get status => value('status') as String?;
  String? get scopeMode => value('scope_mode') as String?;
  List<dynamic>? get branchRefs => value('branch_refs') as List<dynamic>?;
  String? get accountManagerStaffRef =>
      value('account_manager_staff_ref') as String?;
  String? get relationshipStartedAt =>
      value('relationship_started_at') as String?;
  String? get externalOrganizationRef =>
      value('external_organization_ref') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateSummary extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateSummary.fromJson(super.json);
  int? get contactCount => value('contact_count') as int?;
  int? get locationCount => value('location_count') as int?;
  int? get costCenterCount => value('cost_center_count') as int?;
  int? get membershipCount => value('membership_count') as int?;
  int? get openTaskCount => value('open_task_count') as int?;
  int? get overdueTaskCount => value('overdue_task_count') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateCustomerDetail
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateCustomerDetail.fromJson(super.json);
  JsonObject? get corporateCustomer => value('corporate_customer') == null
      ? null
      : jsonObject(value('corporate_customer'));
  JsonObject? get summary =>
      value('summary') == null ? null : jsonObject(value('summary'));
  List<dynamic>? get recentInteractions =>
      value('recent_interactions') as List<dynamic>?;
}

class CrmSection8gCrmCorporateCustomersCorporateCustomerPage
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateCustomerPage.fromJson(super.json);
  List<dynamic>? get corporateCustomers =>
      value('corporate_customers') as List<dynamic>?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8gCrmCorporateCustomersCorporateCustomerRequest
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateCustomerRequest.fromJson(
    super.json,
  );
  String? get organizationName => value('organization_name') as String?;
  String? get displayName => value('display_name') as String?;
  String? get organizationType => value('organization_type') as String?;
  String? get scopeMode => value('scope_mode') as String?;
  List<dynamic>? get branchRefs => value('branch_refs') as List<dynamic>?;
  String? get accountManagerStaffRef =>
      value('account_manager_staff_ref') as String?;
  bool? get clearAccountManager => value('clear_account_manager') as bool?;
  String? get relationshipStartedAt =>
      value('relationship_started_at') as String?;
  String? get externalOrganizationRef =>
      value('external_organization_ref') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateTransitionRequest
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateTransitionRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateBranchScopes
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateBranchScopes.fromJson(super.json);
  String? get scopeMode => value('scope_mode') as String?;
  List<dynamic>? get branchRefs => value('branch_refs') as List<dynamic>?;
  int? get version => value('version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateBranchScopeRequest
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateBranchScopeRequest.fromJson(
    super.json,
  );
  String? get scopeMode => value('scope_mode') as String?;
  List<dynamic>? get branchRefs => value('branch_refs') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateContact extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateContact.fromJson(super.json);
  String? get contactRef => value('contact_ref') as String?;
  String? get customerProfileRef => value('customer_profile_ref') as String?;
  String? get displayLabel => value('display_label') as String?;
  String? get relationshipRole => value('relationship_role') as String?;
  bool? get isPrimaryContact => value('is_primary_contact') as bool?;
  String? get status => value('status') as String?;
  bool? get privacySuppressed => value('privacy_suppressed') as bool?;
  bool? get actionable => value('actionable') as bool?;
  String? get createdAt => value('created_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateContactPage
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateContactPage.fromJson(super.json);
  List<dynamic>? get contacts => value('contacts') as List<dynamic>?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8gCrmCorporateCustomersCorporateContactRequest
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateContactRequest.fromJson(super.json);
  String? get customerProfileRef => value('customer_profile_ref') as String?;
  String? get relationshipRole => value('relationship_role') as String?;
  bool? get isPrimaryContact => value('is_primary_contact') as bool?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateMembership
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateMembership.fromJson(super.json);
  String? get membershipRef => value('membership_ref') as String?;
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
  String? get membershipRole => value('membership_role') as String?;
  String? get status => value('status') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateMembershipPage
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateMembershipPage.fromJson(super.json);
  List<dynamic>? get memberships => value('memberships') as List<dynamic>?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8gCrmCorporateCustomersCorporateMembershipRequest
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateMembershipRequest.fromJson(
    super.json,
  );
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
  bool? get clearLocation => value('clear_location') as bool?;
  bool? get clearCostCenter => value('clear_cost_center') as bool?;
  String? get membershipRole => value('membership_role') as String?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateLocation extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateLocation.fromJson(super.json);
  String? get locationRef => value('location_ref') as String?;
  String? get locationCode => value('location_code') as String?;
  String? get name => value('name') as String?;
  String? get locationType => value('location_type') as String?;
  String? get status => value('status') as String?;
  String? get addressLine1 => value('address_line1') as String?;
  String? get addressLine2 => value('address_line2') as String?;
  String? get city => value('city') as String?;
  String? get stateRegion => value('state_region') as String?;
  String? get postalCode => value('postal_code') as String?;
  String? get countryCode => value('country_code') as String?;
  String? get timezone => value('timezone') as String?;
  String? get externalLocationRef => value('external_location_ref') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateLocationPage
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateLocationPage.fromJson(super.json);
  List<dynamic>? get locations => value('locations') as List<dynamic>?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8gCrmCorporateCustomersCorporateLocationRequest
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateLocationRequest.fromJson(
    super.json,
  );
  String? get locationCode => value('location_code') as String?;
  String? get name => value('name') as String?;
  String? get locationType => value('location_type') as String?;
  String? get addressLine1 => value('address_line1') as String?;
  String? get addressLine2 => value('address_line2') as String?;
  String? get city => value('city') as String?;
  String? get stateRegion => value('state_region') as String?;
  String? get postalCode => value('postal_code') as String?;
  String? get countryCode => value('country_code') as String?;
  String? get timezone => value('timezone') as String?;
  String? get externalLocationRef => value('external_location_ref') as String?;
  String? get status => value('status') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateCostCenter
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateCostCenter.fromJson(super.json);
  String? get costCenterRef => value('cost_center_ref') as String?;
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get status => value('status') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get externalReference => value('external_reference') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get archivedAt => value('archived_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateCostCenterPage
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateCostCenterPage.fromJson(super.json);
  List<dynamic>? get costCenters => value('cost_centers') as List<dynamic>?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8gCrmCorporateCustomersCorporateCostCenterRequest
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateCostCenterRequest.fromJson(
    super.json,
  );
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get status => value('status') as String?;
  String? get locationRef => value('location_ref') as String?;
  bool? get clearLocation => value('clear_location') as bool?;
  String? get externalReference => value('external_reference') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateInteraction
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateInteraction.fromJson(super.json);
  String? get interactionRef => value('interaction_ref') as String?;
  String? get corporateRef => value('corporate_ref') as String?;
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
  String? get type => value('type') as String?;
  String? get direction => value('direction') as String?;
  String? get outcome => value('outcome') as String?;
  String? get subject => value('subject') as String?;
  String? get note => value('note') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get recordedByStaffRef => value('recorded_by_staff_ref') as String?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateInteractionPage
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateInteractionPage.fromJson(
    super.json,
  );
  List<dynamic>? get interactions => value('interactions') as List<dynamic>?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8gCrmCorporateCustomersCorporateInteractionRequest
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateInteractionRequest.fromJson(
    super.json,
  );
  String? get type => value('type') as String?;
  String? get direction => value('direction') as String?;
  String? get outcome => value('outcome') as String?;
  String? get subject => value('subject') as String?;
  String? get note => value('note') as String?;
  String? get occurredAt => value('occurred_at') as String?;
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
}

class CrmSection8gCrmCorporateCustomersCorporateTask extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateTask.fromJson(super.json);
  String? get taskRef => value('task_ref') as String?;
  String? get corporateRef => value('corporate_ref') as String?;
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  String? get status => value('status') as String?;
  String? get priority => value('priority') as String?;
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  bool? get assigneeActive => value('assignee_active') as bool?;
  String? get createdByStaffRef => value('created_by_staff_ref') as String?;
  String? get dueAt => value('due_at') as String?;
  String? get completedAt => value('completed_at') as String?;
  String? get cancelledAt => value('cancelled_at') as String?;
  String? get sourceInteractionRef =>
      value('source_interaction_ref') as String?;
  bool? get isOverdue => value('is_overdue') as bool?;
  String? get createdAt => value('created_at') as String?;
  String? get updatedAt => value('updated_at') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateTaskPage extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateTaskPage.fromJson(super.json);
  List<dynamic>? get tasks => value('tasks') as List<dynamic>?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8gCrmCorporateCustomersCorporateTaskRequest
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateTaskRequest.fromJson(super.json);
  String? get title => value('title') as String?;
  String? get description => value('description') as String?;
  String? get priority => value('priority') as String?;
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
  String? get dueAt => value('due_at') as String?;
  bool? get clearDueAt => value('clear_due_at') as bool?;
  String? get contactRef => value('contact_ref') as String?;
  String? get locationRef => value('location_ref') as String?;
  String? get costCenterRef => value('cost_center_ref') as String?;
  String? get sourceInteractionRef =>
      value('source_interaction_ref') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8gCrmCorporateCustomersCorporateTaskTransitionRequest
    extends VendorPayload {
  CrmSection8gCrmCorporateCustomersCorporateTaskTransitionRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
  String? get assignedStaffRef => value('assigned_staff_ref') as String?;
}

class CrmSection8gCrmCorporateCustomersError extends VendorPayload {
  CrmSection8gCrmCorporateCustomersError.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}

class CrmSection8hCrmIntelligenceExportsScore extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsScore.fromJson(super.json);
  String? get modelKey => value('model_key') as String?;
  int? get modelVersion => value('model_version') as int?;
  String? get scope => value('scope') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get state => value('state') as String?;
  List<dynamic>? get reasonCodes => value('reason_codes') as List<dynamic>?;
  String? get sourceAsOf => value('source_as_of') as String?;
  String? get computedAt => value('computed_at') as String?;
  bool? get stale => value('stale') as bool?;
  int? get recencyScore => value('recency_score') as int?;
  int? get frequencyScore => value('frequency_score') as int?;
  int? get monetaryScore => value('monetary_score') as int?;
  int? get compositeScore => value('composite_score') as int?;
  int? get riskScore => value('risk_score') as int?;
  String? get riskBand => value('risk_band') as String?;
}

class CrmSection8hCrmIntelligenceExportsCustomerIntelligence
    extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsCustomerIntelligence.fromJson(super.json);
  String? get scope => value('scope') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get rfmState => value('rfm_state') as String?;
  String? get churnState => value('churn_state') as String?;
  JsonObject? get rfm => value('rfm') == null ? null : jsonObject(value('rfm'));
  JsonObject? get churn =>
      value('churn') == null ? null : jsonObject(value('churn'));
}

class CrmSection8hCrmIntelligenceExportsModelAvailability
    extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsModelAvailability.fromJson(super.json);
  String? get modelKey => value('model_key') as String?;
  int? get modelVersion => value('model_version') as int?;
  String? get label => value('label') as String?;
  String? get status => value('status') as String?;
  int? get generation => value('generation') as int?;
  String? get sourceAsOf => value('source_as_of') as String?;
  String? get computedAt => value('computed_at') as String?;
  String? get staleSince => value('stale_since') as String?;
  String? get errorCode => value('error_code') as String?;
  int? get profileCount => value('profile_count') as int?;
  int? get availableCount => value('available_count') as int?;
  int? get unavailableCount => value('unavailable_count') as int?;
}

class CrmSection8hCrmIntelligenceExportsIntelligenceOverview
    extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsIntelligenceOverview.fromJson(super.json);
  String? get scope => value('scope') as String?;
  String? get branchRef => value('branch_ref') as String?;
  List<dynamic>? get models => value('models') as List<dynamic>?;
}

class CrmSection8hCrmIntelligenceExportsDistributionBucket
    extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsDistributionBucket.fromJson(super.json);
  String? get key => value('key') as String?;
  int? get count => value('count') as int?;
}

class CrmSection8hCrmIntelligenceExportsRFMDistribution extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsRFMDistribution.fromJson(super.json);
  JsonObject? get availability =>
      value('availability') == null ? null : jsonObject(value('availability'));
  List<dynamic>? get compositeDistribution =>
      value('composite_distribution') as List<dynamic>?;
  List<dynamic>? get recencyDistribution =>
      value('recency_distribution') as List<dynamic>?;
  List<dynamic>? get frequencyDistribution =>
      value('frequency_distribution') as List<dynamic>?;
  List<dynamic>? get monetaryDistribution =>
      value('monetary_distribution') as List<dynamic>?;
  List<dynamic>? get unavailableReasons =>
      value('unavailable_reasons') as List<dynamic>?;
}

class CrmSection8hCrmIntelligenceExportsChurnDistribution
    extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsChurnDistribution.fromJson(super.json);
  JsonObject? get availability =>
      value('availability') == null ? null : jsonObject(value('availability'));
  List<dynamic>? get bandDistribution =>
      value('band_distribution') as List<dynamic>?;
  List<dynamic>? get unavailableReasons =>
      value('unavailable_reasons') as List<dynamic>?;
}

class CrmSection8hCrmIntelligenceExportsRebuildIntelligenceRequest
    extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsRebuildIntelligenceRequest.fromJson(
    super.json,
  );
  String? get modelKey => value('model_key') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
}

class CrmSection8hCrmIntelligenceExportsIntelligenceEvaluation
    extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsIntelligenceEvaluation.fromJson(super.json);
  String? get evaluationRef => value('evaluation_ref') as String?;
  String? get modelKey => value('model_key') as String?;
  int? get modelVersion => value('model_version') as int?;
  String? get scope => value('scope') as String?;
  String? get branchRef => value('branch_ref') as String?;
  int? get generation => value('generation') as int?;
  String? get status => value('status') as String?;
  String? get sourceAsOf => value('source_as_of') as String?;
  String? get requestedAt => value('requested_at') as String?;
  String? get completedAt => value('completed_at') as String?;
  int? get profileCount => value('profile_count') as int?;
  int? get availableCount => value('available_count') as int?;
  int? get unavailableCount => value('unavailable_count') as int?;
}

class CrmSection8hCrmIntelligenceExportsExportFilter extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsExportFilter.fromJson(super.json);
  String? get segmentRef => value('segment_ref') as String?;
  String? get campaignRef => value('campaign_ref') as String?;
  String? get modelKey => value('model_key') as String?;
  List<dynamic>? get statuses => value('statuses') as List<dynamic>?;
  List<dynamic>? get riskBands => value('risk_bands') as List<dynamic>?;
  List<dynamic>? get scoreStates => value('score_states') as List<dynamic>?;
}

class CrmSection8hCrmIntelligenceExportsExport extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsExport.fromJson(super.json);
  String? get exportRef => value('export_ref') as String?;
  String? get exportType => value('export_type') as String?;
  String? get format => value('format') as String?;
  String? get status => value('status') as String?;
  String? get scope => value('scope') as String?;
  String? get branchRef => value('branch_ref') as String?;
  String? get requestedByStaffRef => value('requested_by_staff_ref') as String?;
  JsonObject? get filter =>
      value('filter') == null ? null : jsonObject(value('filter'));
  int? get fieldSetVersion => value('field_set_version') as int?;
  List<dynamic>? get fields => value('fields') as List<dynamic>?;
  int? get rowLimit => value('row_limit') as int?;
  int? get rowCount => value('row_count') as int?;
  String? get checksumSha256 => value('checksum_sha256') as String?;
  int? get sizeBytes => value('size_bytes') as int?;
  String? get requestedAt => value('requested_at') as String?;
  String? get startedAt => value('started_at') as String?;
  String? get completedAt => value('completed_at') as String?;
  String? get expiresAt => value('expires_at') as String?;
  String? get cancelledAt => value('cancelled_at') as String?;
  String? get failedAt => value('failed_at') as String?;
  String? get errorCode => value('error_code') as String?;
  int? get version => value('version') as int?;
}

class CrmSection8hCrmIntelligenceExportsExportPage extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsExportPage.fromJson(super.json);
  List<dynamic>? get exports => value('exports') as List<dynamic>?;
  int? get limit => value('limit') as int?;
  int? get offset => value('offset') as int?;
  bool? get hasMore => value('has_more') as bool?;
}

class CrmSection8hCrmIntelligenceExportsCreateExportRequest
    extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsCreateExportRequest.fromJson(super.json);
  String? get exportType => value('export_type') as String?;
  String? get format => value('format') as String?;
  String? get scopeType => value('scope_type') as String?;
  String? get branchRef => value('branch_ref') as String?;
  int? get rowLimit => value('row_limit') as int?;
  JsonObject? get filter =>
      value('filter') == null ? null : jsonObject(value('filter'));
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
}

class CrmSection8hCrmIntelligenceExportsCancelExportRequest
    extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsCancelExportRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class CrmSection8hCrmIntelligenceExportsDownloadExportRequest
    extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsDownloadExportRequest.fromJson(super.json);
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
}

class CrmSection8hCrmIntelligenceExportsExportAccess extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsExportAccess.fromJson(super.json);
  JsonObject? get export =>
      value('export') == null ? null : jsonObject(value('export'));
  JsonObject? get access =>
      value('access') == null ? null : jsonObject(value('access'));
}

class CrmSection8hCrmIntelligenceExportsError extends VendorPayload {
  CrmSection8hCrmIntelligenceExportsError.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}
