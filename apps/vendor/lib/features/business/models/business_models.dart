// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostBusinessesRequest extends VendorPayload {
  PostBusinessesRequest({
    String? legalName,
    String? tradingName,
    String? supportEmail,
    String? supportPhone,
    String? currency,
    String? timezone,
  }) : super({
         'legal_name': ?legalName,
         'trading_name': ?tradingName,
         'support_email': ?supportEmail,
         'support_phone': ?supportPhone,
         'currency': ?currency,
         'timezone': ?timezone,
       });
  PostBusinessesRequest.fromJson(super.json);
  String? get legalName => value('legal_name') as String?;
  String? get tradingName => value('trading_name') as String?;
  String? get supportEmail => value('support_email') as String?;
  String? get supportPhone => value('support_phone') as String?;
  String? get currency => value('currency') as String?;
  String? get timezone => value('timezone') as String?;
}

class PatchBusinessesBusinessIdRequest extends VendorPayload {
  PatchBusinessesBusinessIdRequest({
    String? legalName,
    String? tradingName,
    String? supportEmail,
    String? supportPhone,
    String? currency,
    String? timezone,
    int? expectedVersion,
  }) : super({
         'legal_name': ?legalName,
         'trading_name': ?tradingName,
         'support_email': ?supportEmail,
         'support_phone': ?supportPhone,
         'currency': ?currency,
         'timezone': ?timezone,
         'expected_version': ?expectedVersion,
       });
  PatchBusinessesBusinessIdRequest.fromJson(super.json);
  String? get legalName => value('legal_name') as String?;
  String? get tradingName => value('trading_name') as String?;
  String? get supportEmail => value('support_email') as String?;
  String? get supportPhone => value('support_phone') as String?;
  String? get currency => value('currency') as String?;
  String? get timezone => value('timezone') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostBusinessesBusinessIdBranchesRequest extends VendorPayload {
  PostBusinessesBusinessIdBranchesRequest({
    String? brandId,
    String? displayName,
    JsonObject? address,
    num? latitude,
    num? longitude,
    String? timezone,
  }) : super({
         'brand_id': ?brandId,
         'display_name': ?displayName,
         'address': ?address,
         'latitude': ?latitude,
         'longitude': ?longitude,
         'timezone': ?timezone,
       });
  PostBusinessesBusinessIdBranchesRequest.fromJson(super.json);
  String? get brandId => value('brand_id') as String?;
  String? get displayName => value('display_name') as String?;
  JsonObject? get address =>
      value('address') == null ? null : jsonObject(value('address'));
  num? get latitude => value('latitude') as num?;
  num? get longitude => value('longitude') as num?;
  String? get timezone => value('timezone') as String?;
}

class PatchBusinessesBusinessIdBranchesBranchIdRequest extends VendorPayload {
  PatchBusinessesBusinessIdBranchesBranchIdRequest({
    String? brandId,
    String? displayName,
    JsonObject? address,
    num? latitude,
    num? longitude,
    String? timezone,
    int? expectedVersion,
  }) : super({
         'brand_id': ?brandId,
         'display_name': ?displayName,
         'address': ?address,
         'latitude': ?latitude,
         'longitude': ?longitude,
         'timezone': ?timezone,
         'expected_version': ?expectedVersion,
       });
  PatchBusinessesBusinessIdBranchesBranchIdRequest.fromJson(super.json);
  String? get brandId => value('brand_id') as String?;
  String? get displayName => value('display_name') as String?;
  JsonObject? get address =>
      value('address') == null ? null : jsonObject(value('address'));
  num? get latitude => value('latitude') as num?;
  num? get longitude => value('longitude') as num?;
  String? get timezone => value('timezone') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PutBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdRequest
    extends VendorPayload {
  PutBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdRequest({
    int? expectedVersion,
  }) : super({'expected_version': ?expectedVersion});
  PutBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdPauseRequest
    extends VendorPayload {
  PostBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdPauseRequest({
    required int expectedVersion,
  }) : super({'expected_version': expectedVersion});
  PostBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdPauseRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdResumeRequest
    extends VendorPayload {
  PostBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdResumeRequest({
    required int expectedVersion,
  }) : super({'expected_version': expectedVersion});
  PostBusinessesBusinessIdBranchesBranchIdCapabilitiesCapabilityIdResumeRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PutBusinessesBusinessIdBranchesBranchIdHoursRequest
    extends VendorPayload {
  PutBusinessesBusinessIdBranchesBranchIdHoursRequest({
    List<dynamic>? hours,
    int? expectedVersion,
  }) : super({'hours': ?hours, 'expected_version': ?expectedVersion});
  PutBusinessesBusinessIdBranchesBranchIdHoursRequest.fromJson(super.json);
  List<dynamic>? get hours => value('hours') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PutBusinessesBusinessIdBranchesBranchIdOperationsSettingsRequest
    extends VendorPayload {
  PutBusinessesBusinessIdBranchesBranchIdOperationsSettingsRequest({
    int? schemaVersion,
    bool? acceptingNewOrders,
    bool? pickupEnabled,
    int? expectedVersion,
  }) : super({
         'schema_version': ?schemaVersion,
         'accepting_new_orders': ?acceptingNewOrders,
         'pickup_enabled': ?pickupEnabled,
         'expected_version': ?expectedVersion,
       });
  PutBusinessesBusinessIdBranchesBranchIdOperationsSettingsRequest.fromJson(
    super.json,
  );
  int? get schemaVersion => value('schema_version') as int?;
  bool? get acceptingNewOrders => value('accepting_new_orders') as bool?;
  bool? get pickupEnabled => value('pickup_enabled') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostBusinessesBusinessIdBranchesBranchIdPauseRequest
    extends VendorPayload {
  PostBusinessesBusinessIdBranchesBranchIdPauseRequest({
    required int expectedVersion,
  }) : super({'expected_version': expectedVersion});
  PostBusinessesBusinessIdBranchesBranchIdPauseRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostBusinessesBusinessIdBranchesBranchIdResumeRequest
    extends VendorPayload {
  PostBusinessesBusinessIdBranchesBranchIdResumeRequest({
    required int expectedVersion,
  }) : super({'expected_version': expectedVersion});
  PostBusinessesBusinessIdBranchesBranchIdResumeRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PutBusinessesBusinessIdBranchesBranchIdServiceAreaRequest
    extends VendorPayload {
  PutBusinessesBusinessIdBranchesBranchIdServiceAreaRequest({
    String? mode,
    num? centerLatitude,
    num? centerLongitude,
    int? radiusMeters,
    int? expectedVersion,
  }) : super({
         'mode': ?mode,
         'center_latitude': ?centerLatitude,
         'center_longitude': ?centerLongitude,
         'radius_meters': ?radiusMeters,
         'expected_version': ?expectedVersion,
       });
  PutBusinessesBusinessIdBranchesBranchIdServiceAreaRequest.fromJson(
    super.json,
  );
  String? get mode => value('mode') as String?;
  num? get centerLatitude => value('center_latitude') as num?;
  num? get centerLongitude => value('center_longitude') as num?;
  int? get radiusMeters => value('radius_meters') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostBusinessesBusinessIdBrandsRequest extends VendorPayload {
  PostBusinessesBusinessIdBrandsRequest({
    String? displayName,
    String? description,
    List<dynamic>? mediaRefs,
  }) : super({
         'display_name': ?displayName,
         'description': ?description,
         'media_refs': ?mediaRefs,
       });
  PostBusinessesBusinessIdBrandsRequest.fromJson(super.json);
  String? get displayName => value('display_name') as String?;
  String? get description => value('description') as String?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
}

class PatchBusinessesBusinessIdBrandsBrandIdRequest extends VendorPayload {
  PatchBusinessesBusinessIdBrandsBrandIdRequest({
    String? displayName,
    String? description,
    List<dynamic>? mediaRefs,
    int? expectedVersion,
  }) : super({
         'display_name': ?displayName,
         'description': ?description,
         'media_refs': ?mediaRefs,
         'expected_version': ?expectedVersion,
       });
  PatchBusinessesBusinessIdBrandsBrandIdRequest.fromJson(super.json);
  String? get displayName => value('display_name') as String?;
  String? get description => value('description') as String?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class PostBusinessesBusinessIdCapabilitiesRequest extends VendorPayload {
  PostBusinessesBusinessIdCapabilitiesRequest({String? capabilityType})
    : super({'capability_type': ?capabilityType});
  PostBusinessesBusinessIdCapabilitiesRequest.fromJson(super.json);
  String? get capabilityType => value('capability_type') as String?;
}

class PostBusinessesBusinessIdCapabilitiesCapabilityIdPauseRequest
    extends VendorPayload {
  PostBusinessesBusinessIdCapabilitiesCapabilityIdPauseRequest({
    required int expectedVersion,
  }) : super({'expected_version': expectedVersion});
  PostBusinessesBusinessIdCapabilitiesCapabilityIdPauseRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostBusinessesBusinessIdCapabilitiesCapabilityIdResumeRequest
    extends VendorPayload {
  PostBusinessesBusinessIdCapabilitiesCapabilityIdResumeRequest({
    required int expectedVersion,
  }) : super({'expected_version': expectedVersion});
  PostBusinessesBusinessIdCapabilitiesCapabilityIdResumeRequest.fromJson(
    super.json,
  );
  int? get expectedVersion => value('expected_version') as int?;
}

class PostBusinessesBusinessIdPauseRequest extends VendorPayload {
  PostBusinessesBusinessIdPauseRequest({required int expectedVersion})
    : super({'expected_version': expectedVersion});
  PostBusinessesBusinessIdPauseRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class PostBusinessesBusinessIdResumeRequest extends VendorPayload {
  PostBusinessesBusinessIdResumeRequest({required int expectedVersion})
    : super({'expected_version': expectedVersion});
  PostBusinessesBusinessIdResumeRequest.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class BusinessSection6VendorBusinessVersionCommand extends VendorPayload {
  BusinessSection6VendorBusinessVersionCommand.fromJson(super.json);
  int? get expectedVersion => value('expected_version') as int?;
}

class BusinessSection6VendorBusinessBusinessCreate extends VendorPayload {
  BusinessSection6VendorBusinessBusinessCreate.fromJson(super.json);
  String? get legalName => value('legal_name') as String?;
  String? get tradingName => value('trading_name') as String?;
  String? get supportEmail => value('support_email') as String?;
  String? get supportPhone => value('support_phone') as String?;
  String? get currency => value('currency') as String?;
  String? get timezone => value('timezone') as String?;
}

class BusinessSection6VendorBusinessBusinessUpdate extends VendorPayload {
  BusinessSection6VendorBusinessBusinessUpdate.fromJson(super.json);
  String? get legalName => value('legal_name') as String?;
  String? get tradingName => value('trading_name') as String?;
  String? get supportEmail => value('support_email') as String?;
  String? get supportPhone => value('support_phone') as String?;
  String? get currency => value('currency') as String?;
  String? get timezone => value('timezone') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class BusinessSection6VendorBusinessCapabilityCreate extends VendorPayload {
  BusinessSection6VendorBusinessCapabilityCreate.fromJson(super.json);
  String? get capabilityType => value('capability_type') as String?;
}

class BusinessSection6VendorBusinessMediaReference extends VendorPayload {
  BusinessSection6VendorBusinessMediaReference.fromJson(super.json);
  String? get kind => value('kind') as String?;
  String? get mediaRef => value('media_ref') as String?;
}

class BusinessSection6VendorBusinessBrandCreate extends VendorPayload {
  BusinessSection6VendorBusinessBrandCreate.fromJson(super.json);
  String? get displayName => value('display_name') as String?;
  String? get description => value('description') as String?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
}

class BusinessSection6VendorBusinessBrandUpdate extends VendorPayload {
  BusinessSection6VendorBusinessBrandUpdate.fromJson(super.json);
  String? get displayName => value('display_name') as String?;
  String? get description => value('description') as String?;
  List<dynamic>? get mediaRefs => value('media_refs') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class BusinessSection6VendorBusinessAddress extends VendorPayload {
  BusinessSection6VendorBusinessAddress.fromJson(super.json);
  String? get line1 => value('line1') as String?;
  String? get line2 => value('line2') as String?;
  String? get city => value('city') as String?;
  String? get region => value('region') as String?;
  String? get postalCode => value('postal_code') as String?;
  String? get countryCode => value('country_code') as String?;
}

class BusinessSection6VendorBusinessBranchCreate extends VendorPayload {
  BusinessSection6VendorBusinessBranchCreate.fromJson(super.json);
  String? get brandId => value('brand_id') as String?;
  String? get displayName => value('display_name') as String?;
  JsonObject? get address =>
      value('address') == null ? null : jsonObject(value('address'));
  num? get latitude => value('latitude') as num?;
  num? get longitude => value('longitude') as num?;
  String? get timezone => value('timezone') as String?;
}

class BusinessSection6VendorBusinessBranchUpdate extends VendorPayload {
  BusinessSection6VendorBusinessBranchUpdate.fromJson(super.json);
  String? get brandId => value('brand_id') as String?;
  String? get displayName => value('display_name') as String?;
  JsonObject? get address =>
      value('address') == null ? null : jsonObject(value('address'));
  num? get latitude => value('latitude') as num?;
  num? get longitude => value('longitude') as num?;
  String? get timezone => value('timezone') as String?;
  int? get expectedVersion => value('expected_version') as int?;
}

class BusinessSection6VendorBusinessOperatingWindow extends VendorPayload {
  BusinessSection6VendorBusinessOperatingWindow.fromJson(super.json);
  int? get weekday => value('weekday') as int?;
  bool? get isClosed => value('is_closed') as bool?;
  String? get opensAt => value('opens_at') as String?;
  String? get closesAt => value('closes_at') as String?;
}

class BusinessSection6VendorBusinessHoursUpdate extends VendorPayload {
  BusinessSection6VendorBusinessHoursUpdate.fromJson(super.json);
  List<dynamic>? get hours => value('hours') as List<dynamic>?;
  int? get expectedVersion => value('expected_version') as int?;
}

class BusinessSection6VendorBusinessServiceAreaUpdate extends VendorPayload {
  BusinessSection6VendorBusinessServiceAreaUpdate.fromJson(super.json);
  String? get mode => value('mode') as String?;
  num? get centerLatitude => value('center_latitude') as num?;
  num? get centerLongitude => value('center_longitude') as num?;
  int? get radiusMeters => value('radius_meters') as int?;
  int? get expectedVersion => value('expected_version') as int?;
}

class BusinessSection6VendorBusinessSettingsUpdate extends VendorPayload {
  BusinessSection6VendorBusinessSettingsUpdate.fromJson(super.json);
  int? get schemaVersion => value('schema_version') as int?;
  bool? get acceptingNewOrders => value('accepting_new_orders') as bool?;
  bool? get pickupEnabled => value('pickup_enabled') as bool?;
  int? get expectedVersion => value('expected_version') as int?;
}
