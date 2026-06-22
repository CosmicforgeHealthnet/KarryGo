import 'dart:async';

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:flutter/foundation.dart';

import '../../auth/models/provider_auth_models.dart';
import '../../auth/state/provider_auth_controller.dart';
import '../../notifications/data/provider_realtime_listener.dart';
import '../data/provider_api.dart';

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
    ProviderRealtimeListener Function(String? Function() accessToken, void Function(String eventType) onEvent)? realtimeListenerFactory,
  })  : _api = api,
        _auth = authController,
        _realtimeFactory = realtimeListenerFactory;

  final ProviderApi _api;
  final ProviderAuthController _auth;

  /// Builds the realtime websocket listener (fast path for incoming bookings).
  /// Optional so tests can omit it; when absent the 4-second poll alone drives
  /// booking updates.
  final ProviderRealtimeListener Function(String? Function(), void Function(String))? _realtimeFactory;
  ProviderRealtimeListener? _realtime;

  Timer? _heartbeatTimer;
  Timer? _pollTimer;

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
      await _api.setOnline(accessToken: token, lat: 6.5244, lng: 3.3792);
      _emit(_state.copyWith(isOnline: true, isLoading: false));
      _startHeartbeat();
      _startPolling();
      _startRealtime();
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  Future<void> goOffline() async {
    final token = _token;
    if (token == null) return;

    _stopHeartbeat();
    _stopPolling();
    _stopRealtime();
    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      await _api.setOffline(accessToken: token);
    } catch (_) {}
    _emit(_state.copyWith(
      isOnline: false,
      isLoading: false,
      status: ProviderHomeStatus.dashboard,
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

  // ─── Heartbeat ────────────────────────────────────────────────────────────

  void _startHeartbeat() {
    _heartbeatTimer?.cancel();
    _heartbeatTimer = Timer.periodic(const Duration(seconds: 30), (_) async {
      final token = _token;
      if (token == null) return;
      try {
        await _api.heartbeat(accessToken: token, lat: 6.5244, lng: 3.3792);
      } catch (_) {}
    });
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
    if (token == null) return;

    try {
      final bookings = await _api.listBookings(accessToken: token);

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

      // Check if current active trip just completed
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
      }

      // Normal dashboard update with pending requests
      _emit(_state.copyWith(
        status: ProviderHomeStatus.dashboard,
        pendingRequests: pending,
      ));
    } catch (_) {}
  }

  // ─── Accept / Reject ──────────────────────────────────────────────────────

  Future<void> acceptBooking(String bookingId) async {
    final token = _token;
    if (token == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final updated = await _api.acceptBooking(accessToken: token, bookingId: bookingId);
      _emit(_state.copyWith(
        status: ProviderHomeStatus.activeTrip,
        isLoading: false,
        activeBooking: updated,
        pendingRequests: _state.pendingRequests.where((b) => b.id != bookingId).toList(),
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  Future<void> rejectBooking(String bookingId) async {
    final token = _token;
    if (token == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      await _api.rejectBooking(accessToken: token, bookingId: bookingId);
      _emit(_state.copyWith(
        isLoading: false,
        pendingRequests: _state.pendingRequests.where((b) => b.id != bookingId).toList(),
        clearError: true,
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _msg(e)));
    }
  }

  // ─── Trip lifecycle ───────────────────────────────────────────────────────

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

  Future<void> confirmDelivery() async {
    final token = _token;
    final booking = _state.activeBooking;
    if (token == null || booking == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final updated = await _api.confirmDelivery(accessToken: token, bookingId: booking.id);
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
