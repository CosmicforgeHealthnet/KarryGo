import 'dart:async';
import 'dart:math' as math;

import 'package:cosmicforge_logistics_api_core/cosmicforge_logistics_api_core.dart';
import 'package:cosmicforge_logistics_live_tracking/cosmicforge_logistics_live_tracking.dart';
import 'package:flutter/foundation.dart';

import '../../auth/models/customer_auth_models.dart';
import '../../auth/state/customer_auth_controller.dart';
import '../../hauling/data/customer_realtime_listener.dart';
import '../../wallet/data/wallet_api.dart';
import '../data/dispatch_api.dart';
import '../models/dispatch_models.dart';

// ─── Flow status ──────────────────────────────────────────────────────────────

enum DispatchFlowStatus {
  idle,
  locationEntry,
  checkingAvailability,
  unavailable,
  packageInfo,
  payment,
  paymentProcessing,
  searching,
  activeTrip,
  review,
  completed,
  cancelled,
  error,
}

// ─── State ────────────────────────────────────────────────────────────────────

class DispatchBookingState {
  const DispatchBookingState({
    required this.status,
    this.isLoading = false,
    this.error,
    this.availability,
    // trip progress (0..1 along pickup→destination while in transit)
    this.tripProgress = 0.0,
    this.tripStartedAt,
    // location inputs
    this.pickupAddress = '',
    this.pickupLat = 0.0,
    this.pickupLng = 0.0,
    this.dropoffAddress = '',
    this.dropoffLat = 0.0,
    this.dropoffLng = 0.0,
    // package info
    this.packageDesc = '',
    this.packageType = '',
    this.packageWeight = 0,
    this.packageSize,
    this.isFragile = false,
    this.serviceTier = ServiceTierOption.standard,
    this.receiverName = '',
    this.receiverPhone = '',
    // review
    this.reviewRating = 0,
    this.reviewText = '',
    this.recommendsRider,
    // payment
    this.paymentMethod,
    this.walletBalanceKobo,
    // fare + booking
    this.fareEstimate,
    this.activeBooking,
    this.bookingHistory = const [],
    // rider snapshot (loaded when booking is accepted)
    this.providerSnapshot,
    // live rider location (map marker during the delivery)
    this.providerLocation,
    // handover verification (pickup: customer verifies rider; delivery:
    // rider verifies customer)
    this.deliveryVerification,
    this.pickupVerifiedLocally = false,
    this.isResendingCode = false,
  });

  const DispatchBookingState.idle() : this(status: DispatchFlowStatus.idle);

  final DispatchFlowStatus status;
  final bool isLoading;
  final String? error;
  final DispatchAvailabilityResult? availability;

  final double tripProgress;
  final DateTime? tripStartedAt;

  final String pickupAddress;
  final double pickupLat;
  final double pickupLng;
  final String dropoffAddress;
  final double dropoffLat;
  final double dropoffLng;

  final String packageDesc;
  final String packageType;
  final double packageWeight;
  final PackageSizeOption? packageSize;
  final bool isFragile;
  final ServiceTierOption serviceTier;
  final String receiverName;
  final String receiverPhone;

  final int reviewRating;
  final String reviewText;
  final bool? recommendsRider;

  final String? paymentMethod;
  final int? walletBalanceKobo;

  final DispatchFareEstimate? fareEstimate;
  final DispatchBooking? activeBooking;
  final List<DispatchBooking> bookingHistory;

  final ProviderSnapshot? providerSnapshot;
  final ProviderLocation? providerLocation;

  final VerificationCodeResult? deliveryVerification;
  final bool pickupVerifiedLocally;
  final bool isResendingCode;

  double get walletBalanceNaira => (walletBalanceKobo ?? 0) / 100;
  bool get locationsReady =>
      pickupAddress.isNotEmpty && dropoffAddress.isNotEmpty;
  bool get packageInfoReady =>
      packageDesc.trim().isNotEmpty &&
      packageSize != null &&
      receiverName.trim().isNotEmpty &&
      receiverPhone.trim().isNotEmpty;

