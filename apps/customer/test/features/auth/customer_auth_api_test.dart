import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:customer/features/auth/data/customer_auth_api.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

void main() {
  late List<http.Request> requests;

  CustomerAuthApi buildApi(
    http.Response Function(http.Request request) handler,
  ) {
    requests = [];
    return CustomerAuthApi(
      config: const ApiCoreConfig(
        baseUrl: 'http://localhost:8101/api/v1/customer',
      ),
      client: MockClient((request) async {
        requests.add(request);
        return handler(request);
      }),
    );
  }

  group('CustomerAuthApi', () {
    test('starts phone auth and parses the challenge', () async {
      final api = buildApi((request) {
        expect(request.method, 'POST');
        expect(request.url.path, '/api/v1/customer/auth/start');
        expect(jsonDecode(request.body), {'phone': '+2348012345678'});
        return http.Response(
          jsonEncode({
            'success': true,
            'data': {
              'challenge_id': 'challenge-123',
              'expires_in': 300,
              'debug_otp': '123456',
            },
          }),
          200,
        );
      });

      final result = await api.startAuth(phone: '+2348012345678');

      expect(result.challengeId, 'challenge-123');
      expect(result.debugOtp, '123456');
      expect(requests, hasLength(1));
    });

    test('maps validation errors to ApiException with field details', () async {
      final api = buildApi((request) {
        return http.Response(
          jsonEncode({
            'success': false,
            'error': {
              'code': 'validation_failed',
              'message': 'Check your details.',
              'request_id': 'req-id',
              'fields': [
                {'field': 'phone', 'message': 'Phone number is required.'},
              ],
            },
          }),
          400,
        );
      });

      await expectLater(
        api.startAuth(phone: ''),
        throwsA(
          isA<ApiException>()
              .having(
                (error) => error.code,
                'code',
                ApiErrorCode.validationFailed,
              )
              .having((error) => error.statusCode, 'statusCode', 400)
              .having((error) => error.requestId, 'requestId', 'req-id')
              .having((error) => error.fields.single.field, 'field', 'phone'),
        ),
      );
    });

    test(
      'verifies OTP and parses authenticated customer session data',
      () async {
        final api = buildApi((request) {
          expect(request.method, 'POST');
          expect(request.url.path, '/api/v1/customer/auth/verify');
          expect(jsonDecode(request.body), {
            'phone': '+2348012345678',
            'otp': '123456',
            'challenge_id': 'challenge-123',
            'device_id': 'device-123',
          });
          return http.Response(jsonEncode(_tokenEnvelope()), 200);
        });

        final result = await api.verifyAuth(
          phone: '+2348012345678',
          otp: '123456',
          challengeId: 'challenge-123',
          deviceId: 'device-123',
        );

        expect(result.accessToken, 'access-token');
        expect(result.refreshToken, 'refresh-token');
        expect(result.customer.phone, '+2348012345678');
      },
    );

    test('maps invalid OTP to an unauthorized ApiException', () async {
      final api = buildApi((request) {
        return http.Response(
          jsonEncode({
            'success': false,
            'error': {
              'code': 'unauthorized',
              'message': 'Invalid or expired verification code.',
            },
          }),
          401,
        );
      });

      await expectLater(
        api.verifyAuth(
          phone: '+2348012345678',
          otp: '000000',
          challengeId: 'challenge-123',
          deviceId: 'device-123',
        ),
        throwsA(
          isA<ApiException>().having(
            (error) => error.isAuthFailure,
            'isAuthFailure',
            isTrue,
          ),
        ),
      );
    });

    test(
      'sends bearer token when loading the current customer profile',
      () async {
        final api = buildApi((request) {
          expect(request.method, 'GET');
          expect(request.url.path, '/api/v1/customer/me');
          expect(request.headers['Authorization'], 'Bearer access-token');
          return http.Response(
            jsonEncode({
              'success': true,
              'data': _customerJson(onboardingStatus: 'complete'),
            }),
            200,
          );
        });

        final customer = await api.me(accessToken: 'access-token');

        expect(customer.phone, '+2348012345678');
        expect(customer.requiresProfile, isFalse);
      },
    );

    test('refreshes tokens with refresh token and device id', () async {
      final api = buildApi((request) {
        expect(request.method, 'POST');
        expect(request.url.path, '/api/v1/customer/auth/refresh');
        expect(jsonDecode(request.body), {
          'refresh_token': 'refresh-token',
          'device_id': 'device-123',
        });
        return http.Response(jsonEncode(_tokenEnvelope()), 200);
      });

      final result = await api.refresh(
        refreshToken: 'refresh-token',
        deviceId: 'device-123',
      );

      expect(result.expiresIn, 900);
    });

    test('maps refresh failure to unauthorized ApiException', () async {
      final api = buildApi((request) {
        return http.Response(
          jsonEncode({
            'success': false,
            'error': {'code': 'unauthorized', 'message': 'Session expired.'},
          }),
          401,
        );
      });

      await expectLater(
        api.refresh(refreshToken: 'expired-token', deviceId: 'device-123'),
        throwsA(
          isA<ApiException>().having(
            (error) => error.code,
            'code',
            'unauthorized',
          ),
        ),
      );
    });
  });
}

Map<String, Object?> _tokenEnvelope() {
  return {
    'success': true,
    'data': {
      'access_token': 'access-token',
      'refresh_token': 'refresh-token',
      'expires_in': 900,
      'customer': _customerJson(),
    },
  };
}

Map<String, Object?> _customerJson({
  String onboardingStatus = 'profile_required',
}) {
  return {
    'id': 'customer-1',
    'phone': '+2348012345678',
    'first_name': null,
    'last_name': null,
    'onboarding_status': onboardingStatus,
    'status': 'active',
  };
}
