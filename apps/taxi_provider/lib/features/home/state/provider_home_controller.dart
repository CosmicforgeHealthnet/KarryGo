import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';
import 'package:geolocator/geolocator.dart';
import 'package:permission_handler/permission_handler.dart';

import '../../auth/data/provider_auth_api.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../data/provider_api.dart';
import '../models/provider_booking.dart';

@immutable
class ProviderHomeState {
  const ProviderHomeState({
    this.online = false,
    this.isTogglingOnline = false,
    this.offers = const [],
    this.activeBooking,
    this.isActionLoading = false,
    this.error,
    this.notice,
  });

  final bool online;
  final bool isTogglingOnline;

  /// Offers awaiting an answer. The service offers one at a time today, but
  /// this is a list so a future multi-offer server needs no client change.
  final List<ProviderBooking> offers;

  /// The committed ride. Takes over the screen while set.
  final ProviderBooking? activeBooking;

  final bool isActionLoading;
  final String? error;

  /// A transient, non-error message (e.g. "that offer expired").
  final String? notice;

  ProviderHomeState copyWith({
    bool? online,
    bool? isTogglingOnline,
    List<ProviderBooking>? offers,
    ProviderBooking? activeBooking,
    bool clearActiveBooking = false,
    bool? isActionLoading,
    String? error,
    bool clearError = false,
    String? notice,
    bool clearNotice = false,
  }) {
    return ProviderHomeState(
      online: online ?? this.online,
      isTogglingOnline: isTogglingOnline ?? this.isTogglingOnline,
      offers: offers ?? this.offers,
      activeBooking:
          clearActiveBooking ? null : (activeBooking ?? this.activeBooking),
      isActionLoading: isActionLoading ?? this.isActionLoading,
      error: clearError ? null : (error ?? this.error),
      notice: clearNotice ? null : (notice ?? this.notice),
    );
  }
}

/// Drives the driver's whole working session: going online, receiving offers,
/// and walking one ride from accept to complete.
class ProviderHomeController extends ChangeNotifier {
  ProviderHomeController({
    required ProviderApi api,
    required ProviderAuthApi authApi,
    required ProviderAuthController authController,
  }) : _api = api,
       _authApi = authApi,
       _auth = authController;

  final ProviderApi _api;
  final ProviderAuthApi _authApi;
  final ProviderAuthController _auth;

  /// Lagos city centre. Only ever used when the device refuses a fix — going
  /// online must not be blocked by a flaky GPS, and the next heartbeat
  /// corrects the position anyway.
  static const _fallbackLat = 6.5244;
  static const _fallbackLng = 3.3792;

  Timer? _pollTimer;
  Timer? _heartbeatTimer;

  ProviderHomeState _state = const ProviderHomeState();
  ProviderHomeState get state => _state;

  /// Guards the poll: one request in flight at a time, and any response
  /// fetched before a local mutation (accept/reject/complete) is dropped —
  /// otherwise a poll started just before an accept lands after it and
  /// re-shows the offer the driver already answered.
  bool _pollInFlight = false;
  int _generation = 0;

  void _bump() => _generation++;

  void _emit(ProviderHomeState next) {
    _state = next;
    notifyListeners();
  }

  String? _token() => _auth.state.accessToken;

  // ─── Location ──────────────────────────────────────────────────────────────

  /// Best-effort device position. Never throws and never blocks: a driver
  /// with location off still gets to go online, just pinned to the fallback
  /// until a real fix arrives.
  Future<({double lat, double lng, double heading, double speed, double accuracy})>
  _position() async {
    try {
      var permission = await Permission.locationWhenInUse.status;
      if (!permission.isGranted && !permission.isLimited) {
        permission = await Permission.locationWhenInUse.request();
      }
      if (!permission.isGranted && !permission.isLimited) {
        return (
          lat: _fallbackLat,
          lng: _fallbackLng,
          heading: 0.0,
          speed: 0.0,
          accuracy: 0.0,
        );
      }
      if (!await Geolocator.isLocationServiceEnabled()) {
        return (
          lat: _fallbackLat,
          lng: _fallbackLng,
          heading: 0.0,
          speed: 0.0,
          accuracy: 0.0,
        );
      }
      final pos = await Geolocator.getCurrentPosition(
        locationSettings: const LocationSettings(
          accuracy: LocationAccuracy.high,
        ),
      );
      return (
        lat: pos.latitude,
        lng: pos.longitude,
        heading: pos.heading,
        speed: pos.speed,
        accuracy: pos.accuracy,
      );
    } catch (_) {
      return (
        lat: _fallbackLat,
        lng: _fallbackLng,
        heading: 0.0,
        speed: 0.0,
        accuracy: 0.0,
      );
    }
  }

