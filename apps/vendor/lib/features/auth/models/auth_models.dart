// GENERATED from actual request schemas/Go JSON fields.
import '../../../core/contracts/vendor_contract.dart';

class PostAuthLogoutRequest extends VendorPayload {
  PostAuthLogoutRequest({String? refreshToken})
    : super({'refresh_token': ?refreshToken});
  PostAuthLogoutRequest.fromJson(super.json);
  String? get refreshToken => value('refresh_token') as String?;
}

class PostAuthRefreshRequest extends VendorPayload {
  PostAuthRefreshRequest({String? refreshToken, String? deviceId})
    : super({'refresh_token': ?refreshToken, 'device_id': ?deviceId});
  PostAuthRefreshRequest.fromJson(super.json);
  String? get refreshToken => value('refresh_token') as String?;
  String? get deviceId => value('device_id') as String?;
}

class PostAuthResendRequest extends VendorPayload {
  PostAuthResendRequest({String? phone, String? email, String? challengeId})
    : super({'phone': ?phone, 'email': ?email, 'challenge_id': ?challengeId});
  PostAuthResendRequest.fromJson(super.json);
  String? get phone => value('phone') as String?;
  String? get email => value('email') as String?;
  String? get challengeId => value('challenge_id') as String?;
}

class PostAuthStartRequest extends VendorPayload {
  PostAuthStartRequest({String? phone, String? email})
    : super({'phone': ?phone, 'email': ?email});
  PostAuthStartRequest.fromJson(super.json);
  String? get phone => value('phone') as String?;
  String? get email => value('email') as String?;
}

class PostAuthStepUpStartRequest extends VendorPayload {
  PostAuthStepUpStartRequest({String? purpose}) : super({'purpose': ?purpose});
  PostAuthStepUpStartRequest.fromJson(super.json);
  String? get purpose => value('purpose') as String?;
}

class PostAuthStepUpVerifyRequest extends VendorPayload {
  PostAuthStepUpVerifyRequest({
    String? purpose,
    String? challengeId,
    String? otp,
  }) : super({'purpose': ?purpose, 'challenge_id': ?challengeId, 'otp': ?otp});
  PostAuthStepUpVerifyRequest.fromJson(super.json);
  String? get purpose => value('purpose') as String?;
  String? get challengeId => value('challenge_id') as String?;
  String? get otp => value('otp') as String?;
}

class PostAuthVerifyRequest extends VendorPayload {
  PostAuthVerifyRequest({
    String? phone,
    String? email,
    String? otp,
    String? challengeId,
    String? vendorId,
    String? deviceId,
  }) : super({
         'phone': ?phone,
         'email': ?email,
         'otp': ?otp,
         'challenge_id': ?challengeId,
         'vendor_id': ?vendorId,
         'device_id': ?deviceId,
       });
  PostAuthVerifyRequest.fromJson(super.json);
  String? get phone => value('phone') as String?;
  String? get email => value('email') as String?;
  String? get otp => value('otp') as String?;
  String? get challengeId => value('challenge_id') as String?;
  String? get vendorId => value('vendor_id') as String?;
  String? get deviceId => value('device_id') as String?;
}

class AuthSection5AuthStaffIamSuccessEnvelope extends VendorPayload {
  AuthSection5AuthStaffIamSuccessEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  Object? get data => value('data');
}

class AuthSection5AuthStaffIamErrorEnvelope extends VendorPayload {
  AuthSection5AuthStaffIamErrorEnvelope.fromJson(super.json);
  bool? get success => value('success') as bool?;
  JsonObject? get error =>
      value('error') == null ? null : jsonObject(value('error'));
}
