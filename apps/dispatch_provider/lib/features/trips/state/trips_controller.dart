import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/app_mode.dart';
import '../../home/mock/dispatch_home_mock.dart';
import '../data/trip_model.dart';
import '../data/trips_api.dart';
import '../mock/trips_tab_mock.dart';

class TripsController extends ChangeNotifier {
  final TripsApi api;
  final String? Function() getAccessToken;

  List<TripModel> _trips = [];
  TripModel? _activeTrip;
  bool _isLoading = false;
  bool _isActionLoading = false;
  String? _error;

  Timer? _activePollTimer;
  bool _activePollInFlight = false;

  PickupCredential? _pickupCredential;
  bool _isResendingPickupCredential = false;

  /// Polls `/trips/:id/pickup-credential` while the rider is at pickup, so the
  /// "Start Trip" gate flips to enabled the moment the customer scans their
  /// pickup code. Stopped when the credential reports `verified=true`, when
  /// the trip leaves `arrived_pickup`, or on dispose.
  Timer? _pickupCredentialPollTimer;

  List<TripModel> get trips => List.unmodifiable(_trips);
  TripModel? get activeTrip => _activeTrip;
  bool get isLoading => _isLoading;
  bool get isActionLoading => _isActionLoading;
  String? get error => _error;
  PickupCredential? get pickupCredential => _pickupCredential;
  bool get isResendingPickupCredential => _isResendingPickupCredential;

  TripsController({required this.api, required this.getAccessToken});

  static void _debugLog(String message) {
    if (kDebugMode) debugPrint(message);
  }

  String? _token() => getAccessToken();

  // ── Active-trip polling ───────────────────────────────────────────────────

  void startActivePolling() {
    if (_activePollTimer != null) return;
    _activePollTimer = Timer.periodic(
      const Duration(seconds: 6),
      (_) => _activePollTick(),
    );
  }

  void stopActivePolling() {
    _activePollTimer?.cancel();
    _activePollTimer = null;
  }

  void pokeRefresh() {
    if (_activePollTimer == null) return;
    _activePollTick();
  }

  Future<void> _activePollTick() async {
    if (AppModeConfig.instance.isMock) return;
    if (_activePollInFlight) return;
    final token = _token();
    if (token == null || token.isEmpty) return;
    if (_isActionLoading) return;
    _activePollInFlight = true;
    try {
      await loadActiveTrip();
    } finally {
      _activePollInFlight = false;
    }
  }

  ApiFailure<T> _unauthorized<T>() => ApiFailure(
    const ApiException(
      code: ApiErrorCode.unauthorized,
      message: 'No access token available.',
    ),
  );

  void clearError() {
    _error = null;
    notifyListeners();
  }

  // ── Load all trips ────────────────────────────────────────────────────────

  Future<ApiResult<List<TripModel>>> loadTrips() async {
    if (AppModeConfig.instance.isMock) {
      _isLoading = true;
      _error = null;
      notifyListeners();

      await Future.delayed(const Duration(milliseconds: 300));
      _trips = TripsTabMock.seed();
      _isLoading = false;
      notifyListeners();
      return ApiSuccess(_trips);
    }

    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isLoading = true;
    _error = null;
    notifyListeners();

    final result = await api.listTrips(accessToken: token);
    result.when(
      success: (list) {
        _trips = list;
        _debugLog('[TRIPS] loaded ${list.length} trip(s)');
      },
      failure: (err) {
        _error = err.message;
        _debugLog('[TRIPS] loadTrips failed: ${err.message}');
      },
    );
    _isLoading = false;
    notifyListeners();
    return result;
  }

  // ── Load active trip ──────────────────────────────────────────────────────

  Future<ApiResult<TripModel?>> loadActiveTrip() async {
    if (AppModeConfig.instance.isMock) {
      _activeTrip = DispatchHomeMock.activeTrip;
      notifyListeners();
      return ApiSuccess(_activeTrip);
    }

    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    final result = await api.getActiveTrip(accessToken: token);
    result.when(
      success: (trip) {
        _activeTrip = trip;
        if (trip?.statusCode != TripStatusCode.arrivedPickup) {
          _stopPickupCredentialPolling();
        }
        _debugLog('[TRIPS] activeTrip=${trip?.id ?? 'none'}');
        notifyListeners();
      },
      failure: (err) {
        _debugLog('[TRIPS] loadActiveTrip failed: ${err.message}');
      },
    );
    return result;
  }

  // ── Load single trip ──────────────────────────────────────────────────────

