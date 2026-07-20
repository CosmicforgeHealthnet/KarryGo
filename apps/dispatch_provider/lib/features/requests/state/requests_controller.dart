import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../data/request_model.dart';
import '../data/requests_api.dart';

class RequestsController extends ChangeNotifier {
  final RequestsApi api;
  final String? Function() getAccessToken;

  /// When set, the periodic poll only fetches while this returns true (used to
  /// gate polling on the rider being online).
  final bool Function()? shouldPoll;

  List<RequestModel> _requests = [];
  RequestModel? _activeRequest;
  bool _isLoading = false;
  String? _error;

  Timer? _pollTimer;

  /// Guards the request poll: one fetch in flight at a time, and a response
  /// started before a local mutation (accept/reject) is dropped — otherwise a
  /// poll that began just before an accept can land after it and revive the
  /// just-accepted request in the pending list. Mirrors the truck provider's
  /// home controller pattern.
  bool _pollInFlight = false;
  int _stateGeneration = 0;

  List<RequestModel> get requests => List.unmodifiable(_requests);
  RequestModel? get activeRequest => _activeRequest;
  bool get isLoading => _isLoading;
  String? get error => _error;

  RequestsController({
    required this.api,
    required this.getAccessToken,
    this.shouldPoll,
  });

  static void _debugLog(String message) {
    if (kDebugMode) debugPrint(message);
  }

  String? _token() => getAccessToken();

  // ── Periodic polling (new-request delivery) ───────────────────────────────

  /// Starts the 4-second inbox poll. Idempotent. The poll is the safety net;
  /// realtime websocket / FCM pushes call [pokeRefresh] for immediacy.
  void startPolling() {
    if (_pollTimer != null) return;
    _pollTimer = Timer.periodic(const Duration(seconds: 4), (_) => _pollTick());
    _pollTick();
  }

  void stopPolling() {
    _pollTimer?.cancel();
    _pollTimer = null;
  }

  /// Fast-path poke (realtime event or foreground FCM push): refresh the inbox
  /// immediately instead of waiting for the next tick.
  void pokeRefresh() {
    if (_pollTimer == null) return;
    _pollTick();
  }

  Future<void> _pollTick() async {
    if (_pollInFlight) return;
    if (shouldPoll != null && !shouldPoll!()) return;
    final token = _token();
    if (token == null || token.isEmpty) return;

    _pollInFlight = true;
    final generation = _stateGeneration;
    try {
      final result = await api.listRequests(accessToken: token);
      if (generation != _stateGeneration) return; // stale — a mutation landed
      result.when(
        success: (list) {
          _requests = list;
          notifyListeners();
        },
        failure: (err) {
          _debugLog('[REQUESTS] poll failed: ${err.message}');
        },
      );
    } finally {
      _pollInFlight = false;
    }
  }

  void clearError() {
    _error = null;
    notifyListeners();
  }

  // ── Load all pending requests ─────────────────────────────────────────────

  Future<ApiResult<List<RequestModel>>> loadRequests() async {
    final token = _token();
    if (token == null || token.isEmpty) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'No access token available.',
        ),
      );
    }
    _isLoading = true;
    _error = null;
    notifyListeners();

    final result = await api.listRequests(accessToken: token);
    result.when(
      success: (list) {
        _requests = list;
        _debugLog('[REQUESTS] loaded ${list.length} request(s)');
      },
      failure: (err) {
        _error = err.message;
        _debugLog('[REQUESTS] loadRequests failed: ${err.message}');
      },
    );
    _isLoading = false;
    notifyListeners();
    return result;
  }

  // ── Load single request ───────────────────────────────────────────────────

  Future<ApiResult<RequestModel>> loadRequest(String id) async {
    final token = _token();
    if (token == null || token.isEmpty) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'No access token available.',
        ),
      );
    }
    final result = await api.getRequest(accessToken: token, id: id);
    result.when(
      success: (req) {
        final idx = _requests.indexWhere((r) => r.id == req.id);
        if (idx >= 0) {
          _requests = [..._requests]..[idx] = req;
        }
        _debugLog('[REQUESTS] loadRequest $id ok');
        notifyListeners();
      },
      failure: (err) {
        _debugLog('[REQUESTS] loadRequest $id failed: ${err.message}');
      },
    );
    return result;
  }

  // ── Accept request ────────────────────────────────────────────────────────

  Future<ApiResult<bool>> accept(String id) async {
    final token = _token();
    if (token == null || token.isEmpty) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'No access token available.',
        ),
      );
    }
    _debugLog('[REQUESTS] accepting $id');
    final result = await api.acceptRequest(accessToken: token, id: id);
    result.when(
      success: (_) {
        _stateGeneration++; // drop any in-flight poll fetched pre-accept
        final req = _requests.where((r) => r.id == id).firstOrNull;
        _activeRequest = req;
        // Remove from pending list — it's now an active trip.
        _requests = _requests.where((r) => r.id != id).toList();
        _debugLog('[REQUESTS] accepted $id — activeRequest set');
        notifyListeners();
      },
      failure: (err) {
        _debugLog('[REQUESTS] accept $id failed: ${err.message}');
      },
    );
    return result;
  }

  // ── Reject request ────────────────────────────────────────────────────────

  Future<ApiResult<bool>> reject(String id, String reason) async {
    final token = _token();
    if (token == null || token.isEmpty) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'No access token available.',
        ),
      );
    }
    _debugLog('[REQUESTS] rejecting $id reason="$reason"');
    final result = await api.rejectRequest(
      accessToken: token,
      id: id,
      reason: reason,
    );
    result.when(
      success: (_) {
        _stateGeneration++; // drop any in-flight poll fetched pre-reject
        _requests = _requests.where((r) => r.id != id).toList();
        _debugLog('[REQUESTS] rejected $id — removed from list');
        notifyListeners();
      },
      failure: (err) {
        _debugLog('[REQUESTS] reject $id failed: ${err.message}');
      },
    );
    return result;
  }

  // ── Clear active request (e.g. after trip completed) ─────────────────────

  void clearActiveRequest() {
    _activeRequest = null;
    notifyListeners();
  }

  @override
  void dispose() {
    stopPolling();
    super.dispose();
  }
}
