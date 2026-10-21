import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';
import 'package:geolocator/geolocator.dart';
import 'package:image_picker/image_picker.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../media/data/media_upload_service.dart';
import '../../notifications/data/provider_realtime_listener.dart';
import '../../../core/alerts/request_alert.dart';
import '../../../core/location/online_location_tracker.dart';
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
    this.currentLat,
    this.currentLng,
    this.pickupCredential,
    this.isResendingPickupCredential = false,
    this.justDelivered,
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

  /// The driver's own pickup credential — shown once arrived at pickup,
  /// cleared once the customer verifies and the trip advances past pickup.
  final PickupCredential? pickupCredential;
  final bool isResendingPickupCredential;

  /// The job the driver has just delivered, shown on the delivery-complete
  /// screen until they tap Done.
  final ProviderBooking? justDelivered;

  /// The provider's most recent device location (refreshed by the heartbeat).
  /// Used to show live trip progress between pickup and destination.
  final double? currentLat;
  final double? currentLng;

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
    double? currentLat,
    double? currentLng,
    PickupCredential? pickupCredential,
    bool? isResendingPickupCredential,
    bool clearError = false,
    bool clearActive = false,
    bool clearSelectedTruck = false,
    bool clearPickupCredential = false,
    ProviderBooking? justDelivered,
    bool clearJustDelivered = false,
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
      currentLat: currentLat ?? this.currentLat,
      currentLng: currentLng ?? this.currentLng,
      pickupCredential: (clearActive || clearPickupCredential)
          ? null
          : (pickupCredential ?? this.pickupCredential),
      isResendingPickupCredential: isResendingPickupCredential ?? this.isResendingPickupCredential,
      justDelivered: clearJustDelivered ? null : (justDelivered ?? this.justDelivered),
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
  final _requestAlert = RequestAlert();
  final _alertedRequestIds = <String>{};
  final _location = OnlineLocationTracker(appName: 'Karryam Trucks');
  DateTime? _lastHeartbeatAt;
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
    _syncRequestAlert(next);
    _state = next;
    notifyListeners();
  }

  /// Rings for a request the driver hasn't been alerted about yet, and stops
  /// once nothing is waiting (accepted, declined, expired) or they go offline.
  void _syncRequestAlert(ProviderHomeState next) {
    final waiting = {for (final b in next.pendingRequests) b.id};
    if (waiting.isEmpty || !next.isOnline) {
      _requestAlert.stop();
      return;
    }
    final fresh = waiting.difference(_alertedRequestIds);
    if (fresh.isNotEmpty) {
      _alertedRequestIds.addAll(fresh);
      _requestAlert.start();
    }
  }

  String? get _token => _auth.state.session?.accessToken;

  // ─── Online / Offline ─────────────────────────────────────────────────────

  Future<void> goOnline() async {
    final token = _token;
    if (token == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final pos = await _currentPosition();
      await _withAuth((t) => _api.setOnline(accessToken: t, lat: pos.$1, lng: pos.$2));
      _emit(_state.copyWith(
        isOnline: true,
        isLoading: false,
        currentLat: pos.$1,
        currentLng: pos.$2,
      ));
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
    final streamed = _location.recentFix;
    if (streamed != null) return streamed;
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
      await _withAuth((t) => _api.setOffline(accessToken: t));
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
      final online = await _withAuth((t) => _api.getAvailability(accessToken: t));
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

  /// Reconciles the local online toggle with the server, in both directions.
  ///
  /// Called when the app returns to the foreground. [restoreOnlineStatus] only
  /// handles the offline→online direction (and returns early when already
  /// online), which leaves the dangerous case unhandled: the app was suspended,
  /// the heartbeat stopped, the ~90s Redis TTL lapsed, and the provider is now
  /// out of the matching pool while the UI still shows "Online". They wait for
  /// requests that can never arrive.
  Future<void> syncOnlineStatus() async {
    if (_token == null) return;
    try {
      final online = await _withAuth((t) => _api.getAvailability(accessToken: t));
      if (online == _state.isOnline) {
        // Agreed. Re-arm the heartbeat in case the timer was killed while
        // suspended, otherwise we would drift offline again.
        if (online) _startHeartbeat();
        return;
      }
      if (online) {
        _emit(_state.copyWith(isOnline: true, clearError: true));
        _startHeartbeat();
        _startPolling();
        _startRealtime();
      } else {
        _markDroppedOffline();
      }
    } catch (_) {
      // Network failure: leave the toggle as-is rather than guessing. The next
      // heartbeat (or resume) reconciles.
    }
  }

  // ─── Heartbeat ────────────────────────────────────────────────────────────

  void _startHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = Timer.periodic(
      const Duration(seconds: 30),
      (_) => _sendHeartbeat(),
    );
    // Keeps the app running with the screen off; a fix also covers a beat
    // the timer missed while the OS held it back.
    _location.start(onFix: () {
      final last = _lastHeartbeatAt;
      if (last == null || DateTime.now().difference(last) > const Duration(seconds: 25)) {
        _sendHeartbeat();
      }
    });
  }

  Future<void> _sendHeartbeat() async {
    final token = _token;
    if (token == null) return;
    _lastHeartbeatAt = DateTime.now();
    // `_currentPosition()` never throws (falls back to Lagos), so record the
    // fix for the live trip-progress bar even if the heartbeat POST fails.
    final pos = await _currentPosition();
    _emit(_state.copyWith(currentLat: pos.$1, currentLng: pos.$2));
    try {
      await _withAuth((t) => _api.heartbeat(accessToken: t, lat: pos.$1, lng: pos.$2));
      _missedHeartbeats = 0;
    } catch (_) {
      // The online marker in Redis expires ~90s after the last heartbeat. If
      // beats keep failing (network drop, or the OS throttling our timer while
      // backgrounded) the server has already dropped this provider from the
      // matching pool — keep showing "Online" and they sit there believing they
      // are available while no request can ever reach them.
      _missedHeartbeats++;
      if (_missedHeartbeats >= _maxMissedHeartbeats && _state.isOnline) {
        _markDroppedOffline();
      }
    }
  }

  /// Consecutive failed heartbeats. Reset on every success.
  int _missedHeartbeats = 0;

  /// Two misses (~60s) still sits inside the server's ~90s online TTL, so this
  /// reacts just before the provider actually falls out of the matching pool
  /// rather than after — without flapping on a single transient failure.
  static const _maxMissedHeartbeats = 2;

  /// Drops the UI back to offline after the heartbeat has failed repeatedly, so
  /// the toggle reflects the server rather than a stale local belief.
  ///
  /// An in-progress trip is deliberately left running: going offline only stops
  /// NEW requests, and cargo already on the truck must stay on screen.
  void _markDroppedOffline() {
    _missedHeartbeats = 0;
    _stopHeartbeat();
    final hasActiveTrip = _state.status == ProviderHomeStatus.activeTrip &&
        _state.activeBooking != null;
    if (!hasActiveTrip) {
      _stopPolling();
      _stopRealtime();
    }
    _emit(_state.copyWith(
      isOnline: false,
      error: 'You went offline — reconnect to keep receiving requests.',
    ));
  }

  void _stopHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = null;
    _lastHeartbeatAt = null;
    _location.stop();
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
      final bookings = await _withAuth((t) => _api.listBookings(accessToken: t));

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
        // The pickup code arrives with the "arrived" call. A trip already at
        // the pickup point when the app opened (restart, re-login) never got
        // it, so fetch it rather than showing an empty code card.
        if (active.status == 'arrived_at_pickup' &&
            !active.pickupVerified &&
            _state.pickupCredential == null) {
          unawaited(fetchPickupCredential());
        }
        return;
      }

      // Local state has an active trip but the poll found none — decide from the
      // booking's own status instead of blindly dropping to the dashboard.
      if (_state.status == ProviderHomeStatus.activeTrip && _state.activeBooking != null) {
        var updated = bookings.where((b) => b.id == _state.activeBooking!.id).firstOrNull;
        // The trip dropped out of the list page: ask for it directly rather
        // than keeping a trip on screen that may have been cancelled, which
        // would hide every new request until the app restarts.
        if (updated == null) {
          final id = _state.activeBooking!.id;
          try {
            updated = await _withAuth((t) => _api.getBooking(accessToken: t, bookingId: id));
          } catch (_) {}
          if (generation != _stateGeneration) return;
        }
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
      final updated = await _withAuth((t) => _api.acceptBooking(accessToken: t, bookingId: bookingId));
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
      await _withAuth((t) => _api.rejectBooking(accessToken: t, bookingId: bookingId));
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
      final updated = await _withAuth((t) => _api.markEnRoutePickup(accessToken: t, bookingId: booking.id));
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
      final result = await _withAuth((t) => _api.markArrived(accessToken: t, bookingId: booking.id));
      _emit(_state.copyWith(
        isLoading: false,
        activeBooking: result.booking,
        pickupCredential: result.pickupCredential,
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  /// Refetches the driver's own pickup credential (e.g. after navigating
  /// away from the arrived-at-pickup screen and back).
  Future<void> fetchPickupCredential() async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;
    try {
      final credential = await _withAuth((t) => _api.getPickupCredential(accessToken: t, bookingId: booking.id));
      _emit(_state.copyWith(pickupCredential: credential));
    } catch (_) {
      // Not fatal — the resend button lets the driver retry.
    }
  }

  /// Regenerates the driver's own pickup credential (e.g. it expired before
  /// the customer could verify it).
  Future<void> resendPickupCredential() async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;
    _emit(_state.copyWith(isResendingPickupCredential: true, clearError: true));
    try {
      final credential = await _withAuth((t) => _api.resendPickupCredential(accessToken: t, bookingId: booking.id));
      _emit(_state.copyWith(pickupCredential: credential, isResendingPickupCredential: false));
    } catch (e) {
      _emit(_state.copyWith(isResendingPickupCredential: false, error: _msg(e)));
    }
  }

  Future<void> confirmPickup() async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final updated = await _withAuth((t) => _api.confirmPickup(accessToken: t, bookingId: booking.id));
      _emit(_state.copyWith(isLoading: false, activeBooking: updated, clearPickupCredential: true));
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
      final updated = await _withAuth((t) => _api.markEnRouteDelivery(accessToken: t, bookingId: booking.id));
      _emit(_state.copyWith(isLoading: false, activeBooking: updated));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  /// Confirms delivery, uploading the proof-of-delivery photo and signature to
  /// media-file-service first. Proof uploads are best-effort: a media outage
  /// must never block completing the trip, so failures fall back to confirming
  /// without the URLs.
  Future<void> confirmDelivery({required String code, XFile? proofPhoto, Uint8List? signaturePng}) async {
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
        code: code,
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
        justDelivered: updated,
      ));
      if (_state.isOnline) _startPolling();
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  /// Closes the delivery-complete screen.
  void dismissDeliveryComplete() => _emit(_state.copyWith(clearJustDelivered: true));

  Future<void> cancelActiveTrip() async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      await _withAuth((t) => _api.cancelActiveTrip(accessToken: t, bookingId: booking.id));
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
      final trucks = await _withAuth((t) => _api.listTrucks(accessToken: t));
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

  /// Runs an authenticated request, refreshing the session and retrying once if
  /// the access token has expired.
  ///
  /// Access tokens last an hour but a shift lasts longer, so without this every
  /// call made after the hour mark fails with "Your session has expired" until
  /// the app is restarted — including going online and advancing a live trip.
  /// Only a genuine auth failure triggers the retry; every other error is
  /// rethrown untouched for the caller to surface.
  Future<T> _withAuth<T>(Future<T> Function(String accessToken) call) async {
    final token = _token;
    if (token == null) throw StateError('not authenticated');
    try {
      return await call(token);
    } on ApiException catch (e) {
      if (!e.isAuthFailure) rethrow;
      if (!await _auth.tryRefreshSession()) rethrow;
      final renewed = _token;
      if (renewed == null) rethrow;
      return await call(renewed);
    }
  }

  @override
  void dispose() {
    _requestAlert.stop();
    _stopHeartbeat();
    _stopPolling();
    _stopRealtime();
    super.dispose();
  }
}
