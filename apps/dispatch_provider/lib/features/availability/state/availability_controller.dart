import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:geolocator/geolocator.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../data/availability_api.dart';
import '../data/availability_models.dart';

class AvailabilityController extends ChangeNotifier {
  final AvailabilityApi api;
  final String? Function() getAccessToken;

  ProviderAvailability? _availability;
  AvailabilitySession? _currentSession;
  ProviderLocation? _lastLocation;

  bool _isLoading = false;
  bool _isChangingStatus = false;
  bool _isSendingLocation = false;
  String? _errorMessage;

  StreamSubscription<Position>? _positionSub;

  ProviderAvailability? get availability => _availability;
  AvailabilitySession? get currentSession => _currentSession;
  ProviderLocation? get lastLocation => _lastLocation;

  bool get isLoading => _isLoading;
  bool get isChangingStatus => _isChangingStatus;
  String? get errorMessage => _errorMessage;

  bool get isOnline => _availability?.isOnline ?? false;

  AvailabilityController({required this.api, required this.getAccessToken});

  static void _debugLog(String message) {
    if (kDebugMode) debugPrint(message);
  }

  String? _token() => getAccessToken();

  // ── Location permission ───────────────────────────────────────────────────

  /// Returns null on success, or an error message string if permission is
  /// denied or location services are disabled.
  Future<String?> _ensureLocationPermission() async {
    final serviceEnabled = await Geolocator.isLocationServiceEnabled();
    if (!serviceEnabled) {
      return 'Please enable location services to go online.';
    }

    var permission = await Geolocator.checkPermission();
    if (permission == LocationPermission.denied) {
      permission = await Geolocator.requestPermission();
      if (permission == LocationPermission.denied) {
        return 'Location permission is required to go online.';
      }
    }

    if (permission == LocationPermission.deniedForever) {
      return 'Location permission is permanently denied. Please enable it in app settings.';
    }

    return null;
  }

  // ── GPS stream ────────────────────────────────────────────────────────────

  void _startLocationUpdates() {
    _positionSub?.cancel();

    const settings = LocationSettings(
      accuracy: LocationAccuracy.high,
      distanceFilter: 10,
    );

    _positionSub = Geolocator.getPositionStream(locationSettings: settings)
        .listen(
          (pos) => _onPosition(pos),
          onError: (Object err) {
            _debugLog('[LOCATION] stream error: $err');
          },
          cancelOnError: false,
        );

    _debugLog('[LOCATION] GPS stream started');
  }

  void _stopLocationUpdates() {
    _positionSub?.cancel();
    _positionSub = null;
    _debugLog('[LOCATION] GPS stream stopped');
  }

  Future<void> _onPosition(Position pos) async {
    if (!isOnline) return;
    if (_isSendingLocation) return;

    _isSendingLocation = true;
    try {
      await sendLocation(
        lat: pos.latitude,
        lng: pos.longitude,
        heading: pos.heading,
        speed: pos.speed,
        accuracy: pos.accuracy,
      );
    } finally {
      _isSendingLocation = false;
    }
  }

  // ── Load current availability ─────────────────────────────────────────────

  Future<ApiResult<ProviderAvailability>> loadAvailability() async {
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
    _errorMessage = null;
    notifyListeners();

    final result = await api.getAvailability(accessToken: token);
    result.when(
      success: (data) {
        _availability = data;
        _debugLog('[AVAIL] loaded: status=${data.status}');
        if (data.isOnline && _positionSub == null) {
          _startLocationUpdates();
        }
      },
      failure: (error) {
        _errorMessage = error.message;
        _debugLog('[AVAIL] load failed: ${error.message}');
      },
    );
    _isLoading = false;
    notifyListeners();
    return result;
  }

  // ── Go online ─────────────────────────────────────────────────────────────

