import 'dart:convert';
import '../../../core/contracts/vendor_contract.dart';

/// Exact token response fields in auth/http/dto.go. No decoded-token authority.
class VendorSession {
  const VendorSession({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresIn,
    required this.staffId,
    required this.vendorId,
  });
  final String accessToken, refreshToken, staffId, vendorId;
  final int expiresIn;
  factory VendorSession.fromJson(JsonObject json) => VendorSession(
    accessToken: json['access_token'] as String,
    refreshToken: json['refresh_token'] as String,
    expiresIn: json['expires_in'] as int,
    staffId: json['staff_id'] as String,
    vendorId: json['vendor_id'] as String,
  );
  JsonObject toJson() => {
    'access_token': accessToken,
    'refresh_token': refreshToken,
    'expires_in': expiresIn,
    'staff_id': staffId,
    'vendor_id': vendorId,
  };
  String encode() => jsonEncode(toJson());
}

class VendorChallenge {
  const VendorChallenge({
    required this.challengeId,
    required this.expiresIn,
    this.debugOtp,
  });
  final String challengeId;
  final int expiresIn;
  final String? debugOtp;
  factory VendorChallenge.fromJson(JsonObject json) => VendorChallenge(
    challengeId: json['challenge_id'] as String,
    expiresIn: json['expires_in'] as int,
    debugOtp: json['debug_otp'] as String?,
  );
}

class VendorVerification {
  VendorVerification.fromJson(JsonObject json)
    : requiresVendorChoice = json['requires_vendor_choice'] == true,
      availableVendorIds = List<String>.from(
        json['available_vendor_ids'] as List? ?? const [],
      ),
      session = json['requires_vendor_choice'] == true
          ? null
          : VendorSession.fromJson(json);
  final bool requiresVendorChoice;
  final List<String> availableVendorIds;
  final VendorSession? session;
}

class VendorStepUpGrant {
  const VendorStepUpGrant({required this.grantId, required this.expiresIn});
  final String grantId;
  final int expiresIn;
  factory VendorStepUpGrant.fromJson(JsonObject json) => VendorStepUpGrant(
    grantId: json['grant_id'] as String,
    expiresIn: json['expires_in'] as int,
  );
}