  // ─── Session start ─────────────────────────────────────────────────────────

  /// Called when the home screen opens. Restores whether the driver was left
  /// online and re-adopts any ride still in progress, so force-quitting the
  /// app mid-trip does not strand a passenger.
  Future<void> initialize() async {
    final token = _token();
    if (token == null) return;
    try {
      final availability = await _api.getAvailability(accessToken: token);
      _emit(_state.copyWith(online: availability.online));
      if (availability.online) _startHeartbeat();
    } on ApiException {
      // Non-fatal: the toggle simply shows offline until the driver taps it.
    }
    await refreshBookings();
    _startPolling();
  }

  // ─── Online / offline ──────────────────────────────────────────────────────

  Future<void> setOnline(bool online) async {
    final token = _token();
    if (token == null) return;

    _emit(_state.copyWith(
      isTogglingOnline: true,
      clearError: true,
      clearNotice: true,
    ));
    try {
      final pos = await _position();
      final next = await _api.setAvailability(
        accessToken: token,
        online: online,
        lat: pos.lat,
        lng: pos.lng,
        heading: pos.heading,
        speed: pos.speed,
        accuracy: pos.accuracy,
      );
      _emit(_state.copyWith(online: next.online, isTogglingOnline: false));
      if (next.online) {
        _startHeartbeat();
        _startPolling();
      } else {
        _stopHeartbeat();
      }
    } on ApiException catch (e) {
      // Going online is gated on compliance and an active vehicle; the
      // service's own message names the specific reason, so surface it
      // verbatim rather than a generic failure.
      _emit(_state.copyWith(isTogglingOnline: false, error: e.message));
    }
  }

