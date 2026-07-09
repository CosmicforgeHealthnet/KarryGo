import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';
import 'package:geolocator/geolocator.dart';
import 'package:image_picker/image_picker.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../media/data/media_upload_service.dart';
import '../../notifications/data/provider_realtime_listener.dart';
import '../data/provider_api.dart';

/// Lagos city-center fallback used when the device location is unavailable
/// (permission denied, services off, or a lookup error). Going online must never
/// be blocked by a location failure, so we degrade to this instead.
const _fallbackLat = 6.5244;
const _fallbackLng = 3.3792;

enum ProviderHomeStatus {
  /// Dashboard / Requests / other tabs — no active trip
  dashboard,
  /// Active trip in progress (full-screen overlay)
  activeTrip,
}

class ProviderHomeState {
  const ProviderHomeState({
    required this.status,
    this.isOnline = false,
    this.isLoading = false,
    this.error,
    this.pendingRequests = const [],
    this.activeBooking,
    this.history = const [],
    this.trucks = const [],
    this.trucksLoading = false,
    this.selectedTruckId,
  });

  final ProviderHomeStatus status;
  final bool isOnline;
  final bool isLoading;
  final String? error;
  final List<ProviderBooking> pendingRequests;
  final ProviderBooking? activeBooking;
  final List<ProviderBooking> history;
  final List<ProviderTruck> trucks;
  final bool trucksLoading;
  final String? selectedTruckId;

  ProviderHomeState copyWith({
    ProviderHomeStatus? status,
    bool? isOnline,
    bool? isLoading,
    String? error,
    List<ProviderBooking>? pendingRequests,
    ProviderBooking? activeBooking,
    List<ProviderBooking>? history,
    List<ProviderTruck>? trucks,
    bool? trucksLoading,
    String? selectedTruckId,
    bool clearError = false,
    bool clearActive = false,
    bool clearSelectedTruck = false,
  }) {
    return ProviderHomeState(
      status: status ?? this.status,
      isOnline: isOnline ?? this.isOnline,
      isLoading: isLoading ?? this.isLoading,
      error: clearError ? null : (error ?? this.error),
      pendingRequests: pendingRequests ?? this.pendingRequests,
      activeBooking: clearActive ? null : (activeBooking ?? this.activeBooking),
      history: history ?? this.history,
      trucks: trucks ?? this.trucks,
      trucksLoading: trucksLoading ?? this.trucksLoading,
      selectedTruckId: clearSelectedTruck ? null : (selectedTruckId ?? this.selectedTruckId),
    );
  }
}

class ProviderHomeController extends ChangeNotifier {
  ProviderHomeController({
    required ProviderApi api,
    required ProviderAuthController authController,
    MediaUploadService? mediaUploadService,
    ProviderRealtimeListener Function(String? Function() accessToken, void Function(String eventType) onEvent)? realtimeListenerFactory,
  })  : _api = api,
        _auth = authController,
        _media = mediaUploadService,
        _realtimeFactory = realtimeListenerFactory;

  final ProviderApi _api;
  final ProviderAuthController _auth;

  /// Uploads proof-of-delivery photo/signature to media-file-service. Optional
  /// so tests can omit it; without it delivery confirms without proof URLs.
  final MediaUploadService? _media;

  /// Builds the realtime websocket listener (fast path for incoming bookings).
  /// Optional so tests can omit it; when absent the 4-second poll alone drives
  /// booking updates.
  final ProviderRealtimeListener Function(String? Function(), void Function(String))? _realtimeFactory;
  ProviderRealtimeListener? _realtime;

  Timer? _heartbeatTimer;
  Timer? _pollTimer;

  /// Guards the booking poll: only one request in flight at a time, and any
  /// response fetched before a local mutation (accept/reject/complete/cancel)
  /// is dropped — otherwise a poll that started just before an accept can
  /// re-add the accepted booking to [ProviderHomeState.pendingRequests] and
  /// re-show the incoming-request sheet ("asked to accept again").
  bool _pollInFlight = false;
  int _stateGeneration = 0;

  void _bumpGeneration() => _stateGeneration++;

  ProviderHomeState _state = const ProviderHomeState(status: ProviderHomeStatus.dashboard);
  ProviderHomeState get state => _state;

  void _emit(ProviderHomeState next) {
    _state = next;
    notifyListeners();
  }

  String? get _token => _auth.state.session?.accessToken;

  // ─── Online / Offline ─────────────────────────────────────────────────────

