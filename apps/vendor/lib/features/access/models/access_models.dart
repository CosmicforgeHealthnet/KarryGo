// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostRolesRequest extends VendorPayload {
  PostRolesRequest({String? code, String? name, String? description, List<dynamic>? permissionCodes, String? stepUpGrantId}) : super({'code': ?code, 'name': ?name, 'description': ?description, 'permission_codes': ?permissionCodes, 'step_up_grant_id': ?stepUpGrantId});
  PostRolesRequest.fromJson(super.json);
  String? get code => value('code') as String?;
  String? get name => value('name') as String?;
  String? get description => value('description') as String?;
  List<dynamic>? get permissionCodes => value('permission_codes') as List<dynamic>?;
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
}

class PostRolesIdAssignRequest extends VendorPayload {
  PostRolesIdAssignRequest({String? staffId, String? roleId, String? branchId, String? stepUpGrantId}) : super({'staff_id': ?staffId, 'role_id': ?roleId, 'branch_id': ?branchId, 'step_up_grant_id': ?stepUpGrantId});
  PostRolesIdAssignRequest.fromJson(super.json);
  String? get staffId => value('staff_id') as String?;
  String? get roleId => value('role_id') as String?;
  String? get branchId => value('branch_id') as String?;
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
}

class PutRolesIdPermissionsRequest extends VendorPayload {
  PutRolesIdPermissionsRequest({List<dynamic>? permissionCodes, int? expectedVersion, String? stepUpGrantId}) : super({'permission_codes': ?permissionCodes, 'expected_version': ?expectedVersion, 'step_up_grant_id': ?stepUpGrantId});
  PutRolesIdPermissionsRequest.fromJson(super.json);
  List<dynamic>? get permissionCodes => value('permission_codes') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
  String? get stepUpGrantId => value('step_up_grant_id') as String?;
}

class PostStaffRequest extends VendorPayload {
  PostStaffRequest({String? phone, String? email, String? roleId, String? branchId}) : super({'phone': ?phone, 'email': ?email, 'role_id': ?roleId, 'branch_id': ?branchId});
  PostStaffRequest.fromJson(super.json);
  String? get phone => value('phone') as String?;
  String? get email => value('email') as String?;
  String? get roleId => value('role_id') as String?;
  String? get branchId => value('branch_id') as String?;
}

class PutStaffIdReactivateRequest extends VendorPayload {
  PutStaffIdReactivateRequest({int? expectedVersion}) : super({'expected_version': ?expectedVersion});
  PutStaffIdReactivateRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutStaffIdSuspendRequest extends VendorPayload {
  PutStaffIdSuspendRequest({String? reason, int? expectedVersion}) : super({'reason': ?reason, 'expected_version': ?expectedVersion});
  PutStaffIdSuspendRequest.fromJson(super.json);
  String? get reason => value('reason') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class AccessSection5AuthStaffIamSuccessEnvelope extends VendorPayload {
  AccessSection5AuthStaffIamSuccessEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  Object? get data => value('data');
}

class AccessSection5AuthStaffIamErrorEnvelope extends VendorPayload {
  AccessSection5AuthStaffIamErrorEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error => value('error') == null ? null : jsonObject(value('error'));
}
