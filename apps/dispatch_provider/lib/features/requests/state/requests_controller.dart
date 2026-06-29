import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../data/request_model.dart';
import '../data/requests_api.dart';

class RequestsController extends ChangeNotifier {
  final RequestsApi api;
  final String? Function() getAccessToken;

  List<RequestModel> _requests = [];
  RequestModel? _activeRequest;
  bool _isLoading = false;
  String? _error;

  List<RequestModel> get requests => List.unmodifiable(_requests);
  RequestModel? get activeRequest => _activeRequest;
  bool get isLoading => _isLoading;
  String? get error => _error;

  RequestsController({required this.api, required this.getAccessToken});

  static void _debugLog(String message) {
    if (kDebugMode) debugPrint(message);
  }

  String? _token() => getAccessToken();

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
}
