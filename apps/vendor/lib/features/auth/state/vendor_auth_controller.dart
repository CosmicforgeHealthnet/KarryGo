import 'package:flutter/foundation.dart';
import '../../../core/contracts/vendor_contract.dart';
import '../../../core/network/vendor_api_client.dart';
import '../../../core/network/vendor_feature_api.dart';
import '../models/vendor_session.dart';

enum VendorAuthPhase { signedOut, challenge, vendorChoice, authenticated }

class VendorAuthController extends ChangeNotifier {
  VendorAuthController({required this.api, required this.client});
  final VendorFeatureApi api;
  final VendorApiClient client;
  VendorAuthPhase phase = VendorAuthPhase.signedOut;
  VendorChallenge? challenge;
  VendorSession? session;
  List<String> availableVendorIds = const [];
  bool busy = false;
  Object? error;
  String? _phone, _email, _pendingOtp, _deviceId;
  int _generation = 0;
  bool _disposed = false;
  void _notify() {
    if (!_disposed) notifyListeners();
  }

  Future<void> restore() async {
    final value = await client.sessions.read();
    sessionChanged(value);
  }

  void sessionChanged(VendorSession? value) {
    if (_disposed) return;
    session = value;
    phase = value == null
        ? VendorAuthPhase.signedOut
        : VendorAuthPhase.authenticated;
    if (value == null) {
      _generation++;
      challenge = null;
      _pendingOtp = null;
      availableVendorIds = const [];
    }
    _notify();
  }

  JsonObject _identifier() => {
    if (_phone != null) 'phone': _phone,
    if (_email != null) 'email': _email,
  };
  Future<T> _run<T>(Future<T> Function() action) async {
    if (busy) throw StateError('Authentication operation already pending');
    busy = true;
    error = null;
    _notify();
    try {
      return await action();
    } catch (e) {
      error = e;
      rethrow;
    } finally {
      busy = false;
      _notify();
    }
  }

  Future<VendorChallenge> start({String? phone, String? email}) =>
      _run(() async {
        if ((phone == null) == (email == null)) {
          throw ArgumentError('Provide phone OR email');
        }
        _phone = phone;
        _email = email;
        _pendingOtp = null;
        availableVendorIds = const [];
        challenge = null;
        final result = await api.request(
          'POST',
          '/api/v1/vendor/auth/start',
          body: VendorPayload(_identifier()),
        );
        challenge = VendorChallenge.fromJson(result.object);
        phase = VendorAuthPhase.challenge;
        return challenge!;
      });
  Future<VendorChallenge> resend() => _run(() async {
    if (challenge == null) throw StateError('Start a challenge first');
    final result = await api.request(
      'POST',
      '/api/v1/vendor/auth/resend',
      body: VendorPayload({
        ..._identifier(),
        'challenge_id': challenge!.challengeId,
      }),
    );
    challenge = VendorChallenge.fromJson(result.object);
    _pendingOtp = null;
    availableVendorIds = const [];
    phase = VendorAuthPhase.challenge;
    return challenge!;
  });
  Future<VendorVerification> verify({required String otp, String? deviceId}) =>
      _run(() async {
        if (challenge == null) throw StateError('Start a challenge first');
        _deviceId = deviceId;
        _pendingOtp = otp;
        return _verify(otp: otp);
      });
  Future<VendorVerification> chooseVendor(String vendorId) => _run(() async {
    if (phase != VendorAuthPhase.vendorChoice ||
        _pendingOtp == null ||
        !availableVendorIds.contains(vendorId)) {
      throw StateError('No matching vendor choice');
    }
    return _verify(otp: _pendingOtp!, vendorId: vendorId);
  });
  Future<VendorVerification> _verify({
    required String otp,
    String? vendorId,
  }) async {
    final generation = _generation;
    final result = await api.request(
      'POST',
      '/api/v1/vendor/auth/verify',
      body: VendorPayload({
        ..._identifier(),
        'otp': otp,
        'challenge_id': challenge!.challengeId,
        'vendor_id': ?vendorId,
        if (_deviceId != null) 'device_id': _deviceId,
      }),
    );
    final value = VendorVerification.fromJson(result.object);
    if (generation != _generation) {
      throw StateError('Authentication attempt cancelled');
    }
    if (value.requiresVendorChoice) {
      availableVendorIds = value.availableVendorIds;
      phase = VendorAuthPhase.vendorChoice;
    } else {
      await client.acceptSession(value.session!);
      session = value.session;
      phase = VendorAuthPhase.authenticated;
      challenge = null;
      _pendingOtp = null;
      availableVendorIds = const [];
    }
    return value;
  }

  Future<VendorChallenge> startStepUp(String purpose) => _run(() async {
    await client.refreshSession();
    return VendorChallenge.fromJson(
      (await api.request(
        'POST',
        '/api/v1/vendor/auth/step-up/start',
        body: VendorPayload({'purpose': purpose}),
      )).object,
    );
  });
  Future<VendorStepUpGrant> verifyStepUp({
    required String purpose,
    required String challengeId,
    required String otp,
  }) => _run(
    () async => VendorStepUpGrant.fromJson(
      (await api.request(
        'POST',
        '/api/v1/vendor/auth/step-up/verify',
        retryAuthentication: false,
        body: VendorPayload({
          'purpose': purpose,
          'challenge_id': challengeId,
          'otp': otp,
        }),
      )).object,
    ),
  );
  Future<void> logout() => _run(() async {
    _generation++;
    final current = await client.sessions.read();
    try {
      if (current != null) {
        await api.request(
          'POST',
          '/api/v1/vendor/auth/logout',
          body: VendorPayload({'refresh_token': current.refreshToken}),
        );
      }
    } finally {
      await client.clearSession();
      sessionChanged(null);
    }
  });
  @override
  void dispose() {
    _disposed = true;
    _generation++;
    _pendingOtp = null;
    super.dispose();
  }
}
