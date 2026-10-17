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
  DateTime? _lastSuccessfulRefresh;
  String? _snapshotToken;
  static const _offerFreshnessWindow = Duration(seconds: 20);

  Timer? _pollTimer;
  bool _pollInFlight = false;
  int _stateGeneration = 0;
  final Set<String> _actionsInFlight = {};

  List<RequestModel> get requests => List.unmodifiable(_requests);
  RequestModel? get activeRequest => _activeRequest;
  bool get isLoading => _isLoading;
  String? get error => _error;
  bool get canAcceptOffers =>
      _error == null &&
      _lastSuccessfulRefresh != null &&
      _snapshotToken != null &&
      _snapshotToken == _token() &&
      (shouldPoll == null || shouldPoll!()) &&
      DateTime.now().difference(_lastSuccessfulRefresh!) <= _offerFreshnessWindow;

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
      final result = await _fetchAll(token);
      if (generation != _stateGeneration || token != _token()) return;
      result.when(
        success: (merged) {
          _requests = merged;
          _lastSuccessfulRefresh = DateTime.now();
          _snapshotToken = token;
          _error = null;
        },
        failure: (error) {
          // Keep the last good snapshot for diagnostics, never mark it fresh.
          _error = 'Unable to refresh opportunities. Retry to see current requests. '
              '${error.message}';
        },
      );
      notifyListeners();
    } finally {
      _pollInFlight = false;
    }
  }

  void clearError() {
    // Only a successful API response clears an error; user taps cannot mark
    // cached offers as current.
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
      _lastSuccessfulRefresh = null;
      _snapshotToken = null;
      _error = 'Please sign in again to view current opportunities.';
      notifyListeners();
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'No access token available.',
        ),
      );
    }
    final generation = ++_stateGeneration;
    _isLoading = true;
    // Do not unlock acceptance while a refresh is in progress.
    _lastSuccessfulRefresh = null;
    _error = null;
    notifyListeners();

    final result = await _fetchAll(token);
    if (generation != _stateGeneration || token != _token()) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.conflict,
          message: 'Session or request list changed. Refresh and try again.',
        ),
      );
    }
    result.when(
      success: (merged) {
        _requests = merged;
        _lastSuccessfulRefresh = DateTime.now();
        _snapshotToken = token;
        _error = null;
      },
      failure: (error) {
        _error = 'Unable to refresh opportunities. Retry to see current requests. '
            '${error.message}';
      },
    );
    _isLoading = false;
    notifyListeners();
    return result;
  }

  /// Fetch all four sources in parallel. Optional disabled endpoints may be
  /// empty; a transport/server failure must not masquerade as a fresh list.
  Future<ApiResult<List<RequestModel>>> _fetchAll(String token) async {
    final results = await Future.wait<ApiResult<List<RequestModel>>>([
      api.listBroadcasts(accessToken: token),
      api.listAssignmentOffers(accessToken: token),
      api.listOrganizationOffers(accessToken: token),
      api.listCompanyAssignments(accessToken: token),
    ]);

    final merged = <RequestModel>[];
    ApiException? firstFailure;
    for (final result in results) {
      result.when(
        success: (list) => merged.addAll(list),
        failure: (err) {
          firstFailure ??= err;
          _debugLog('[REQUESTS] source failed: ${err.message}');
        },
      );
    }
    if (firstFailure != null) return ApiFailure(firstFailure!);

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
    return ApiSuccess(merged);
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
    // Cached or partially failed source data must not authorize a new job.
    if (!canAcceptOffers ||
        !_requests.any((candidate) =>
            candidate.id == request.id && candidate.kind == request.kind) ||
        !request.isAcceptable) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.conflict,
          message: 'This offer may have changed. Refresh opportunities before accepting.',
        ),
      );
    }
    final key = '${_stateGeneration}:${request.kind.name}:${request.id}';
    if (!_actionsInFlight.add(key)) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.conflict,
          message: 'This request is already being processed.',
        ),
      );
    }
    final generation = _stateGeneration;
    try {
      _debugLog('[REQUESTS] accepting ${request.source.label} ${request.id}');
      final result = await api.accept(accessToken: token, request: request);
      if (generation != _stateGeneration || token != _token()) {
        return ApiFailure(
          const ApiException(
            code: ApiErrorCode.conflict,
            message: 'Your session or offer changed. Refresh your trips.',
          ),
        );
      }
      result.when(
        success: (_) {
          _stateGeneration++;
          _activeRequest = request;
          _requests = _requests.where(
            (r) => r.id != request.id || r.kind != request.kind,
          ).toList();
          _lastSuccessfulRefresh = null;
          notifyListeners();
        },
        failure: (err) =>
            _debugLog('[REQUESTS] accept ${request.id} failed: ${err.message}'),
      );
      return result;
    } finally {
      _actionsInFlight.remove(key);
    }
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
    final key = '${_stateGeneration}:${request.kind.name}:${request.id}';
    if (!_actionsInFlight.add(key)) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.conflict,
          message: 'This request is already being processed.',
        ),
      );
    }
    final generation = _stateGeneration;
    try {
      _debugLog('[REQUESTS] rejecting ${request.source.label} ${request.id}');
      final result = await api.reject(
        accessToken: token,
        request: request,
        reason: reason,
      );
      if (generation != _stateGeneration || token != _token()) {
        return ApiFailure(
          const ApiException(
            code: ApiErrorCode.conflict,
            message: 'Your session or offer changed. Refresh opportunities.',
          ),
        );
      }
      result.when(
        success: (_) {
          _stateGeneration++;
          _requests = _requests.where(
            (r) => r.id != request.id || r.kind != request.kind,
          ).toList();
          _lastSuccessfulRefresh = null;
          notifyListeners();
        },
        failure: (err) =>
            _debugLog('[REQUESTS] reject ${request.id} failed: ${err.message}'),
      );
      return result;
    } finally {
      _actionsInFlight.remove(key);
    }
  }

  void clearActiveRequest() {
    _activeRequest = null;
    notifyListeners();
  }

  /// Call on logout, account deletion and session expiry. Invalidates
  /// in-flight responses so one provider never sees another's offers.
  void resetSession() {
    _stateGeneration++;
    stopPolling();
    _requests = [];
    _activeRequest = null;
    _isLoading = false;
    _error = null;
    _lastSuccessfulRefresh = null;
    _snapshotToken = null;
    _actionsInFlight.clear();
    notifyListeners();
  }

  @override
  void dispose() {
    stopPolling();
    super.dispose();
  }
}