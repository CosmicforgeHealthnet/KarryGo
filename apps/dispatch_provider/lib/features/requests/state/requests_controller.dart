import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/app_mode.dart';
import '../../home/mock/dispatch_home_mock.dart';
import '../data/request_model.dart';
import '../data/requests_api.dart';

class RequestsController extends ChangeNotifier {
  final RequestsApi api;
  final String? Function() getAccessToken;
  final bool Function()? shouldPoll;

  List<RequestModel> _requests = [];
  RequestModel? _activeRequest;
  bool _isLoading = false;
  String? _error;

  Timer? _pollTimer;
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

  // ── Polling ───────────────────────────────────────────────────────────────

  void startPolling() {
    if (_pollTimer != null) return;
    _pollTimer = Timer.periodic(const Duration(seconds: 4), (_) => _pollTick());
    _pollTick();
  }

  void stopPolling() {
    _pollTimer?.cancel();
    _pollTimer = null;
  }

  void pokeRefresh() {
    if (_pollTimer == null) return;
    _pollTick();
  }

  Future<void> _pollTick() async {
    if (AppModeConfig.instance.isMock) return;
    if (_pollInFlight) return;
    if (shouldPoll != null && !shouldPoll!()) return;
    final token = _token();
    if (token == null || token.isEmpty) return;

    _pollInFlight = true;
    final generation = _stateGeneration;
    try {
      final merged = await _fetchAll(token);
      if (generation != _stateGeneration) return;
      _requests = merged;
      notifyListeners();
    } finally {
      _pollInFlight = false;
    }
  }

  void clearError() {
    _error = null;
    notifyListeners();
  }

  // ── Load ──────────────────────────────────────────────────────────────────

  Future<ApiResult<List<RequestModel>>> loadRequests() async {
    if (AppModeConfig.instance.isMock) {
      _requests = DispatchHomeMock.isOnline
          ? List<RequestModel>.from(DispatchHomeMock.pendingRequests)
          : <RequestModel>[];
      _error = null;
      notifyListeners();
      return ApiSuccess(_requests);
    }

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

    final merged = await _fetchAll(token);
    _requests = merged;
    _isLoading = false;
    notifyListeners();
    return ApiSuccess(merged);
  }

  /// Fetch all four sources in parallel. Fail-open per source: a 404 or 503
  /// from one endpoint (flag off) leaves that source empty without breaking
  /// the others.
  Future<List<RequestModel>> _fetchAll(String token) async {
    final results = await Future.wait<ApiResult<List<RequestModel>>>([
      api.listBroadcasts(accessToken: token),
      api.listAssignmentOffers(accessToken: token),
      api.listOrganizationOffers(accessToken: token),
      api.listCompanyAssignments(accessToken: token),
    ]);

    final merged = <RequestModel>[];
    for (final result in results) {
      result.when(
        success: (list) => merged.addAll(list),
        failure: (err) {
          // 404 / 503 are the "flag off" signals — treat as empty, not error.
          if (err.statusCode != 404 && err.statusCode != 503) {
            _debugLog('[REQUESTS] source failed: ${err.message}');
          }
        },
      );
    }

    // Stable ordering: assignments first (directed at you), then commerce,
    // then broadcast. Within a group, soonest-to-expire first.
    merged.sort((a, b) {
      final cmp = _sourceRank(a.source).compareTo(_sourceRank(b.source));
      if (cmp != 0) return cmp;
      final ae = a.expiresAt;
      final be = b.expiresAt;
      if (ae == null && be == null) return 0;
      if (ae == null) return 1;
      if (be == null) return -1;
      return ae.compareTo(be);
    });
    return merged;
  }

  static int _sourceRank(RequestSource s) => switch (s) {
        RequestSource.assignment => 0,
        RequestSource.commerce => 1,
        RequestSource.broadcast => 2,
      };

  // ── Single fetch ──────────────────────────────────────────────────────────

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
    // Broadcast is the only source with a per-item detail endpoint we
    // currently call. Others return a list-only view; we serve from cache.
    final existing = _requests.where((r) => r.id == id).firstOrNull;
    if (existing == null) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.notFound,
          message: 'Request not found.',
        ),
      );
    }
    return ApiSuccess(existing);
  }

  // ── Accept ────────────────────────────────────────────────────────────────

  Future<ApiResult<bool>> accept(RequestModel request) async {
    if (AppModeConfig.instance.isMock) {
      _activeRequest = request;
      _requests = _requests.where((r) => r.id != request.id).toList();
      DispatchHomeMock.pendingRequests = List<RequestModel>.from(_requests);
      DispatchHomeMock.activeTrip = DispatchHomeMock.tripFromRequest(request);
      notifyListeners();
      return const ApiSuccess(true);
    }

    final token = _token();
    if (token == null || token.isEmpty) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'No access token available.',
        ),
      );
    }
    _debugLog('[REQUESTS] accepting ${request.source.label} ${request.id}');
    final result = await api.accept(accessToken: token, request: request);
    result.when(
      success: (_) {
        _stateGeneration++;
        _activeRequest = request;
        _requests = _requests.where((r) => r.id != request.id).toList();
        notifyListeners();
      },
      failure: (err) =>
          _debugLog('[REQUESTS] accept ${request.id} failed: ${err.message}'),
    );
    return result;
  }

  // ── Reject / decline ──────────────────────────────────────────────────────

  Future<ApiResult<bool>> reject(RequestModel request, String reason) async {
    if (AppModeConfig.instance.isMock) {
      _requests = _requests.where((r) => r.id != request.id).toList();
      DispatchHomeMock.pendingRequests = List<RequestModel>.from(_requests);
      notifyListeners();
      return const ApiSuccess(true);
    }

    final token = _token();
    if (token == null || token.isEmpty) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'No access token available.',
        ),
      );
    }
    _debugLog('[REQUESTS] rejecting ${request.source.label} ${request.id}');
    final result = await api.reject(
      accessToken: token,
      request: request,
      reason: reason,
    );
    result.when(
      success: (_) {
        _stateGeneration++;
        _requests = _requests.where((r) => r.id != request.id).toList();
        notifyListeners();
      },
      failure: (err) =>
          _debugLog('[REQUESTS] reject ${request.id} failed: ${err.message}'),
    );
    return result;
  }

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