  DispatchBookingState copyWith({
    DispatchFlowStatus? status,
    bool? isLoading,
    String? error,
    DispatchAvailabilityResult? availability,
    double? tripProgress,
    DateTime? tripStartedAt,
    bool clearTripProgress = false,
    String? pickupAddress,
    double? pickupLat,
    double? pickupLng,
    String? dropoffAddress,
    double? dropoffLat,
    double? dropoffLng,
    String? packageDesc,
    String? packageType,
    double? packageWeight,
    PackageSizeOption? packageSize,
    bool? isFragile,
    ServiceTierOption? serviceTier,
    String? receiverName,
    String? receiverPhone,
    int? reviewRating,
    String? reviewText,
    bool? recommendsRider,
    String? paymentMethod,
    int? walletBalanceKobo,
    DispatchFareEstimate? fareEstimate,
    DispatchBooking? activeBooking,
    List<DispatchBooking>? bookingHistory,
    ProviderSnapshot? providerSnapshot,
    ProviderLocation? providerLocation,
    VerificationCodeResult? deliveryVerification,
    bool? pickupVerifiedLocally,
    bool? isResendingCode,
    bool clearError = false,
    bool clearFare = false,
    bool clearBooking = false,
    bool clearProviderSnapshot = false,
    bool clearRecommendsRider = false,
  }) {
    return DispatchBookingState(
      status: status ?? this.status,
      isLoading: isLoading ?? this.isLoading,
      error: clearError ? null : (error ?? this.error),
      availability: availability ?? this.availability,
      tripProgress:
          clearTripProgress ? 0.0 : (tripProgress ?? this.tripProgress),
      tripStartedAt:
          clearTripProgress ? null : (tripStartedAt ?? this.tripStartedAt),
      pickupAddress: pickupAddress ?? this.pickupAddress,
      pickupLat: pickupLat ?? this.pickupLat,
      pickupLng: pickupLng ?? this.pickupLng,
      dropoffAddress: dropoffAddress ?? this.dropoffAddress,
      dropoffLat: dropoffLat ?? this.dropoffLat,
      dropoffLng: dropoffLng ?? this.dropoffLng,
      packageDesc: packageDesc ?? this.packageDesc,
      packageType: packageType ?? this.packageType,
      packageWeight: packageWeight ?? this.packageWeight,
      packageSize: packageSize ?? this.packageSize,
      isFragile: isFragile ?? this.isFragile,
      serviceTier: serviceTier ?? this.serviceTier,
      receiverName: receiverName ?? this.receiverName,
      receiverPhone: receiverPhone ?? this.receiverPhone,
      reviewRating: reviewRating ?? this.reviewRating,
      reviewText: reviewText ?? this.reviewText,
      recommendsRider: clearRecommendsRider
          ? null
          : (recommendsRider ?? this.recommendsRider),
      paymentMethod: paymentMethod ?? this.paymentMethod,
      walletBalanceKobo: walletBalanceKobo ?? this.walletBalanceKobo,
      fareEstimate: clearFare ? null : (fareEstimate ?? this.fareEstimate),
      activeBooking: clearBooking ? null : (activeBooking ?? this.activeBooking),
      bookingHistory: bookingHistory ?? this.bookingHistory,
      providerSnapshot: clearProviderSnapshot
          ? null
          : (providerSnapshot ?? this.providerSnapshot),
      providerLocation: clearProviderSnapshot
          ? null
          : (providerLocation ?? this.providerLocation),
      deliveryVerification: deliveryVerification ?? this.deliveryVerification,
      pickupVerifiedLocally: pickupVerifiedLocally ?? this.pickupVerifiedLocally,
      isResendingCode: isResendingCode ?? this.isResendingCode,
    );
  }
}

// ─── Controller ───────────────────────────────────────────────────────────────

class DispatchBookingController extends ChangeNotifier {
  DispatchBookingController({
    required DispatchApi api,
    required CustomerAuthController authController,
    required WalletApi walletApi,
    CustomerRealtimeListener Function(
      String? Function() accessToken,
      void Function(String eventType) onEvent,
    )? realtimeListenerFactory,
    this.savedLocationsLoader,
  })  : _api = api,
        _auth = authController,
        _walletApi = walletApi,
        _realtimeFactory = realtimeListenerFactory;

  final DispatchApi _api;
  final CustomerAuthController _auth;
  final WalletApi _walletApi;

  /// Fetches the customer's saved locations so the location entry view can
  /// offer them as one-tap fills. Optional — the view hides the section when
  /// unset (tests/headless configs) or when the fetch fails.
  final Future<List<SavedLocation>> Function()? savedLocationsLoader;

  /// Exposed so views (e.g. the Trips list) can lazily fetch rider snapshots
  /// for display without duplicating the HTTP client.
  DispatchApi get api => _api;

  /// Builds the realtime websocket listener (fast path for booking updates).
  /// Optional so tests/headless configs can omit it; polling is the fallback.
  final CustomerRealtimeListener Function(
    String? Function(),
    void Function(String),
  )? _realtimeFactory;
  CustomerRealtimeListener? _realtime;