  Future<ApiResult<TripModel>> loadTrip(String id) async {
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    final result = await api.getTrip(accessToken: token, id: id);
    result.when(
      success: (trip) {
        final idx = _trips.indexWhere((t) => t.id == id);
        if (idx >= 0) {
          _trips = [..._trips]..[idx] = trip;
        }
        if (_activeTrip?.id == id) _activeTrip = trip;
        notifyListeners();
      },
      failure: (err) {
        _debugLog('[TRIPS] loadTrip $id failed: ${err.message}');
      },
    );
    return result;
  }

  // ── Mark arrived ──────────────────────────────────────────────────────────

  Future<ApiResult<PickupCredential>> markArrived(String id) async {
    if (AppModeConfig.instance.isMock) {
      _isActionLoading = true;
      notifyListeners();
      await Future.delayed(const Duration(milliseconds: 600));

      final trip = DispatchHomeMock.activeTrip;
      if (trip != null) {
        DispatchHomeMock.activeTrip = DispatchHomeMock.advanceTrip(
          trip,
          TripStatusCode.arrivedPickup,
        );
        _activeTrip = DispatchHomeMock.activeTrip;
      }
      _pickupCredential = PickupCredential(
        code: '482913',
        expiresAt: DateTime.now().add(const Duration(minutes: 10)),
      );
      _isActionLoading = false;
      notifyListeners();
      return ApiSuccess(_pickupCredential!);
    }

    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();

    final result = await api.markArrived(accessToken: token, id: id);
    result.when(
      success: (credential) {
        _pickupCredential = credential;
        if (!credential.verified) {
          _startPickupCredentialPolling(id);
        }
        _debugLog('[TRIPS] markArrived $id ok');
      },
      failure: (err) =>
          _debugLog('[TRIPS] markArrived $id failed: ${err.message}'),
    );
    _isActionLoading = false;
    notifyListeners();
    return result;
  }

  Future<void> fetchPickupCredential(String id) async {
    final token = _token();
    if (token == null || token.isEmpty) return;
    final result = await api.getPickupCredential(accessToken: token, id: id);
    result.when(
      success: (credential) {
        _pickupCredential = credential;
        notifyListeners();
        if (!credential.verified) {
          _startPickupCredentialPolling(id);
        } else {
          _stopPickupCredentialPolling();
        }
      },
      failure: (_) {},
    );
  }

  Future<void> resendPickupCredential(String id) async {
    final token = _token();
    if (token == null || token.isEmpty) return;
    _isResendingPickupCredential = true;
    notifyListeners();
    final result = await api.resendPickupCredential(accessToken: token, id: id);
    result.when(
      success: (credential) {
        _pickupCredential = credential;
        _startPickupCredentialPolling(id);
      },
      failure: (err) => _error = err.message,
    );
    _isResendingPickupCredential = false;
    notifyListeners();
  }

  // ── Pickup-credential verification watch ──────────────────────────────────

  void _startPickupCredentialPolling(String tripId) {
    _pickupCredentialPollTimer?.cancel();
    _pickupCredentialPollTimer = Timer.periodic(
      const Duration(seconds: 4),
      (_) => _pollPickupCredential(tripId),
    );
    _debugLog('[TRIPS] pickup-credential watch started trip=$tripId');
  }

  void _stopPickupCredentialPolling() {
    if (_pickupCredentialPollTimer == null) return;
    _pickupCredentialPollTimer?.cancel();
    _pickupCredentialPollTimer = null;
    _debugLog('[TRIPS] pickup-credential watch stopped');
  }

  Future<void> _pollPickupCredential(String tripId) async {
    final active = _activeTrip;
    if (active == null ||
        active.id != tripId ||
        active.statusCode != TripStatusCode.arrivedPickup) {
      _stopPickupCredentialPolling();
      return;
    }

    final token = _token();
    if (token == null || token.isEmpty) return;

    final result = await api.getPickupCredential(accessToken: token, id: tripId);
    result.when(
      success: (credential) {
        final prev = _pickupCredential;
        final changed = prev?.verified != credential.verified ||
            prev?.code != credential.code ||
            prev?.expiresAt != credential.expiresAt;
        _pickupCredential = credential;
        if (changed) notifyListeners();
        if (credential.verified) {
          _debugLog('[TRIPS] pickup verified by customer trip=$tripId');
          _stopPickupCredentialPolling();
        }
      },
      failure: (_) {},
    );
  }

  // ── Start trip ────────────────────────────────────────────────────────────

