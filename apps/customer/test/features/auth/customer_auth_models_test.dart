import 'package:customer/features/auth/models/customer_auth_models.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  group('StartAuthResult', () {
    test('parses OTP challenge payload', () {
      final result = StartAuthResult.fromJson(const {
        'challenge_id': 'challenge-123',
        'expires_in': 300,
        'debug_otp': '123456',
      });

      expect(result.challengeId, 'challenge-123');
      expect(result.expiresIn, 300);
      expect(result.debugOtp, '123456');
    });
  });

  group('AuthTokenResult', () {
    test('parses token payload and creates a stored session', () {
      final result = AuthTokenResult.fromJson(const {
        'access_token': 'access-token',
        'refresh_token': 'refresh-token',
        'expires_in': 900,
        'customer': {
          'id': 'customer-1',
          'phone': '+2348012345678',
          'first_name': null,
          'last_name': null,
          'onboarding_status': 'profile_required',
          'status': 'active',
        },
      });

      final now = DateTime.utc(2026, 1, 1, 12);
      final session = result.toSession(now);

      expect(result.customer.requiresProfile, isTrue);
      expect(session.accessToken, 'access-token');
      expect(session.refreshToken, 'refresh-token');
      expect(session.expiresAt, now.add(const Duration(seconds: 900)));
      expect(session.customer.phone, '+2348012345678');
    });
  });

  group('CustomerSession', () {
    test('round trips through JSON string storage', () {
      final session = CustomerSession(
        accessToken: 'access-token',
        refreshToken: 'refresh-token',
        expiresAt: DateTime.utc(2026, 1, 1, 12, 15),
        customer: const CustomerProfile(
          id: 'customer-1',
          phone: '+2348012345678',
          onboardingStatus: 'complete',
          firstName: 'Ada',
          lastName: 'Okafor',
          status: 'active',
        ),
      );

      final restored = CustomerSession.fromJsonString(session.toJsonString());

      expect(restored.accessToken, session.accessToken);
      expect(restored.refreshToken, session.refreshToken);
      expect(restored.expiresAt, session.expiresAt);
      expect(restored.customer.displayName, 'Ada Okafor');
      expect(restored.customer.requiresProfile, isFalse);
    });
  });
}