  Future<ApiResult<ProviderAvailability>> goOnline() async {
    if (_isChangingStatus) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unknown,
          message: 'A status change is already in progress.',
        ),
      );
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

    // Check GPS permission before attempting to go online.
    final permissionError = await _ensureLocationPermission();
    if (permissionError != null) {
      return ApiFailure(
        ApiException(code: ApiErrorCode.unknown, message: permissionError),
      );
    }

    _isChangingStatus = true;
    _errorMessage = null;
    notifyListeners();

    _debugLog('[AVAIL] going online...');
    final result = await api.updateAvailabilityStatus(
      accessToken: token,
      status: 'online',
    );
    result.when(
      success: (data) {
        _availability = data;
        _debugLog('[AVAIL] now online: status=${data.status}');
        _startLocationUpdates();
      },
      failure: (error) {
        _errorMessage = error.message;
        _debugLog(
          '[AVAIL] goOnline failed: ${error.message} code=${error.code}',
        );
      },
    );
    _isChangingStatus = false;
    notifyListeners();
    return result;
  }

  // ── Go offline ────────────────────────────────────────────────────────────

  Future<ApiResult<ProviderAvailability>> goOffline() async {
    if (_isChangingStatus) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unknown,
          message: 'A status change is already in progress.',
        ),
      );
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

    // Stop GPS immediately when going offline regardless of backend result.
    _stopLocationUpdates();

    _isChangingStatus = true;
    _errorMessage = null;
    notifyListeners();

    _debugLog('[AVAIL] going offline...');
    final result = await api.updateAvailabilityStatus(
      accessToken: token,
      status: 'offline',
    );
    result.when(
      success: (data) {
        _availability = data;
        _debugLog('[AVAIL] now offline: status=${data.status}');
      },
      failure: (error) {
        _errorMessage = error.message;
        _debugLog(
          '[AVAIL] goOffline failed: ${error.message} code=${error.code}',
        );
      },
    );
    _isChangingStatus = false;
    notifyListeners();
    return result;
  }

  // ── Current session ───────────────────────────────────────────────────────

  Future<ApiResult<AvailabilitySession>> loadCurrentSession() async {
    final token = _token();
    if (token == null || token.isEmpty) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'No access token available.',
        ),
      );
    }
    final result = await api.getCurrentSession(accessToken: token);
    result.when(
      success: (session) {
        _currentSession = session;
        notifyListeners();
        _debugLog(
          '[AVAIL] session loaded: id=${session.sessionId} status=${session.status}',
        );
      },
      failure: (error) {
        _debugLog('[AVAIL] loadCurrentSession failed: ${error.message}');
      },
    );
    return result;
  }

  // ── Location ──────────────────────────────────────────────────────────────

  Future<ApiResult<ProviderLocation>> sendLocation({
    required double lat,
    required double lng,
    double? heading,
    double? speed,
    double? accuracy,
  }) async {
    if (!isOnline) {
      _debugLog('[LOCATION] Skipping sendLocation — provider is not online.');
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unknown,
          message:
              'Location updates are only sent when the provider is online.',
        ),
      );
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
    final result = await api.updateLocation(
      accessToken: token,
      lat: lat,
      lng: lng,
      heading: heading,
      speed: speed,
      accuracy: accuracy,
    );
    result.when(
      success: (loc) {
        _lastLocation = loc;
        notifyListeners();
      },
      failure: (error) {
        _debugLog('[LOCATION] sendLocation failed: ${error.message}');
      },
    );
    return result;
  }

  Future<ApiResult<ProviderLocation>> loadLocation() async {
    final token = _token();
    if (token == null || token.isEmpty) {
      return ApiFailure(
        const ApiException(
          code: ApiErrorCode.unauthorized,
          message: 'No access token available.',
        ),
      );
    }
    final result = await api.getLocation(accessToken: token);
    result.when(
      success: (loc) {
        _lastLocation = loc;
        notifyListeners();
        _debugLog('[LOCATION] loaded: lat=${loc.lat} lng=${loc.lng}');
      },
      failure: (error) {
        _debugLog('[LOCATION] loadLocation failed: ${error.message}');
      },
    );
    return result;
  }

  @override
  void dispose() {
    _stopLocationUpdates();
    super.dispose();
  }
}