  Future<ApiResult<bool>> startTrip(String id) async {
    if (AppModeConfig.instance.isMock) {
      _isActionLoading = true;
      notifyListeners();
      await Future.delayed(const Duration(milliseconds: 600));

      final trip = DispatchHomeMock.activeTrip;
      if (trip != null) {
        DispatchHomeMock.activeTrip = DispatchHomeMock.advanceTrip(
          trip,
          TripStatusCode.inProgress,
        );
        _activeTrip = DispatchHomeMock.activeTrip;
      }
      _pickupCredential = null;
      _stopPickupCredentialPolling();
      _isActionLoading = false;
      notifyListeners();
      return const ApiSuccess(true);
    }

    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();

    final result = await api.startTrip(accessToken: token, id: id);
    result.when(
      success: (_) {
        _pickupCredential = null;
        _stopPickupCredentialPolling();
        _debugLog('[TRIPS] startTrip $id ok');
      },
      failure: (err) =>
          _debugLog('[TRIPS] startTrip $id failed: ${err.message}'),
    );
    _isActionLoading = false;
    notifyListeners();
    return result;
  }

  // ── Submit proof ──────────────────────────────────────────────────────────

  Future<ApiResult<bool>> submitProof({
    required String id,
    required String photoPath,
    required Uint8List signaturePng,
    required String receiverName,
    required String receiverPhone,
    required String code,
  }) async {
    if (AppModeConfig.instance.isMock) {
      _isActionLoading = true;
      notifyListeners();
      await Future.delayed(const Duration(milliseconds: 700));

      final trip = DispatchHomeMock.activeTrip;
      if (trip != null) {
        DispatchHomeMock.activeTrip = DispatchHomeMock.advanceTrip(
          trip,
          TripStatusCode.proofSubmitted,
        );
        _activeTrip = DispatchHomeMock.activeTrip;
      }
      _isActionLoading = false;
      notifyListeners();
      return const ApiSuccess(true);
    }

    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();

    final result = await api.submitProof(
      accessToken: token,
      id: id,
      photoPath: photoPath,
      signaturePng: signaturePng,
      receiverName: receiverName,
      receiverPhone: receiverPhone,
      code: code,
    );
    result.when(
      success: (_) => _debugLog('[TRIPS] submitProof $id ok'),
      failure: (err) =>
          _debugLog('[TRIPS] submitProof $id failed: ${err.message}'),
    );
    _isActionLoading = false;
    notifyListeners();
    return result;
  }

  // ── Get proof ─────────────────────────────────────────────────────────────

