import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:geolocator/geolocator.dart';
import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';

import '../../../core/app_mode.dart';
import '../../offline/state/offline_updates_controller.dart';
import '../../home/mock/dispatch_home_mock.dart';
import '../data/availability_api.dart';
import '../data/availability_models.dart';

class AvailabilityController extends ChangeNotifier {
  final AvailabilityApi api;
  final OfflineUpdatesController? offlineUpdates;
  final String? Function() getAccessToken;

  ProviderAvailability? _availability;
  AvailabilitySession? _currentSession;
  ProviderLocation? _lastLocation;

  bool _isLoading = false;
  bool _isChangingStatus = false;
  bool _isSendingLocation = false;
  String? _errorMessage;

  StreamSubscription<Position>? _positionSub;

  Timer? _heartbeatTimer;
  Position? _lastPosition;

  static const Duration _heartbeatInterval = Duration(seconds: 15);

  int _consecutiveLocationFailures = 0;

  static const int _locationFailureThreshold = 3;

  bool get hasStaleLocation =>
      isOnline && _consecutiveLocationFailures >= _locationFailureThreshold;

  ProviderAvailability? get availability => _availability;
  AvailabilitySession? get currentSession => _currentSession;
  ProviderLocation? get lastLocation => _lastLocation;

  bool get isLoading => _isLoading;
  bool get isChangingStatus => _isChangingStatus;
  String? get errorMessage => _errorMessage;

  bool get isOnline => _availability?.isOnline ?? false;

  AvailabilityController({
    required this.api,
    required this.getAccessToken,
    this.offlineUpdates,
  });

  static void _debugLog(String message) {
    if (kDebugMode) debugPrint(message);
  }

  String? _token() => getAccessToken();

  // ── Location permission ───────────────────────────────────────────────────

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

    unawaited(_seedLastPosition());

    _heartbeatTimer?.cancel();
    _heartbeatTimer = Timer.periodic(_heartbeatInterval, (_) => _heartbeat());

    _debugLog('[LOCATION] GPS stream started');
  }

  Future<void> _seedLastPosition() async {
    if (_lastPosition != null) return;
    try {
      final cached = await Geolocator.getLastKnownPosition();
      if (cached != null) {
        _lastPosition ??= cached;
        _debugLog('[LOCATION] seeded heartbeat from last known position');
        await _heartbeat();
        return;
      }
    } catch (err) {
      _debugLog('[LOCATION] getLastKnownPosition failed: $err');
    }

    try {
      final fresh = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(
          accuracy: LocationAccuracy.medium,
          timeLimit: Duration(seconds: 20),
        ),
      );
      _lastPosition ??= fresh;
      _debugLog('[LOCATION] seeded heartbeat from current position');
      await _heartbeat();
    } catch (err) {
      _debugLog('[LOCATION] getCurrentPosition failed: $err');
    }
  }

  void stopTracking() => _stopLocationUpdates();

  void _stopLocationUpdates() {
    _positionSub?.cancel();
    _positionSub = null;
    _heartbeatTimer?.cancel();
    _heartbeatTimer = null;
    _lastPosition = null;
    _consecutiveLocationFailures = 0;
    _debugLog('[LOCATION] GPS stream stopped');
  }

  Future<void> _heartbeat() async {
    if (!isOnline) return;
    if (_isSendingLocation) return;
    final pos = _lastPosition;
    if (pos == null) return;

    _isSendingLocation = true;
    try {
      await sendLocation(
        lat: pos.latitude,
        lng: pos.longitude,
        heading: pos.heading,
        speed: pos.speed,
        accuracy: pos.accuracy,
        observedAt: pos.timestamp,
      );
    } finally {
      _isSendingLocation = false;
    }
  }

  Future<void> _onPosition(Position pos) async {
    if (!isOnline) return;
    _lastPosition = pos;
    if (_isSendingLocation) return;

    _isSendingLocation = true;
    try {
      await sendLocation(
        lat: pos.latitude,
        lng: pos.longitude,
        heading: pos.heading,
        speed: pos.speed,
        accuracy: pos.accuracy,
        observedAt: pos.timestamp,
      );
    } finally {
      _isSendingLocation = false;
    }
  }

  // ── Load current availability ─────────────────────────────────────────────

  Future<ApiResult<ProviderAvailability>> loadAvailability() async {
    // MOCK: skip the network, return state from DispatchHomeMock.
    if (AppModeConfig.instance.isMock) {
      final avail = ProviderAvailability(
        status: DispatchHomeMock.isOnline ? 'online' : 'offline',
        sessionId: DispatchHomeMock.isOnline ? 'mock-session' : null,
        sessionStart: DispatchHomeMock.isOnline ? DateTime.now() : null,
      );
      _availability = avail;
      notifyListeners();
      return ApiSuccess(avail);
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

    // MOCK: 1.5 s fake latency gives the UI time to show the "Getting you
    // online…" processing state before flipping to online.
    if (AppModeConfig.instance.isMock) {
      _isChangingStatus = true;
      _errorMessage = null;
      notifyListeners();

      await Future.delayed(const Duration(milliseconds: 3200));

      DispatchHomeMock.isOnline = true;
      DispatchHomeMock.seedPendingRequests();

      final avail = ProviderAvailability(
        status: 'online',
        sessionId: 'mock-session-${DateTime.now().millisecondsSinceEpoch}',
        sessionStart: DateTime.now(),
      );
      _availability = avail;
      _isChangingStatus = false;
      notifyListeners();
      return ApiSuccess(avail);
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

    // MOCK: brief pause, clear online state + pending requests.
    if (AppModeConfig.instance.isMock) {
      _isChangingStatus = true;
      _errorMessage = null;
      notifyListeners();

      await Future.delayed(const Duration(milliseconds: 400));

      DispatchHomeMock.isOnline = false;
      DispatchHomeMock.clearPendingRequests();

      final avail = ProviderAvailability(status: 'offline');
      _availability = avail;
      _isChangingStatus = false;
      notifyListeners();
      return ApiSuccess(avail);
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
    DateTime? observedAt,
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

    // MOCK: no-op, just return a synthetic location.
    if (AppModeConfig.instance.isMock) {
      final loc = ProviderLocation(
        lat: lat,
        lng: lng,
        heading: heading,
        speed: speed,
        accuracy: accuracy,
        updatedAt: DateTime.now(),
      );
      _lastLocation = loc;
      return ApiSuccess(loc);
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
        if (_consecutiveLocationFailures > 0) {
          _consecutiveLocationFailures = 0;
          _debugLog('[LOCATION] ping recovered');
        }
        notifyListeners();
      },
      failure: (error) {
        _debugLog('[LOCATION] sendLocation failed: ${error.message}');
        _consecutiveLocationFailures++;
        if (_consecutiveLocationFailures == _locationFailureThreshold) {
          notifyListeners();
        }
      },
    );
    if (offlineUpdates != null) {
      await result.when(
        success: (_) async {
          unawaited(offlineUpdates!.sync());
        },
        failure: (error) async {
          if (observedAt != null &&
              heading != null &&
              speed != null &&
              accuracy != null &&
              (error.code == ApiErrorCode.network ||
                  (error.statusCode ?? 0) >= 500 ||
                  error.statusCode == 429)) {
            await offlineUpdates!.saveLocation(
              latitude: lat,
              longitude: lng,
              heading: heading,
              speed: speed,
              accuracy: accuracy,
              observedAt: observedAt,
            );
          }
        },
      );
    }
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