  Future<void> goOnline() async {
    final token = _token;
    if (token == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final pos = await _currentPosition();
      await _api.setOnline(accessToken: token, lat: pos.$1, lng: pos.$2);
      _emit(_state.copyWith(isOnline: true, isLoading: false));
      _startHeartbeat();
      _startPolling();
      _startRealtime();
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  /// Resolves the device's current position for availability + matching. Returns
  /// the Lagos fallback (never throws) so going online is never blocked by a
  /// permission prompt or a disabled location service.
  Future<(double, double)> _currentPosition() async {
    try {
      if (!await Geolocator.isLocationServiceEnabled()) {
        return (_fallbackLat, _fallbackLng);
      }
      var permission = await Geolocator.checkPermission();
      if (permission == LocationPermission.denied) {
        permission = await Geolocator.requestPermission();
      }
      if (permission == LocationPermission.denied ||
          permission == LocationPermission.deniedForever) {
        return (_fallbackLat, _fallbackLng);
      }
      final pos = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(accuracy: LocationAccuracy.high),
      );
      return (pos.latitude, pos.longitude);
    } catch (_) {
      return (_fallbackLat, _fallbackLng);
    }
  }

  Future<void> goOffline() async {
    final token = _token;
    if (token == null) return;

    _bumpGeneration();
    _stopHeartbeat();
    // An in-progress trip must stay on screen and keep updating while offline —
    // going offline only stops availability for NEW requests, it doesn't pause
    // the cargo already on the truck.
    final hasActiveTrip =
        _state.status == ProviderHomeStatus.activeTrip && _state.activeBooking != null;
    if (!hasActiveTrip) {
      _stopPolling();
      _stopRealtime();
    }
    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      await _api.setOffline(accessToken: token);
    } catch (_) {}
    _emit(_state.copyWith(
      isOnline: false,
      isLoading: false,
      status: hasActiveTrip ? ProviderHomeStatus.activeTrip : ProviderHomeStatus.dashboard,
      pendingRequests: const [],
    ));
  }

  void toggleOnline() {
    if (_state.isOnline) {
      goOffline();
    } else {
      goOnline();
    }
  }

  /// Re-syncs the in-memory online state with the backend on app startup.
  ///
  /// The provider's online status is persisted server-side in Redis (with a TTL
  /// refreshed by heartbeats), so a quick refresh/restart should keep them
  /// online. We only restore the UI + background loops here — we deliberately do
  /// NOT re-announce via `setOnline` (the provider is already online server-side
  /// and re-posting would overwrite their location). The immediate heartbeat
  /// refreshes the backend lat/lng + TTL.
  Future<void> restoreOnlineStatus() async {
    final token = _token;
    if (token == null || _state.isOnline) return;
    // Recover an in-progress trip regardless of availability: the trip lives in
    // the database, not Redis, so an expired online TTL (e.g. a longer app
    // restart) must not hide cargo that's still on the truck.
    try {
      await _pollBookings();
      if (_state.status == ProviderHomeStatus.activeTrip && !_state.isOnline) {
        _startPolling();
        _startRealtime();
      }
    } catch (_) {}
    try {
      final online = await _api.getAvailability(accessToken: token);
      if (!online) return;
      _emit(_state.copyWith(isOnline: true));
      unawaited(_sendHeartbeat());
      _startHeartbeat();
      _startPolling();
      _startRealtime();
    } catch (_) {
      // Leave the provider offline in the UI if the check fails; they can toggle
      // manually.
    }
  }

  // ─── Heartbeat ────────────────────────────────────────────────────────────