  Timer? _pollTimer;
  Timer? _searchTimeoutTimer;
  Timer? _locationTimer;
  WebSocketTrackSource? _locationSocket;

  DispatchBookingState _state = const DispatchBookingState.idle();
  DispatchBookingState get state => _state;

  /// Guards the booking/location polls: one request in flight at a time, and a
  /// response fetched before a local flow mutation (cancel/reset/review/open)
  /// is dropped — otherwise a poll that started just before a cancel can land
  /// after it and revive the searching/active screen with stale data.
  bool _pollInFlight = false;
  int _stateGeneration = 0;

  /// Guards [restoreActiveBookingIfAny] so it only ever runs once per app
  /// session — it's called from a widget rebuild (post session-restore), not a
  /// one-shot lifecycle hook, so it needs its own idempotency flag.
  bool _restoreAttempted = false;

  void _bumpGeneration() => _stateGeneration++;

  void _emit(DispatchBookingState next) {
    _state = next;
    notifyListeners();
  }

  String? _accessToken() => _auth.state.session?.accessToken;

  // ─── Flow entry ────────────────────────────────────────────────────────────

  void startDispatchFlow() {
    _emit(_state.copyWith(
      status: DispatchFlowStatus.locationEntry,
      clearError: true,
    ));
  }

  /// Re-opens an in-progress booking (active or still searching) into the live
  /// flow and resumes polling.
  void openActiveBooking(DispatchBooking booking) {
    _bumpGeneration();
    _emit(_state.copyWith(
      activeBooking: booking,
      clearError: true,
      clearProviderSnapshot: true,
    ));
    _startPolling();
    _applyBookingUpdate(booking);
  }

  /// Resumes a delivery left in progress when the app was closed. See
  /// [HaulingBookingController.restoreActiveBookingIfAny] for context — same
  /// gap, same fix, mirrored here for bike deliveries. `delivered` is included
  /// because that status still has a pending review step in this flow.
  Future<void> restoreActiveBookingIfAny() async {
    if (_restoreAttempted) return;
    _restoreAttempted = true;
    if (_state.status != DispatchFlowStatus.idle) return;
    await loadHistory();
    if (_state.status != DispatchFlowStatus.idle) return;
    DispatchBooking? match;
    for (final b in _state.bookingHistory) {
      if (b.status.isActive ||
          b.status.isSearching ||
          b.status == DispatchBookingStatus.delivered) {
        match = b;
        break;
      }
    }
    if (match != null) openActiveBooking(match);
  }

  /// Prefills the booking form from a previous delivery and starts the flow at
  /// location entry.
  void rebookFrom(DispatchBooking booking) {
    reset();
    setPickupLocation(
        booking.pickupAddress, booking.pickupLat, booking.pickupLng);
    if (booking.dropoffAddress.isNotEmpty) {
      setDropoffLocation(
          booking.dropoffAddress, booking.dropoffLat, booking.dropoffLng);
    }
    if (booking.packageDesc.isNotEmpty) setPackageDesc(booking.packageDesc);
    if (booking.packageType.isNotEmpty) setPackageType(booking.packageType);
    final size = PackageSizeOption.fromName(booking.packageSize);
    if (size != null) setPackageSize(size);
    final tier = ServiceTierOption.fromName(booking.serviceTier);
    if (tier != null) setServiceTier(tier);
    setIsFragile(booking.isFragile);
    setReceiverName(booking.receiverName);
    setReceiverPhone(booking.receiverPhone);
    startDispatchFlow();
  }

  // ─── Location entry ────────────────────────────────────────────────────────

  void setPickupLocation(String address, double lat, double lng) {
    _emit(_state.copyWith(
      pickupAddress: address,
      pickupLat: lat,
      pickupLng: lng,
      clearFare: true,
    ));
  }

  void setDropoffLocation(String address, double lat, double lng) {
    _emit(_state.copyWith(
      dropoffAddress: address,
      dropoffLat: lat,
      dropoffLng: lng,
      clearFare: true,
    ));
  }