  Future<ApiResult<Map<String, dynamic>>> getProof(String id) async {
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 200));
      return const ApiSuccess(<String, dynamic>{
        'photo_ref': 'mock://photo.jpg',
        'signature_ref': 'mock://signature.png',
        'receiver_name': 'Mock Receiver',
        'receiver_phone': '+2348000000000',
        'verified': true,
      });
    }
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();
    final result = await api.getProof(accessToken: token, id: id);
    result.when(
      success: (_) => _debugLog('[TRIPS] getProof $id ok'),
      failure: (err) => _debugLog('[TRIPS] getProof $id failed: ${err.message}'),
    );
    return result;
  }

  // ── Complete trip ─────────────────────────────────────────────────────────

  Future<ApiResult<bool>> completeTrip(String id) async {
    if (AppModeConfig.instance.isMock) {
      _isActionLoading = true;
      notifyListeners();
      await Future.delayed(const Duration(milliseconds: 500));

      DispatchHomeMock.activeTrip = null;
      _activeTrip = null;
      _stopPickupCredentialPolling();
      _isActionLoading = false;
      notifyListeners();
      return const ApiSuccess(true);
    }

    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();

    final result = await api.completeTrip(accessToken: token, id: id);
    result.when(
      success: (_) {
        _activeTrip = null;
        _stopPickupCredentialPolling();
        _debugLog('[TRIPS] completeTrip $id ok');
        notifyListeners();
      },
      failure: (err) =>
          _debugLog('[TRIPS] completeTrip $id failed: ${err.message}'),
    );
    _isActionLoading = false;
    notifyListeners();
    return result;
  }

  // ── Cancel trip ───────────────────────────────────────────────────────────

  Future<ApiResult<bool>> cancelTrip(
    String id, {
    required String reasonCode,
    String reasonText = '',
  }) async {
    if (AppModeConfig.instance.isMock) {
      _isActionLoading = true;
      notifyListeners();
      await Future.delayed(const Duration(milliseconds: 400));

      DispatchHomeMock.activeTrip = null;
      _activeTrip = null;
      _pickupCredential = null;
      _stopPickupCredentialPolling();
      _isActionLoading = false;
      notifyListeners();
      return const ApiSuccess(true);
    }

    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();

    final result = await api.cancelTrip(
      accessToken: token,
      id: id,
      reasonCode: reasonCode,
      reasonText: reasonText,
    );
    result.when(
      success: (_) {
        _activeTrip = null;
        _pickupCredential = null;
        _stopPickupCredentialPolling();
        _debugLog('[TRIPS] cancelTrip $id ok');
        notifyListeners();
      },
      failure: (err) =>
          _debugLog('[TRIPS] cancelTrip $id failed: ${err.message}'),
    );
    _isActionLoading = false;
    notifyListeners();
    return result;
  }

  // ── Rate customer ─────────────────────────────────────────────────────────

  Future<ApiResult<bool>> rateCustomer({
    required String id,
    required int score,
    String? comment,
  }) async {
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 300));
      return const ApiSuccess(true);
    }
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();
    final result = await api.rateCustomer(
      accessToken: token,
      id: id,
      score: score,
      comment: comment,
    );
    result.when(
      success: (_) => _debugLog('[TRIPS] rateCustomer $id ok score=$score'),
      failure: (err) =>
          _debugLog('[TRIPS] rateCustomer $id failed: ${err.message}'),
    );
    return result;
  }

  // ── Recipient confirmation ────────────────────────────────────────────────

  Future<ApiResult<bool>> confirmRecipient({
    required String id,
    required String verificationMethod,
    required String recipientContact,
    required int expectedSourceVersion,
    required String idempotencyKey,
  }) async {
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 200));
      return const ApiSuccess(true);
    }
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();
    final result = await api.confirmRecipient(
      accessToken: token,
      id: id,
      verificationMethod: verificationMethod,
      recipientContact: recipientContact,
      expectedSourceVersion: expectedSourceVersion,
      idempotencyKey: idempotencyKey,
    );
    result.when(
      success: (_) => _debugLog('[TRIPS] confirmRecipient $id ok'),
      failure: (err) =>
          _debugLog('[TRIPS] confirmRecipient $id failed: ${err.message}'),
    );
    return result;
  }

  Future<ApiResult<Map<String, dynamic>>> getRecipientConfirmation(
    String id,
  ) async {
    if (AppModeConfig.instance.isMock) {
      await Future.delayed(const Duration(milliseconds: 200));
      return const ApiSuccess(<String, dynamic>{
        'trip_id': 'mock-trip',
        'verification_method': 'code',
        'confirmed_at': null,
      });
    }
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();
    return api.getRecipientConfirmation(accessToken: token, id: id);
  }

  // ── Fail delivery ─────────────────────────────────────────────────────────

  Future<ApiResult<bool>> failDelivery({
    required String id,
    required String reasonCode,
    String? operatorNotes,
    String? attemptReference,
    String? incidentReference,
    required int expectedSourceVersion,
    required String idempotencyKey,
  }) async {
    if (AppModeConfig.instance.isMock) {
      _isActionLoading = true;
      notifyListeners();
      await Future.delayed(const Duration(milliseconds: 400));
      DispatchHomeMock.activeTrip = null;
      _activeTrip = null;
      _pickupCredential = null;
      _stopPickupCredentialPolling();
      _isActionLoading = false;
      notifyListeners();
      return const ApiSuccess(true);
    }
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();
    final result = await api.failDelivery(
      accessToken: token,
      id: id,
      reasonCode: reasonCode,
      operatorNotes: operatorNotes,
      attemptReference: attemptReference,
      incidentReference: incidentReference,
      expectedSourceVersion: expectedSourceVersion,
      idempotencyKey: idempotencyKey,
    );
    result.when(
      success: (_) {
        _activeTrip = null;
        _pickupCredential = null;
        _stopPickupCredentialPolling();
        _debugLog('[TRIPS] failDelivery $id ok reason=$reasonCode');
      },
      failure: (err) =>
          _debugLog('[TRIPS] failDelivery $id failed: ${err.message}'),
    );
    _isActionLoading = false;
    notifyListeners();
    return result;
  }

  void clearActiveTrip() {
    _activeTrip = null;
    _stopPickupCredentialPolling();
    notifyListeners();
  }

  @override
  void dispose() {
    stopActivePolling();
    _stopPickupCredentialPolling();
    super.dispose();
  }
}