  void _startHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = Timer.periodic(
      const Duration(seconds: 30),
      (_) => _sendHeartbeat(),
    );
  }

  Future<void> _sendHeartbeat() async {
    final token = _token;
    if (token == null) return;
    try {
      final pos = await _currentPosition();
      await _api.heartbeat(accessToken: token, lat: pos.$1, lng: pos.$2);
    } catch (_) {}
  }

  void _stopHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = null;
  }

  // ─── Realtime (websocket fast path) ─────────────────────────────────────────

  void _startRealtime() {
    if (_realtimeFactory == null) return;
    _realtime ??= _realtimeFactory(() => _token, _onRealtimeEvent);
    _realtime!.start();
  }

  void _stopRealtime() {
    _realtime?.stop();
  }

  // A pushed event means booking state may have changed — poll immediately so
  // the provider sees the incoming request or status change without waiting for
  // the next 4-second tick. The poll remains the fallback if the socket drops.
  void _onRealtimeEvent(String eventType) {
    if (!_state.isOnline) return;
    _pollBookings();
  }

  /// External fast-path poke (e.g. a foreground FCM push): refresh bookings
  /// immediately when online. No-op while offline.
  void pokeRefresh() => _onRealtimeEvent('push');

  // ─── Booking polling ──────────────────────────────────────────────────────

  void _startPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(const Duration(seconds: 4), (_) => _pollBookings());
  }

  void _stopPolling() {
    _pollTimer?.cancel();
    _pollTimer = null;
  }

  Future<void> _pollBookings() async {
    final token = _token;
    if (token == null || _pollInFlight) return;

    _pollInFlight = true;
    final generation = _stateGeneration;
    try {
      final bookings = await _api.listBookings(accessToken: token);

      // A local mutation happened while this response was in flight — the data
      // is stale (e.g. the just-accepted booking still reads awaiting_acceptance).
      if (generation != _stateGeneration) return;

      // Active trip statuses
      const activeStatuses = {'accepted', 'en_route_pickup', 'arrived_at_pickup', 'picked_up', 'en_route_delivery'};

      // Separate into pending and active
      final pending = bookings.where((b) => b.status == 'awaiting_acceptance').toList();
      final active = bookings.where((b) => activeStatuses.contains(b.status)).firstOrNull;

      if (active != null) {
        _emit(_state.copyWith(
          status: ProviderHomeStatus.activeTrip,
          activeBooking: active,
          pendingRequests: pending,
        ));
        return;
      }

      // Local state has an active trip but the poll found none — decide from the
      // booking's own status instead of blindly dropping to the dashboard.
      if (_state.status == ProviderHomeStatus.activeTrip && _state.activeBooking != null) {
        final updated = bookings.where((b) => b.id == _state.activeBooking!.id).firstOrNull;
        if (updated != null && (updated.status == 'delivered' || updated.status == 'completed')) {
          _stopPolling();
          _emit(_state.copyWith(
            status: ProviderHomeStatus.dashboard,
            clearActive: true,
            pendingRequests: pending,
            history: [updated, ..._state.history],
          ));
          if (_state.isOnline) _startPolling();
          return;
        }
        if (updated != null && (updated.status == 'cancelled' || updated.status == 'unmatched')) {
          _emit(_state.copyWith(
            status: ProviderHomeStatus.dashboard,
            clearActive: true,
            pendingRequests: pending,
            error: 'This trip was cancelled by the customer.',
          ));
          // The poll may have been kept alive solely for this trip (provider
          // went offline mid-trip) — stop it once the trip is gone.
          if (!_state.isOnline) _stopPolling();
          return;
        }
        // Booking missing from the page or in an unexpected state — keep the
        // active trip on screen rather than kicking it off on partial data.
        _emit(_state.copyWith(pendingRequests: pending));
        return;
      }

      // Normal dashboard update with pending requests
      _emit(_state.copyWith(
        status: ProviderHomeStatus.dashboard,
        pendingRequests: pending,
      ));
    } catch (_) {
    } finally {
      _pollInFlight = false;
    }
  }

  // ─── Accept / Reject ──────────────────────────────────────────────────────

  /// Returns true when the booking was accepted; false when the accept failed
  /// (e.g. the request expired or was reassigned) — the error is in
  /// [ProviderHomeState.error].
  Future<bool> acceptBooking(String bookingId) async {
    final token = _token;
    if (token == null) return false;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final updated = await _api.acceptBooking(accessToken: token, bookingId: bookingId);
      _bumpGeneration();
      _emit(_state.copyWith(
        status: ProviderHomeStatus.activeTrip,
        isLoading: false,
        activeBooking: updated,
        pendingRequests: _state.pendingRequests.where((b) => b.id != bookingId).toList(),
      ));
      return true;
    } catch (e) {
      // The request may have expired/reassigned while we were accepting — drop
      // it from the sheet instead of leaving a dead card the provider can re-tap.
      _bumpGeneration();
      _emit(_state.copyWith(
        isLoading: false,
        error: _msg(e),
        pendingRequests: _state.pendingRequests.where((b) => b.id != bookingId).toList(),
      ));
      return false;
    }
  }

  Future<bool> rejectBooking(String bookingId) async {
    final token = _token;
    if (token == null) return false;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      await _api.rejectBooking(accessToken: token, bookingId: bookingId);
      _bumpGeneration();
      _emit(_state.copyWith(
        isLoading: false,
        pendingRequests: _state.pendingRequests.where((b) => b.id != bookingId).toList(),
        clearError: true,
      ));
      return true;
    } catch (e) {
      _bumpGeneration();
      _emit(_state.copyWith(
        isLoading: false,
        error: _msg(e),
        pendingRequests: _state.pendingRequests.where((b) => b.id != bookingId).toList(),
      ));
      return false;
    }
  }

  // ─── Trip lifecycle ───────────────────────────────────────────────────────

  Future<void> markEnRoutePickup() async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final updated = await _api.markEnRoutePickup(accessToken: token, bookingId: booking.id);
      _emit(_state.copyWith(isLoading: false, activeBooking: updated));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  Future<void> markArrived() async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final updated = await _api.markArrived(accessToken: token, bookingId: booking.id);
      _emit(_state.copyWith(isLoading: false, activeBooking: updated));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  Future<void> confirmPickup() async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final updated = await _api.confirmPickup(accessToken: token, bookingId: booking.id);
      _emit(_state.copyWith(isLoading: false, activeBooking: updated));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  Future<void> markEnRouteDelivery() async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final updated = await _api.markEnRouteDelivery(accessToken: token, bookingId: booking.id);
      _emit(_state.copyWith(isLoading: false, activeBooking: updated));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  /// Confirms delivery, uploading the proof-of-delivery photo and signature to
  /// media-file-service first. Proof uploads are best-effort: a media outage
  /// must never block completing the trip, so failures fall back to confirming
  /// without the URLs.
  Future<void> confirmDelivery({XFile? proofPhoto, Uint8List? signaturePng}) async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));

    var proofUrl = '';
    var signatureUrl = '';
    final media = _media;
    final providerId = _auth.state.session?.provider.id ?? '';
    if (media != null && providerId.isNotEmpty) {
      if (proofPhoto != null) {
        try {
          final result = await media.uploadPicked(
            ownerId: providerId,
            purpose: MediaPurpose.proofImage,
            file: proofPhoto,
          );
          proofUrl = result.url;
        } catch (e) {
          if (kDebugMode) debugPrint('proof photo upload failed: $e');
        }
      }
      if (signaturePng != null && signaturePng.isNotEmpty) {
        try {
          final result = await media.uploadBytes(
            ownerId: providerId,
            purpose: MediaPurpose.signature,
            filename: 'signature-${booking.id}.png',
            bytes: signaturePng,
          );
          signatureUrl = result.url;
        } catch (e) {
          if (kDebugMode) debugPrint('signature upload failed: $e');
        }
      }
    }

    try {
      final updated = await _api.confirmDelivery(
        accessToken: token,
        bookingId: booking.id,
        proofPhotoUrl: proofUrl,
        signatureUrl: signatureUrl,
      );
      _bumpGeneration();
      _stopPolling();
      _emit(_state.copyWith(
        status: ProviderHomeStatus.dashboard,
        isLoading: false,
        clearActive: true,
        history: [updated, ..._state.history],
      ));
      if (_state.isOnline) _startPolling();
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  Future<void> cancelActiveTrip() async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      await _api.cancelActiveTrip(accessToken: token, bookingId: booking.id);
      _bumpGeneration();
      _stopPolling();
      _emit(_state.copyWith(
        status: ProviderHomeStatus.dashboard,
        isLoading: false,
        clearActive: true,
        pendingRequests: const [],
      ));
      if (_state.isOnline) _startPolling();
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  // ─── Trucks ───────────────────────────────────────────────────────────────

  Future<void> loadTrucks() async {
    final token = _token;
    if (token == null) return;

    _emit(_state.copyWith(trucksLoading: true));
    try {
      final trucks = await _api.listTrucks(accessToken: token);
      _emit(_state.copyWith(trucks: trucks, trucksLoading: false));
    } catch (_) {
      _emit(_state.copyWith(trucksLoading: false));
    }
  }

  void setSelectedTruck(String? truckId) {
    _emit(_state.copyWith(
      selectedTruckId: truckId,
      clearSelectedTruck: truckId == null,
    ));
  }

  String _msg(Object e) => e is ApiException ? e.message : e.toString();

  @override
  void dispose() {
    _stopHeartbeat();
    _stopPolling();
    _stopRealtime();
    super.dispose();
  }
}