  Future<void> checkAvailabilityAndProceed() async {
    final token = _accessToken();
    if (token == null) {
      // Expired/missing session: route to sign-in instead of dead-ending the
      // flow (mirrors the hauling controller).
      await _auth.handleAuthFailure();
      return;
    }

    _emit(_state.copyWith(
      status: DispatchFlowStatus.checkingAvailability,
      isLoading: true,
      clearError: true,
    ));

    try {
      final result = await _api.checkAvailability(
        accessToken: token,
        lat: _state.pickupLat,
        lng: _state.pickupLng,
      );
      if (result.available) {
        _emit(_state.copyWith(
          status: DispatchFlowStatus.packageInfo,
          isLoading: false,
          availability: result,
        ));
        _fetchPreviewFare();
      } else {
        _emit(_state.copyWith(
          status: DispatchFlowStatus.unavailable,
          isLoading: false,
          availability: result,
        ));
      }
    } catch (e) {
      _emit(_state.copyWith(
        status: DispatchFlowStatus.error,
        isLoading: false,
        error: _errorMessage(e),
      ));
    }
  }

  void _fetchPreviewFare() {
    if (_state.pickupLat == 0 && _state.pickupLng == 0) return;
    _api
        .estimateFare(
          pickupLat: _state.pickupLat,
          pickupLng: _state.pickupLng,
          dropoffLat: _state.dropoffLat,
          dropoffLng: _state.dropoffLng,
          packageSize: _state.packageSize?.name ?? 'small',
          serviceTier: _state.serviceTier.name,
        )
        .then((estimate) {
          _emit(_state.copyWith(fareEstimate: estimate));
        })
        .catchError((_) {});
  }

  // ─── Package info setters ──────────────────────────────────────────────────

  void setPackageDesc(String v) => _emit(_state.copyWith(packageDesc: v));
  void setPackageType(String v) => _emit(_state.copyWith(packageType: v));
  void setPackageWeight(double v) => _emit(_state.copyWith(packageWeight: v));
  void setPackageSize(PackageSizeOption v) {
    _emit(_state.copyWith(packageSize: v, clearFare: true));
    _fetchPreviewFare();
  }

  void setIsFragile(bool v) => _emit(_state.copyWith(isFragile: v));
  void setServiceTier(ServiceTierOption v) {
    _emit(_state.copyWith(serviceTier: v, clearFare: true));
    _fetchPreviewFare();
  }

  void setReceiverName(String v) => _emit(_state.copyWith(receiverName: v));
  void setReceiverPhone(String v) => _emit(_state.copyWith(receiverPhone: v));

  void proceedFromPackageInfoToPayment() {
    initiatePayment();
  }

  // ─── Review setters ───────────────────────────────────────────────────────

  void setReviewRating(int v) => _emit(_state.copyWith(reviewRating: v));
  void setReviewText(String v) => _emit(_state.copyWith(reviewText: v));
  void setRecommendsRider(bool? v) {
    if (v == null) {
      _emit(_state.copyWith(clearRecommendsRider: true));
    } else {
      _emit(_state.copyWith(recommendsRider: v));
    }
  }

