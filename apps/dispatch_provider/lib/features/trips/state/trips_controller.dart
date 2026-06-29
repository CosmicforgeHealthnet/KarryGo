import 'package:flutter/foundation.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../data/trip_model.dart';
import '../data/trips_api.dart';

class TripsController extends ChangeNotifier {
  final TripsApi api;
  final String? Function() getAccessToken;

  List<TripModel> _trips = [];
  TripModel? _activeTrip;
  bool _isLoading = false;
  bool _isActionLoading = false;
  String? _error;

  List<TripModel> get trips => List.unmodifiable(_trips);
  TripModel? get activeTrip => _activeTrip;
  bool get isLoading => _isLoading;
  bool get isActionLoading => _isActionLoading;
  String? get error => _error;

  TripsController({required this.api, required this.getAccessToken});

  static void _debugLog(String message) {
    if (kDebugMode) debugPrint(message);
  }

  String? _token() => getAccessToken();

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
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    final result = await api.getActiveTrip(accessToken: token);
    result.when(
      success: (trip) {
        _activeTrip = trip;
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

  Future<ApiResult<bool>> markArrived(String id) async {
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();

    final result = await api.markArrived(accessToken: token, id: id);
    result.when(
      success: (_) => _debugLog('[TRIPS] markArrived $id ok'),
      failure: (err) =>
          _debugLog('[TRIPS] markArrived $id failed: ${err.message}'),
    );
    _isActionLoading = false;
    notifyListeners();
    return result;
  }

  // ── Start trip ────────────────────────────────────────────────────────────

  Future<ApiResult<bool>> startTrip(String id) async {
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();

    final result = await api.startTrip(accessToken: token, id: id);
    result.when(
      success: (_) => _debugLog('[TRIPS] startTrip $id ok'),
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
    required String filePath,
  }) async {
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();

    final result = await api.submitProof(
      accessToken: token,
      id: id,
      filePath: filePath,
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

  // ── Complete trip ─────────────────────────────────────────────────────────

  Future<ApiResult<bool>> completeTrip(String id) async {
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();

    final result = await api.completeTrip(accessToken: token, id: id);
    result.when(
      success: (_) {
        _activeTrip = null;
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

  Future<ApiResult<bool>> cancelTrip(String id) async {
    final token = _token();
    if (token == null || token.isEmpty) return _unauthorized();

    _isActionLoading = true;
    notifyListeners();

    final result = await api.cancelTrip(accessToken: token, id: id);
    result.when(
      success: (_) {
        _activeTrip = null;
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

  void clearActiveTrip() {
    _activeTrip = null;
    notifyListeners();
  }
}
