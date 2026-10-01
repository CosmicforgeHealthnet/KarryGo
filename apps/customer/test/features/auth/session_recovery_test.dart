import 'dart:convert';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:customer/features/auth/data/customer_auth_api.dart';
import 'package:customer/features/auth/data/customer_session_store.dart';
import 'package:customer/features/auth/models/customer_auth_models.dart';
import 'package:customer/features/auth/state/customer_auth_controller.dart';
import 'package:customer/features/media/data/media_file_api.dart';
import 'package:customer/features/media/data/media_upload_service.dart';
import 'package:customer/features/media/models/media_upload_result.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

/// The controller requires a media service; these tests never upload anything.
class _StubMediaUploadService extends MediaUploadService {
  _StubMediaUploadService()
      : super(
          api: MediaFileApi(
            config: const ApiCoreConfig(
              baseUrl: 'http://localhost:8109/api/v1/media-files',
            ),
            sessionStore: MemoryCustomerSessionStore(),
            client: MockClient((_) async => http.Response('{}', 500)),
          ),
        );

  @override
  Future<MediaUploadResult?> pickAndUpload({
    required String ownerId,
    required String purpose,
    dynamic source,
  }) async => null;
}

/// Access tokens live 15 minutes while refresh tokens live 30 days, so a 401
/// during normal use almost always means "access token aged out", not "session
/// revoked". These tests pin that a mid-session 401 refreshes in place instead
/// of dumping the user to the sign-in screen — which previously happened every
/// 15 minutes and could interrupt a live booking.
void main() {
  CustomerProfile profile() => const CustomerProfile(
        id: 'c1',
        phone: '+2348011111111',
        email: 'test@example.com',
        onboardingStatus: 'complete',
      );

  CustomerSession session({String accessToken = 'old-access'}) =>
      CustomerSession(
        accessToken: accessToken,
        refreshToken: 'refresh-token',
        expiresAt: DateTime.now().add(const Duration(minutes: 15)),
        customer: profile(),
      );

  /// Builds a controller already sitting in the authenticated state, as if the
  /// user were mid-session when a bearer call came back 401.
  Future<CustomerAuthController> authenticatedController({
    required http.Client client,
    CustomerSessionStore? store,
  }) async {
    final sessionStore = store ?? MemoryCustomerSessionStore();
    await sessionStore.saveSession(session());
    final controller = CustomerAuthController(
      api: CustomerAuthApi(
        config: const ApiCoreConfig(baseUrl: 'http://localhost:8101/api/v1/customer'),
        client: client,
      ),
      sessionStore: sessionStore,
      mediaUploadService: _StubMediaUploadService(),
    );
    await controller.initialize();
    return controller;
  }

  http.Response ok(Map<String, dynamic> data) =>
      http.Response(jsonEncode({'success': true, 'data': data}), 200);

  http.Response unauthorized() => http.Response(
        jsonEncode({
          'success': false,
          'error': {
            'code': 'unauthorized',
            'message': 'Your session has expired. Please sign in again.',
          },
        }),
        401,
      );

  Map<String, dynamic> customerJson() => {
        'id': 'c1',
        'phone': '+2348011111111',
        'email': 'test@example.com',
        'onboarding_status': 'complete',
        'status': 'active',
      };

  test('a mid-session 401 refreshes the session instead of signing out',
      () async {
    var refreshCalls = 0;
    final controller = await authenticatedController(
      client: MockClient((request) async {
        if (request.url.path.endsWith('/auth/refresh')) {
          refreshCalls++;
          return ok({
            'access_token': 'new-access',
            'refresh_token': 'new-refresh',
            'expires_in': 900,
            'customer': customerJson(),
          });
        }
        if (request.url.path.endsWith('/me')) return ok(customerJson());
        return unauthorized();
      }),
    );

    expect(controller.state.status, CustomerAuthStatus.authenticated);

    await controller.handleAuthFailure();

    expect(refreshCalls, 1, reason: 'should have attempted a refresh');
    expect(
      controller.state.status,
      CustomerAuthStatus.authenticated,
      reason: 'the user should stay signed in, not be bounced to phone entry',
    );
    expect(controller.state.session?.accessToken, 'new-access');
    expect(controller.state.error, isNull);
  });

  test('signs out when the refresh token is itself rejected', () async {
    final store = MemoryCustomerSessionStore();
    final controller = await authenticatedController(
      store: store,
      client: MockClient((request) async {
        if (request.url.path.endsWith('/auth/refresh')) return unauthorized();
        if (request.url.path.endsWith('/me')) return ok(customerJson());
        return unauthorized();
      }),
    );

    await controller.handleAuthFailure();

    expect(
      controller.state.status,
      CustomerAuthStatus.phoneEntry,
      reason: 'a revoked session must still sign the user out',
    );
    expect(controller.state.error?.code, ApiErrorCode.unauthorized);
    expect(
      await store.readSession(),
      isNull,
      reason: 'the dead session must be cleared from storage',
    );
  });

  test('signs out when refresh fails at the network level', () async {
    final controller = await authenticatedController(
      client: MockClient((request) async {
        if (request.url.path.endsWith('/auth/refresh')) {
          throw const SocketExceptionStub();
        }
        if (request.url.path.endsWith('/me')) return ok(customerJson());
        return unauthorized();
      }),
    );

    await controller.handleAuthFailure();

    expect(controller.state.status, CustomerAuthStatus.phoneEntry);
  });

  test('concurrent 401s from parallel calls only refresh once', () async {
    var refreshCalls = 0;
    final controller = await authenticatedController(
      client: MockClient((request) async {
        if (request.url.path.endsWith('/auth/refresh')) {
          refreshCalls++;
          // Give the second caller a window to land while this is in flight.
          await Future<void>.delayed(const Duration(milliseconds: 20));
          return ok({
            'access_token': 'new-access',
            'refresh_token': 'new-refresh',
            'expires_in': 900,
            'customer': customerJson(),
          });
        }
        if (request.url.path.endsWith('/me')) return ok(customerJson());
        return unauthorized();
      }),
    );

    // Six API clients share this callback, so simultaneous 401s are routine.
    await Future.wait([
      controller.handleAuthFailure(),
      controller.handleAuthFailure(),
      controller.handleAuthFailure(),
    ]);

    expect(refreshCalls, 1, reason: 'the in-flight guard should dedupe these');
    expect(controller.state.status, CustomerAuthStatus.authenticated);
  });
}

/// Stand-in for a transport-level failure; MockClient has no built-in way to
/// simulate one and the controller only cares that *something* was thrown.
class SocketExceptionStub implements Exception {
  const SocketExceptionStub();
}