  Future<void> submitReview() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      await _api.submitReview(
        accessToken: token,
        bookingId: bookingId,
        rating: _state.reviewRating,
        reviewText: _state.reviewText,
        recommendsRider: _state.recommendsRider,
      );
      _bumpGeneration();
      _emit(_state.copyWith(
        status: DispatchFlowStatus.completed,
        isLoading: false,
      ));
      unawaited(loadHistory());
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _errorMessage(e)));
    }
  }

  void skipReview() {
    _bumpGeneration();
    _emit(_state.copyWith(status: DispatchFlowStatus.completed));
    unawaited(loadHistory());
  }

  /// Submits a review for an arbitrary delivered booking (used from the Trips
  /// detail screen). Refreshes history afterwards.
  Future<DispatchBookingReview> submitReviewForBooking({
    required String bookingId,
    required int rating,
    String reviewText = '',
    bool? recommendsRider,
  }) async {
    final token = _accessToken();
    if (token == null) {
      throw StateError('Not authenticated');
    }
    final review = await _api.submitReview(
      accessToken: token,
      bookingId: bookingId,
      rating: rating,
      reviewText: reviewText,
      recommendsRider: recommendsRider,
    );
    unawaited(loadHistory());
    return review;
  }

  // ─── Payment initiation ────────────────────────────────────────────────────

  Future<void> initiatePayment() async {
    final token = _accessToken();
    if (token == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));

    try {
      final estimate = await _api.estimateFare(
        pickupLat: _state.pickupLat,
        pickupLng: _state.pickupLng,
        dropoffLat: _state.dropoffLat,
        dropoffLng: _state.dropoffLng,
        packageSize: _state.packageSize?.name ?? 'small',
        serviceTier: _state.serviceTier.name,
      );

      int? walletBalance;
      try {
        final wallet = await _walletApi.getWallet(accessToken: token);
        walletBalance = wallet.availableKobo;
      } catch (_) {
        // non-fatal: wallet fetch failure just means balance won't show
      }

      _emit(_state.copyWith(
        status: DispatchFlowStatus.payment,
        isLoading: false,
        fareEstimate: estimate,
        walletBalanceKobo: walletBalance ?? _state.walletBalanceKobo,
        // Wallet is the only payment method; preselect it.
        paymentMethod: _state.paymentMethod ?? 'wallet',
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _errorMessage(e)));
    }
  }

  /// Booking is wallet-only. The fare is held from the wallet when the booking
  /// is created (fail fast on low balance), then rider matching starts.
  Future<void> confirmPayment() async {
    final token = _accessToken();
    if (token == null) return;

    _emit(_state.copyWith(
      status: DispatchFlowStatus.paymentProcessing,
      isLoading: true,
      clearError: true,
    ));

    await _createBookingAndStartSearch(token);
  }

  /// Exposes what the payment view needs to launch the shared wallet top-up
  /// flow (the funding screen wires its own controller from these).
  WalletApi get walletApi => _walletApi;
  String? get accessTokenForWallet => _accessToken();
  String get customerEmail {
    final s = _auth.state;
    return s.profileEmail.trim().isNotEmpty
        ? s.profileEmail.trim()
        : s.email.trim();
  }

  /// Re-fetches the wallet balance (e.g. after returning from a top-up).
  Future<void> refreshWalletBalance() async {
    final token = _accessToken();
    if (token == null) return;
    try {
      final wallet = await _walletApi.getWallet(accessToken: token);
      _emit(_state.copyWith(walletBalanceKobo: wallet.availableKobo));
    } catch (_) {
      // non-fatal: balance just won't update
    }
  }

  // ─── Booking creation ──────────────────────────────────────────────────────

  Future<void> _createBookingAndStartSearch(String token) async {
    try {
      final booking = await _api.createBooking(
        accessToken: token,
        pickupAddress: _state.pickupAddress,
        pickupLat: _state.pickupLat,
        pickupLng: _state.pickupLng,
        dropoffAddress: _state.dropoffAddress,
        dropoffLat: _state.dropoffLat,
        dropoffLng: _state.dropoffLng,
        receiverName: _state.receiverName,
        receiverPhone: _state.receiverPhone,
        packageDesc: _state.packageDesc,
        packageWeight: _state.packageWeight,
        packageType: _state.packageType,
        packageSize: _state.packageSize?.name ?? 'small',
        isFragile: _state.isFragile,
        serviceTier: _state.serviceTier.name,
        paymentMethod: 'wallet',
      );
      _emit(_state.copyWith(
        status: DispatchFlowStatus.searching,
        isLoading: false,
        activeBooking: booking,
      ));
      _startPolling();
    } catch (e) {
      _emit(_state.copyWith(
        status: DispatchFlowStatus.payment,
        isLoading: false,
        error: _errorMessage(e),
      ));
    }
  }

  // ─── Cancel ────────────────────────────────────────────────────────────────

  Future<void> cancelBooking({String reason = ''}) async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;

    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      final updated = await _api.cancelBooking(
        accessToken: token,
        bookingId: bookingId,
        reason: reason,
      );
      _bumpGeneration();
      _stopPolling();
      _emit(_state.copyWith(
        status: DispatchFlowStatus.cancelled,
        isLoading: false,
        activeBooking: updated,
      ));
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _errorMessage(e)));
    }
  }

  /// "Edit Destination" while still connecting to a rider: the booking is
  /// already created server-side, so editing means cancelling the current
  /// search and returning to location entry with the addresses still filled.
  Future<void> editDestinationDuringSearch() async {
    await cancelBooking(reason: 'edit_destination');
    if (_state.status == DispatchFlowStatus.cancelled) {
      startDispatchFlow();
    }
  }

  // ─── History ───────────────────────────────────────────────────────────────

  Future<void> loadHistory() async {
    final token = _accessToken();
    if (token == null) return;
    try {
      final bookings = await _api.listBookings(accessToken: token);
      _emit(_state.copyWith(bookingHistory: bookings));
    } catch (_) {
      // silently fail — history is non-critical
    }
  }

  // ─── Polling ───────────────────────────────────────────────────────────────

  void _startPolling() {
    _pollTimer?.cancel();
    _pollTimer =
        Timer.periodic(const Duration(seconds: 5), (_) => _pollBookingStatus());
    _startSearchTimeout();
    _startRealtime();
  }

  void _stopPolling() {
    _pollTimer?.cancel();
    _pollTimer = null;
    _stopSearchTimeout();
    _stopLocationPolling();
    _stopRealtime();
  }

  // ─── Realtime (websocket fast path) ─────────────────────────────────────────

  void _startRealtime() {
    if (_realtimeFactory == null) return;
    _realtime ??= _realtimeFactory(() => _accessToken(), _onRealtimeEvent);
    _realtime!.start();
  }

  void _stopRealtime() {
    _realtime?.stop();
  }

  /// A pushed event means booking state likely changed — refresh now instead of
  /// waiting for the next 5s poll.
  void _onRealtimeEvent(String _) {
    _pollBookingStatus();
  }

  /// External fast-path poke (e.g. a foreground FCM push): refresh the active
  /// booking immediately when one is being tracked. No-op otherwise.
  void pokeRefresh() {
    if (_pollTimer == null) return;
    _pollBookingStatus();
  }

  // ─── Live rider location ─────────────────────────────────────────────────────

  void _startLocationPolling() {
    if (_locationTimer != null) return;
    _pollRiderLocation();
    // The websocket is the fast path: it carries every fix the rider reports
    // instead of whatever a poll happens to catch. The poll stays on as the
    // safety net, relaxed because the socket is doing the work.
    _startLocationSocket();
    _locationTimer = Timer.periodic(
        const Duration(seconds: 20), (_) => _pollRiderLocation());
  }

  void _stopLocationPolling() {
    _locationTimer?.cancel();
    _locationTimer = null;
    _locationSocket?.stop();
    _locationSocket = null;
  }

  void _startLocationSocket() {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;

    final uri = _api.bookingTrackUri(accessToken: token, bookingId: bookingId);
    if (uri == null) return;

    _locationSocket?.stop();
    _locationSocket = WebSocketTrackSource(
      url: uri.toString(),
      // If the service has no live-track route configured the socket gives up
      // and the poll carries the delivery on its own.
      onUnavailable: _tightenLocationPoll,
    );
    _locationSocket!.start(_onTrackedPosition);
  }

  /// Falls back to the original poll cadence once the socket has given up.
  void _tightenLocationPoll() {
    if (_locationTimer == null) return;
    _locationTimer?.cancel();
    _locationTimer = Timer.periodic(
        const Duration(seconds: 8), (_) => _pollRiderLocation());
  }

  /// Applies a position pushed over the websocket.
  void _onTrackedPosition(TrackPoint point) {
    final booking = _state.activeBooking;
    if (booking == null) return;

    final location = ProviderLocation(
      lat: point.lat,
      lng: point.lng,
      available: true,
      updatedAt:
          (point.recordedAt ?? DateTime.now().toUtc()).millisecondsSinceEpoch ~/ 1000,
      heading: point.headingDegrees,
      speed: point.speedMetersPerSecond,
      accuracy: point.accuracyMeters,
    );
    _emit(_state.copyWith(
      providerLocation: location,
      tripProgress: _computeTripProgress(booking, location: location),
    ));
  }

  Future<void> _pollRiderLocation() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;
    final generation = _stateGeneration;
    try {
      final loc = await _api.getBookingLocation(
          accessToken: token, bookingId: bookingId);
      // Flow moved on (cancelled/reset) while this was in flight — drop it.
      if (generation != _stateGeneration) return;
      final booking = _state.activeBooking;
      if (booking == null) return;
      if (loc.hasFix) {
        final progress = _computeTripProgress(booking, location: loc);
        _emit(_state.copyWith(providerLocation: loc, tripProgress: progress));
      } else {
        // No fix: still advance the time-based bar.
        _emit(_state.copyWith(tripProgress: _computeTripProgress(booking)));
      }
    } catch (_) {
      // non-fatal — advance time-based bar anyway
      if (generation != _stateGeneration) return;
      final booking = _state.activeBooking;
      if (booking != null) {
        _emit(_state.copyWith(tripProgress: _computeTripProgress(booking)));
      }
    }
  }

  /// How long to wait for a rider before giving up. Slightly longer than the
  /// backend's full broadcast window (3 attempts x 60s = 180s) so the
  /// server-side no-provider-found path (which also refunds) usually lands
  /// first. On expiry the booking is cancelled server-side and the wallet hold
  /// refunded. Keep this above BROADCAST_MAX_ATTEMPTS x BROADCAST_WINDOW_SECONDS.
  static const searchTimeout = Duration(seconds: 190);

  void _startSearchTimeout() {
    _searchTimeoutTimer?.cancel();
    _searchTimeoutTimer = Timer(searchTimeout, () async {
      if (_state.status != DispatchFlowStatus.searching) return;
      final bookingId = _state.activeBooking?.id;
      final token = _accessToken();
      if (bookingId != null && token != null) {
        try {
          final updated = await _api.cancelBooking(
            accessToken: token,
            bookingId: bookingId,
            reason: 'search_timeout',
          );
          _bumpGeneration();
          _stopPolling();
          _emit(_state.copyWith(
            status: DispatchFlowStatus.cancelled,
            isLoading: false,
            activeBooking: updated,
            error: 'No riders found nearby. Please try again later.',
          ));
          return;
        } catch (_) {
          // Fall through to local cancel if the server call fails.
        }
      }
      _bumpGeneration();
      _stopPolling();
      _emit(_state.copyWith(
        status: DispatchFlowStatus.cancelled,
        isLoading: false,
        error: 'No riders found nearby. Please try again later.',
      ));
    });
  }

  void _stopSearchTimeout() {
    _searchTimeoutTimer?.cancel();
    _searchTimeoutTimer = null;
  }

  // ─── Trip progress computation ──────────────────────────────────────────────

  /// Returns a 0..1 value for the delivery-leg progress slider.
  double _computeTripProgress(DispatchBooking booking,
      {ProviderLocation? location}) {
    final prev = _state.tripProgress;

    if (booking.status != DispatchBookingStatus.inTransit) return prev;

    final totalKm = _haversine(
      booking.pickupLat, booking.pickupLng,
      booking.dropoffLat, booking.dropoffLng,
    );
    if (totalKm < 0.05) return prev;

    double computed;
    final loc = location ?? _state.providerLocation;
    if (loc != null && loc.hasFix) {
      final remaining =
          _haversine(loc.lat, loc.lng, booking.dropoffLat, booking.dropoffLng);
      computed = (1 - remaining / totalKm).clamp(0.05, 0.95);
    } else {
      // Time-based fallback: estimate progress from elapsed time since pickup.
      final started = _state.tripStartedAt;
      if (started == null) return prev;
      final etaMinutes = (totalKm / 18.0 * 60).ceil().clamp(1, 300);
      final elapsedSeconds = DateTime.now().difference(started).inSeconds;
      computed = (elapsedSeconds / (etaMinutes * 60)).clamp(0.05, 0.95);
    }

    // Monotonic: never regress even if GPS jitters or the clock re-anchors.
    return math.max(prev, computed);
  }

  static double _haversine(double lat1, double lng1, double lat2, double lng2) {
    const r = 6371.0;
    final dLat = (lat2 - lat1) * math.pi / 180;
    final dLng = (lng2 - lng1) * math.pi / 180;
    final a = math.sin(dLat / 2) * math.sin(dLat / 2) +
        math.cos(lat1 * math.pi / 180) *
            math.cos(lat2 * math.pi / 180) *
            math.sin(dLng / 2) *
            math.sin(dLng / 2);
    return 2 * r * math.asin(math.min(1, math.sqrt(a)));
  }

  Future<void> _pollBookingStatus() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null || _pollInFlight) return;

    _pollInFlight = true;
    final generation = _stateGeneration;
    try {
      final booking =
          await _api.getBooking(accessToken: token, bookingId: bookingId);
      // A local mutation (cancel/reset/review) happened while this response was
      // in flight — the data is stale, don't let it revive the old screen.
      if (generation != _stateGeneration) return;
      await _applyBookingUpdate(booking);
    } catch (_) {
      // network hiccup — keep polling
    } finally {
      _pollInFlight = false;
    }
  }

  Future<void> _applyBookingUpdate(DispatchBooking booking) async {
    final status = booking.status;

    if (status.isTerminal) {
      _stopPolling();
      final flowStatus = status == DispatchBookingStatus.completed
          ? DispatchFlowStatus.completed
          : DispatchFlowStatus.cancelled;
      _emit(_state.copyWith(status: flowStatus, activeBooking: booking));
      unawaited(loadHistory());
      return;
    }

    if (status == DispatchBookingStatus.delivered) {
      _stopPolling();
      // The customer may already have reviewed this delivery (e.g. from the
      // Trips detail screen). Skip straight to completed instead of offering a
      // form whose submit the server would reject as a duplicate.
      final token = _accessToken();
      if (token != null) {
        try {
          final existing = await _api.getReviewForBooking(
            accessToken: token,
            bookingId: booking.id,
          );
          if (existing != null) {
            _emit(_state.copyWith(
                status: DispatchFlowStatus.completed, activeBooking: booking));
            unawaited(loadHistory());
            return;
          }
        } catch (_) {
          // Review lookup failing must not block the flow — show the form; the
          // server still guards against duplicates.
        }
      }
      _emit(_state.copyWith(
          status: DispatchFlowStatus.review, activeBooking: booking));
      return;
    }

    if (status.isActive) {
      // Delivery is live — start streaming the rider's location for the map.
      _startLocationPolling();

      // Anchor the start time the first time the package is picked up so the
      // time-based progress branch has a reference point.
      final startedAt = (status == DispatchBookingStatus.inTransit &&
              _state.tripStartedAt == null)
          ? DateTime.now()
          : _state.tripStartedAt;

      _emit(_state.copyWith(
        status: DispatchFlowStatus.activeTrip,
        activeBooking: booking,
        tripStartedAt: startedAt,
        tripProgress: _computeTripProgress(booking),
      ));
      _maybeFetchRiderInfo(booking);
      // The delivery code is generated once the rider starts the trip —
      // fetch it once so the customer can display it to the rider later.
      if (status == DispatchBookingStatus.inTransit &&
          _state.deliveryVerification == null) {
        unawaited(_fetchDeliveryCode());
      }
      return;
    }

    // Still searching. Clear a stale rider snapshot if the booking dropped
    // back to searching without a provider.
    _emit(_state.copyWith(
      activeBooking: booking,
      clearProviderSnapshot: booking.providerId == null,
    ));
    _maybeFetchRiderInfo(booking);
  }

  /// Loads the assigned rider's public profile once a provider is known.
  void _maybeFetchRiderInfo(DispatchBooking booking) {
    final providerId = booking.providerId;
    if (providerId == null) return;
    final token = _accessToken();
    if (token == null) return;

    if (_state.providerSnapshot?.id != providerId) {
      _api
          .getProvider(accessToken: token, providerId: providerId)
          .then((p) {
            _emit(_state.copyWith(providerSnapshot: p));
          })
          .catchError((_) {});
    }
  }

  // ─── Handover verification ─────────────────────────────────────────────────

  /// Submits the rider's pickup credential (scanned or typed) for the
  /// customer to verify. On success, the rider's own "start trip" action
  /// becomes unblocked server-side.
  Future<bool> verifyDriver(String code) async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return false;
    _emit(_state.copyWith(isLoading: true, clearError: true));
    try {
      await _api.verifyDriver(accessToken: token, bookingId: bookingId, code: code);
      _emit(_state.copyWith(isLoading: false, pickupVerifiedLocally: true, clearError: true));
      return true;
    } catch (e) {
      _emit(_state.copyWith(isLoading: false, error: _errorMessage(e)));
      return false;
    }
  }

  Future<void> _fetchDeliveryCode() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;
    try {
      final result = await _api.getVerificationCode(
        accessToken: token,
        bookingId: bookingId,
      );
      _emit(_state.copyWith(deliveryVerification: result));
    } catch (_) {
      // Missed/failed fetch — the resend button lets the customer retry.
    }
  }

  Future<void> resendDeliveryCode() async {
    final token = _accessToken();
    final bookingId = _state.activeBooking?.id;
    if (token == null || bookingId == null) return;
    _emit(_state.copyWith(isResendingCode: true, clearError: true));
    try {
      final result = await _api.resendVerificationCode(
        accessToken: token,
        bookingId: bookingId,
      );
      _emit(_state.copyWith(deliveryVerification: result, isResendingCode: false));
    } catch (e) {
      _emit(_state.copyWith(isResendingCode: false, error: _errorMessage(e)));
    }
  }

  // ─── Navigation helpers ────────────────────────────────────────────────────

  void reset() {
    _bumpGeneration();
    _stopPolling();
    _emit(const DispatchBookingState.idle());
  }

  void backToPackageInfo() {
    _emit(_state.copyWith(
        status: DispatchFlowStatus.packageInfo, clearError: true));
  }

  void backToLocationEntry() {
    _emit(_state.copyWith(
        status: DispatchFlowStatus.locationEntry, clearError: true));
  }

  // ─── Utilities ─────────────────────────────────────────────────────────────

  String _errorMessage(Object e) {
    if (e is ApiException) return e.message;
    return e.toString();
  }

  @override
  void dispose() {
    _stopPolling();
    _stopSearchTimeout();
    _stopLocationPolling();
    super.dispose();
  }
}
