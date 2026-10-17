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
  final _sourceCache = <int, List<RequestModel>>{};
  ApiException? _loadFailure;
  RequestModel? _activeRequest;
  bool _isLoading = false;
  String? _error;

  Timer? _pollTimer;
  bool _pollInFlight = false;
  int _stateGeneration = 0;
  int _fetchGeneration = 0;
  bool _disposed = false;
  Timer? _expiryTimer;
  final Set<String> _handled = {};
  void _notify() {
    if (!_disposed) notifyListeners();
  }

  void _scheduleExpiry() {
    _expiryTimer?.cancel();
    final expiries =
        _requests
            .where((r) => !r.isExpired && r.expiresAt != null)
            .map((r) => r.expiresAt!)
            .toList()
          ..sort();
    if (expiries.isEmpty || _disposed) return;
    final delay = expiries.first.difference(DateTime.now());
    _expiryTimer = Timer(delay.isNegative ? Duration.zero : delay, () {
      _notify();
      _scheduleExpiry();
    });
  }

  void _removeHandled(RequestModel request) {
    _stateGeneration++;
    _handled.add('${request.source.name}:${request.id}');
    _requests.removeWhere(
      (r) => r.id == request.id && r.source == request.source,
    );
    for (final list in _sourceCache.values) {
      list.removeWhere((r) => r.id == request.id && r.source == request.source);
    }
    _scheduleExpiry();
  }

  List<RequestModel> get requests => List.unmodifiable(
    _requests.where(
      (request) =>
          request.expiresAt == null ||
          request.expiresAt!.isAfter(DateTime.now()),
    ),
  );
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
    if (_pollInFlight || _isLoading || _disposed) return;
    if (shouldPoll != null && !shouldPoll!()) return;
    final token = _token();
    if (token == null || token.isEmpty) return;

    _pollInFlight = true;
    final generation = _stateGeneration;
    try {
      final merged = await _fetchAll(token);
      if (generation != _stateGeneration) return;
      _requests = merged;
      _scheduleExpiry();
      _notify();
    } finally {
      _pollInFlight = false;
    }
  }

  void clearError() {
    _error = null;
    _notify();
  }

  // ── Load ──────────────────────────────────────────────────────────────────

  Future<ApiResult<List<RequestModel>>> loadRequests() async {
    if (_disposed || _isLoading) return ApiSuccess(requests);
    if (AppModeConfig.instance.isMock) {
      _requests = DispatchHomeMock.isOnline
          ? List<RequestModel>.from(DispatchHomeMock.pendingRequests)
          : <RequestModel>[];
      _error = null;
      _scheduleExpiry();
      _notify();
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
    _notify();

    final merged = await _fetchAll(token);
    if (_disposed) return ApiSuccess(merged);
    _requests = merged;
    _scheduleExpiry();
    _isLoading = false;
    _notify();
    if (_error != null && merged.isEmpty) {
      return ApiFailure(
        _loadFailure ??
            ApiException(code: ApiErrorCode.network, message: _error!),
      );
    }
    return ApiSuccess(merged);
  }

  Future<List<RequestModel>> _fetchAll(String token) async {
    final fetchGeneration = ++_fetchGeneration;
    final results = await Future.wait<ApiResult<List<RequestModel>>>([
      api.listBroadcasts(accessToken: token),
      api.listAssignmentOffers(accessToken: token),
      api.listOrganizationOffers(accessToken: token),
      api.listCompanyAssignments(accessToken: token),
    ]);

    if (_disposed || fetchGeneration != _fetchGeneration) return requests;
    final merged = <RequestModel>[];
    final failures = <ApiException>[];
    var successfulSources = 0;
    _error = null;
    _loadFailure = null;
    for (final entry in results.asMap().entries) {
      entry.value.when(
        success: (list) {
          successfulSources++;
          final current = list
              .where((r) => !_handled.contains('${r.source.name}:${r.id}'))
              .toList();
          _sourceCache[entry.key] = current;
          merged.addAll(current);
        },
        failure: (err) {
          if (err.statusCode == 404) {
            _sourceCache.remove(entry.key);
          } else {
            failures.add(err);
            merged.addAll(_sourceCache[entry.key] ?? const <RequestModel>[]);
            _debugLog('[REQUESTS] source failed: ${err.message}');
          }
        },
      );
    }

    if (failures.isNotEmpty) {
      _loadFailure = successfulSources == 0 ? failures.first : null;
      _error = successfulSources == 0
          ? failures.first.message
          : 'Some opportunities could not be refreshed. Showing the last available requests.';
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
    return merged
        .where(
          (request) =>
              request.expiresAt == null ||
              request.expiresAt!.isAfter(DateTime.now()),
        )
        .toList();
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
    final existing = requests.where((r) => r.id == id).firstOrNull;
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
    if (request.isExpired)
      return const ApiFailure(
        ApiException(
          code: ApiErrorCode.conflict,
          message: 'This opportunity has expired. Choose another request.',
        ),
      );
    if (AppModeConfig.instance.isMock) {
      _activeRequest = request;
      _requests = _requests.where((r) => r.id != request.id).toList();
      DispatchHomeMock.pendingRequests = List<RequestModel>.from(_requests);
      DispatchHomeMock.activeTrip = DispatchHomeMock.tripFromRequest(request);
      _notify();
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
        _removeHandled(request);
        _activeRequest = request;
        _notify();
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
      _notify();
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
        _removeHandled(request);
        _notify();
      },
      failure: (err) =>
          _debugLog('[REQUESTS] reject ${request.id} failed: ${err.message}'),
    );
    return result;
  }

  void clearActiveRequest() {
    _activeRequest = null;
    _notify();
  }

  @override
  void dispose() {
    _disposed = true;
    _stateGeneration++;
    _fetchGeneration++;
    _expiryTimer?.cancel();
    stopPolling();
    super.dispose();
  }
}
