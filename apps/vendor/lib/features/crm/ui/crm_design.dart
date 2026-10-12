import 'package:flutter/material.dart';
import '../../../core/contracts/vendor_contract.dart';

const crmBase = '/api/v1/vendor/crm';

enum CrmArea {
  customers,
  tags,
  segments,
  interactions,
  tasks,
  corporates,
  contacts,
  locations,
  costCenters,
  memberships,
  corporateInteractions,
  corporateTasks,
  program,
  tiers,
  loyalty,
  entries,
  consent,
  privacy,
  intelligence,
  members,
  revisions,
  history,
}

extension CrmAreaInfo on CrmArea {
  String get title => switch (this) {
    CrmArea.customers => 'Customers',
    CrmArea.tags => 'Customer tags',
    CrmArea.segments => 'Customer segments',
    CrmArea.interactions || CrmArea.corporateInteractions => 'Interactions',
    CrmArea.tasks || CrmArea.corporateTasks => 'Follow-ups',
    CrmArea.corporates => 'Corporate accounts',
    CrmArea.contacts => 'Contacts',
    CrmArea.locations => 'Locations',
    CrmArea.costCenters => 'Cost centres',
    CrmArea.memberships => 'Memberships',
    CrmArea.program => 'Loyalty programme',
    CrmArea.tiers => 'Loyalty tiers',
    CrmArea.loyalty => 'Customer loyalty',
    CrmArea.entries => 'Points history',
    CrmArea.consent => 'Consent & eligibility',
    CrmArea.privacy => 'Privacy status',
    CrmArea.intelligence => 'Customer insights',
    CrmArea.members => 'Segment members',
    CrmArea.revisions => 'Interaction revisions',
    CrmArea.history => 'Task history',
  };
  IconData get icon => switch (this) {
    CrmArea.customers || CrmArea.contacts => Icons.people_outline_rounded,
    CrmArea.tags => Icons.sell_outlined,
    CrmArea.segments || CrmArea.members => Icons.hub_outlined,
    CrmArea.interactions ||
    CrmArea.corporateInteractions ||
    CrmArea.revisions => Icons.forum_outlined,
    CrmArea.tasks ||
    CrmArea.corporateTasks ||
    CrmArea.history => Icons.task_alt_rounded,
    CrmArea.corporates => Icons.apartment_rounded,
    CrmArea.locations => Icons.location_on_outlined,
    CrmArea.costCenters => Icons.account_tree_outlined,
    CrmArea.memberships => Icons.badge_outlined,
    CrmArea.program ||
    CrmArea.tiers ||
    CrmArea.loyalty ||
    CrmArea.entries => Icons.workspace_premium_outlined,
    CrmArea.consent || CrmArea.privacy => Icons.shield_outlined,
    CrmArea.intelligence => Icons.insights_rounded,
  };
  String get collection => switch (this) {
    CrmArea.corporates => 'corporate-customers',
    CrmArea.corporateTasks => 'tasks',
    CrmArea.corporateInteractions => 'interactions',
    CrmArea.costCenters => 'cost-centers',
    CrmArea.program => 'loyalty/program',
    CrmArea.tiers => 'loyalty/tiers',
    CrmArea.entries => 'loyalty/entries',
    _ => name,
  };
  String get idKey => switch (this) {
    CrmArea.customers => 'profileId',
    CrmArea.tags => 'tagId',
    CrmArea.segments => 'segmentId',
    CrmArea.interactions || CrmArea.corporateInteractions => 'interactionId',
    CrmArea.tasks || CrmArea.corporateTasks => 'taskId',
    CrmArea.corporates => 'corporateId',
    CrmArea.contacts => 'contactId',
    CrmArea.locations => 'locationId',
    CrmArea.costCenters => 'costCenterId',
    CrmArea.memberships => 'membershipId',
    _ => 'id',
  };
  String get refKey => switch (this) {
    CrmArea.customers => 'profile_ref',
    CrmArea.tags => 'tag_ref',
    CrmArea.segments => 'segment_ref',
    CrmArea.interactions || CrmArea.corporateInteractions => 'interaction_ref',
    CrmArea.tasks || CrmArea.corporateTasks => 'task_ref',
    CrmArea.corporates => 'corporate_ref',
    CrmArea.contacts => 'contact_ref',
    CrmArea.locations => 'location_ref',
    CrmArea.costCenters => 'cost_center_ref',
    CrmArea.memberships => 'membership_ref',
    CrmArea.entries => 'entry_ref',
    _ => 'id',
  };
  bool get corporateChild => [
    CrmArea.contacts,
    CrmArea.locations,
    CrmArea.costCenters,
    CrmArea.memberships,
    CrmArea.corporateInteractions,
    CrmArea.corporateTasks,
  ].contains(this);
  bool get profileChild => [
    CrmArea.loyalty,
    CrmArea.entries,
    CrmArea.consent,
    CrmArea.privacy,
    CrmArea.intelligence,
  ].contains(this);
  bool get singleton => [
    CrmArea.program,
    CrmArea.tiers,
    ...CrmArea.values.where((a) => a.profileChild && a != CrmArea.entries),
  ].contains(this);
  bool get creatable => [
    CrmArea.tags,
    CrmArea.segments,
    CrmArea.interactions,
    CrmArea.tasks,
    CrmArea.corporates,
    ...CrmArea.values.where((a) => a.corporateChild),
  ].contains(this);
}

String crmLabel(Object? value) => '${value ?? ''}'
    .replaceAll('_', ' ')
    .split(' ')
    .map(
      (w) => w.isEmpty
          ? w
          : '${w[0].toUpperCase()}${w.substring(1).toLowerCase()}',
    )
    .join(' ');