  void _startHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = Timer.periodic(
      const Duration(seconds: 25),
      (_) => _sendHeartbeat(),
    );
    _sendHeartbeat();
  }

  void _stopHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = null;
  }

  Future<void> _sendHeartbeat() async {
    final token = _token();
    if (token == null) return;
    try {
      final pos = await _position();
      await _api.heartbeat(
        accessToken: token,
        lat: pos.lat,
        lng: pos.lng,
        heading: pos.heading,
        speed: pos.speed,
        accuracy: pos.accuracy,
        onTrip: _state.activeBooking != null,
      );
    } on ApiException {
      // A missed beat is recoverable: the online TTL outlives one interval.
    }
  }

  // ─── Offers + active ride ──────────────────────────────────────────────────

  void _startPolling() {
    _pollTimer?.cancel();
    _pollTimer = Timer.periodic(
      const Duration(seconds: 4),
      (_) => refreshBookings(),
    );
  }

  void _stopPolling() {
    _pollTimer?.cancel();
    _pollTimer = null;
  }

  Future<void> refreshBookings() async {
    if (_pollInFlight) return;
    final token = _token();
    if (token == null) return;

    _pollInFlight = true;
    final generation = _generation;
    try {
      final bookings = await _api.listActiveBookings(accessToken: token);
      if (generation != _generation) return;

      final offers = bookings.where((b) => b.status.isOffer).toList();
      ProviderBooking? active;
      for (final b in bookings) {
        if (b.status.isActive) {
          active = b;
          break;
        }
      }

      // A ride that vanished from the list while we held it locally was
      // ended by someone else (customer cancel, or an operator). Say so
      // rather than silently dropping the driver back to the dashboard.
      final hadActive = _state.activeBooking;
      final endedElsewhere = hadActive != null && active == null;

      _emit(_state.copyWith(
        offers: offers,
        activeBooking: active,
        clearActiveBooking: active == null,
        notice: endedElsewhere ? 'That trip was ended by the customer.' : null,
        clearNotice: !endedElsewhere,
      ));
    } on ApiException {
      // Transient: the next tick retries rather than tearing down the screen.
    } finally {
      _pollInFlight = false;
    }
  }

  Future<bool> acceptOffer(String bookingId) =>
      _act(() => _api.accept(accessToken: _token()!, bookingId: bookingId));

  Future<bool> rejectOffer(String bookingId) =>
      _act(() => _api.reject(accessToken: _token()!, bookingId: bookingId));

  Future<bool> markEnRoutePickup() => _actOnActive(
    (token, id) => _api.markEnRoutePickup(accessToken: token, bookingId: id),
  );

  Future<bool> markArrived() => _actOnActive(
    (token, id) => _api.markArrived(accessToken: token, bookingId: id),
  );

  Future<bool> verifyPickup(String code) => _actOnActive(
    (token, id) =>
        _api.verifyPickup(accessToken: token, bookingId: id, code: code),
  );

  Future<bool> verifyDelivery(String code) => _actOnActive(
    (token, id) =>
        _api.verifyDelivery(accessToken: token, bookingId: id, code: code),
  );

  Future<bool> startTrip() => _actOnActive(
    (token, id) => _api.startTrip(accessToken: token, bookingId: id),
  );

  Future<bool> cancelTrip({String reason = 'driver_cancelled'}) => _actOnActive(
    (token, id) => _api.cancel(accessToken: token, bookingId: id, reason: reason),
  );

  Future<bool> completeTrip() {
    final booking = _state.activeBooking;
    if (booking == null) return Future.value(false);
    return _actOnActive(
      (token, id) => _api.completeTrip(
        accessToken: token,
        bookingId: id,
        // The service recomputes the final fare from these; sending the
        // quoted values keeps a dev run honest without inventing telemetry
        // this build does not yet collect per trip.
        actualDistanceKm: booking.distanceKm ?? 0,
        actualDurationMin: booking.durationMinutes ?? 0,
      ),
    );
  }

  Future<bool> _actOnActive(
    Future<ProviderBooking> Function(String token, String bookingId) action,
  ) {
    final booking = _state.activeBooking;
    if (booking == null) return Future.value(false);
    return _act(() => action(_token()!, booking.id));
  }

  /// Runs one lifecycle call, folding the result back into state. Returns
  /// whether it succeeded so a screen can decide whether to navigate.
  Future<bool> _act(Future<ProviderBooking> Function() action) async {
    if (_token() == null) {
      await _auth.handleAuthFailure();
      return false;
    }
    _emit(_state.copyWith(
      isActionLoading: true,
      clearError: true,
      clearNotice: true,
    ));
    try {
      final booking = await action();
      _bump();
      _applyBooking(booking);
      return true;
    } on ApiException catch (e) {
      _bump();
      _emit(_state.copyWith(isActionLoading: false, error: e.message));
      // An offer that expired or was reassigned is gone: drop it locally so
      // the driver is not left tapping a dead card. A genuinely live one
      // reappears on the next poll.
      unawaited(refreshBookings());
      return false;
    }
  }

  void _applyBooking(ProviderBooking booking) {
    if (booking.status.isTerminal) {
      _emit(_state.copyWith(
        isActionLoading: false,
        clearActiveBooking: true,
        offers: _state.offers.where((o) => o.id != booking.id).toList(),
      ));
      return;
    }
    _emit(_state.copyWith(
      isActionLoading: false,
      activeBooking: booking.status.isActive ? booking : null,
      clearActiveBooking: !booking.status.isActive,
      offers: _state.offers.where((o) => o.id != booking.id).toList(),
    ));
  }

  // ─── Parcel opt-in ─────────────────────────────────────────────────────────

  /// Toggles whether this driver may be matched to package deliveries.
  Future<void> setAcceptsParcels(bool accepts) async {
    final token = _token();
    if (token == null) return;
    try {
      final provider = await _authApi.setParcelPreference(
        accessToken: token,
        acceptsParcels: accepts,
      );
      _auth.applyProviderUpdate(provider);
    } on ApiException catch (e) {
      _emit(_state.copyWith(error: e.message));
    }
  }

  void clearError() => _emit(_state.copyWith(clearError: true));
  void clearNotice() => _emit(_state.copyWith(clearNotice: true));

  /// Stops all timers — called on sign-out so a stale session cannot keep
  /// heartbeating the driver online.
  void stop() {
    _bump();
    _stopPolling();
    _stopHeartbeat();
    _emit(const ProviderHomeState());
  }

  @override
  void dispose() {
    _stopPolling();
    _stopHeartbeat();
    super.dispose();
  }
}