Map<String, String> crmOptions(List<String> values) => {
  for (final v in values) v: crmLabel(v),
};
const crmTypes = [
  'NOTE',
  'PHONE_CALL',
  'IN_PERSON',
  'EMAIL',
  'SMS',
  'IN_APP',
  'SOCIAL',
  'SALES_FOLLOW_UP',
  'SERVICE_FOLLOW_UP',
  'FEEDBACK',
  'ACCOUNT_REVIEW',
  'OTHER',
];
const crmOutcomes = [
  'NONE',
  'SUCCESSFUL',
  'NO_RESPONSE',
  'FOLLOW_UP_REQUIRED',
  'RESOLVED',
  'UNRESOLVED',
  'CUSTOMER_DECLINED',
  'VENDOR_DECLINED',
  'ESCALATED',
  'CANCELLED',
];
const crmPriorities = ['LOW', 'NORMAL', 'HIGH', 'URGENT'];
const crmContactRoles = [
  'PRIMARY_CONTACT',
  'PROCUREMENT',
  'OPERATIONS',
  'FINANCE',
  'MANAGEMENT',
  'ADMINISTRATION',
  'TECHNICAL',
  'OTHER',
];
const crmLocationTypes = [
  'HEADQUARTERS',
  'OFFICE',
  'BRANCH',
  'STORE',
  'WAREHOUSE',
  'SITE',
  'CAMPUS',
  'OTHER',
];
const crmMembershipRoles = [
  'MEMBER',
  'MANAGER',
  'APPROVER',
  'REQUESTER',
  'PROCUREMENT',
  'FINANCE',
  'OPERATIONS',
  'OTHER',
];
const crmOrganizationTypes = [
  'COMPANY',
  'INSTITUTION',
  'SCHOOL',
  'HOSPITAL',
  'HOTEL',
  'RESTAURANT',
  'RETAILER',
  'NGO',
  'ASSOCIATION',
  'GOVERNMENT',
  'OTHER',
];
Map<String, String> crmEnums(String key, CrmArea area) => switch (key) {
  'type' => crmOptions(crmTypes),
  'direction' => crmOptions([
    'INBOUND',
    'OUTBOUND',
    'INTERNAL',
    'NOT_APPLICABLE',
  ]),
  'outcome' => crmOptions(crmOutcomes),
  'priority' => crmOptions(crmPriorities),
  'visibility' => crmOptions(['BUSINESS', 'BRANCH']),
  'scope_mode' => crmOptions(['BUSINESS', 'BRANCH_SET']),
  'organization_type' => crmOptions(crmOrganizationTypes),
  'relationship_role' => crmOptions(crmContactRoles),
  'location_type' => crmOptions(crmLocationTypes),
  'membership_role' => crmOptions(crmMembershipRoles),
  'segment_type' => crmOptions(['STATIC', 'DYNAMIC']),
  'expiry_policy' => crmOptions(['NEVER', 'FIXED_DAYS']),
  'status' =>
    area == CrmArea.program
        ? crmOptions(['DISABLED', 'ACTIVE', 'PAUSED'])
        : crmOptions(['ACTIVE', 'INACTIVE']),
  _ => {},
};
Map<String, Object?> crmDefaults(CrmArea area) => switch (area) {
  CrmArea.tags => {
    'name': 'Frequent visitors',
    'description': 'Customers we know well',
  },
  CrmArea.segments => {
    'name': 'Returning customers',
    'description': 'Our regular customer group',
    'segment_type': 'STATIC',
  },
  CrmArea.interactions || CrmArea.corporateInteractions => {
    'type': 'NOTE',
    'direction': 'INTERNAL',
    'outcome': 'FOLLOW_UP_REQUIRED',
    'subject': 'Customer follow-up',
    'note': 'Check in after the recent order.',
    'visibility': 'BUSINESS',
    'occurred_at': DateTime.now().toUtc().toIso8601String(),
  },
  CrmArea.tasks || CrmArea.corporateTasks => {
    'title': 'Follow up on the recent order',
    'description': 'Check that everything arrived as expected.',
    'priority': 'NORMAL',
    'visibility': 'BUSINESS',
    'due_at': DateTime.now()
        .add(const Duration(days: 1))
        .toUtc()
        .toIso8601String(),
  },
  CrmArea.corporates => {
    'organization_name': 'Lagos Office Collective',
    'display_name': 'Lagos Office',
    'organization_type': 'COMPANY',
    'scope_mode': 'BUSINESS',
  },
  CrmArea.contacts => {
    'relationship_role': 'PRIMARY_CONTACT',
    'is_primary_contact': true,
    'status': 'ACTIVE',
  },
  CrmArea.locations => {
    'location_code': 'HQ',
    'name': 'Head office',
    'location_type': 'HEADQUARTERS',
    'address_line1': '12 Marina Road',
    'city': 'Lagos',
    'state_region': 'Lagos',
    'country_code': 'NG',
    'timezone': 'Africa/Lagos',
    'status': 'ACTIVE',
  },
  CrmArea.costCenters => {
    'code': 'OPERATIONS',
    'name': 'Operations',
    'status': 'ACTIVE',
  },
  CrmArea.memberships => {'membership_role': 'MEMBER', 'status': 'ACTIVE'},
  _ => {},
};
String crmRecordTitle(CrmArea area, JsonObject row) =>
    '${row['display_label'] ?? row['display_name'] ?? row['organization_name'] ?? row['name'] ?? row['title'] ?? row['subject'] ?? row[area.refKey] ?? area.title}';
